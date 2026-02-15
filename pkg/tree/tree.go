package tree

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Tree interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (*Status, error)
	Restart(ctx context.Context) error
	Destroy(ctx context.Context) error
	RotateLog() error
	Config() Config
	Reload(ctx context.Context) error
}

type TreeImpl struct {
	configMu sync.RWMutex
	config   Config

	stateMu       sync.Mutex
	stopChan      chan bool
	fullStop      bool
	runCount      int
	currState     State
	startedAt     time.Time
	lastChangedAt time.Time
	logger        *RotatingFileWriter
}

var _ Tree = (*TreeImpl)(nil)

func NewTree(cfgFile string) (*TreeImpl, error) {
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		return nil, err
	}

	return &TreeImpl{
		config:        cfg,
		stopChan:      make(chan bool, 1),
		fullStop:      false,
		runCount:      0,
		currState:     StoppedState,
		lastChangedAt: time.Now(),
	}, nil
}

func (t *TreeImpl) Start(ctx context.Context) error {
	var err error

	t.configMu.RLock()
	restartMode := t.config.Restart
	restartAttempts := t.config.RestartAttempts
	restartDelay := t.config.RestartDelay
	name := t.config.Name
	t.configMu.RUnlock()

	t.stateMu.Lock()
	t.fullStop = false
	t.runCount = 0
	t.stateMu.Unlock()

	for {
		t.stateMu.Lock()
		if t.fullStop {
			t.currState = StoppedState
			t.stateMu.Unlock()
			return err
		}
		t.currState = RestartingState
		runCount := t.runCount
		t.stateMu.Unlock()

		if runCount > 0 {
			timer := time.NewTimer(restartDelay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				return nil
			}
		}
		slog.Info("starting tree", "name", name)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		t.configMu.RLock()
		cmd := t.config.Command
		envFile := t.config.EnvironmentFile
		user := t.config.User
		logFile := t.config.LogFile
		maxLogAge := t.config.MaxLogAge
		t.configMu.RUnlock()

		errChan := make(chan error)
		go t.run(ctx, cmd, envFile, user, logFile, maxLogAge, errChan)
		err = t.runWait(cancel, errChan)
		close(errChan)

		t.stateMu.Lock()
		currentRunCount := t.runCount
		shouldStop := t.fullStop || (restartMode == NeverRestart) || (restartMode == LimitedRestart && currentRunCount >= restartAttempts)
		t.stateMu.Unlock()

		if shouldStop {
			t.stateMu.Lock()
			t.currState = StoppedState
			t.stateMu.Unlock()
			return err
		}
	}
}

func (t *TreeImpl) run(ctx context.Context, cmd string, envFile string, user string, logFile string, maxLogAge int, errChan chan error) {
	t.stateMu.Lock()
	t.runCount++
	t.stateMu.Unlock()

	commandParts := strings.Split(cmd, " ")
	args := []string{}
	if len(commandParts) > 1 {
		args = commandParts[1:]
	}
	execCmd := exec.CommandContext(ctx, commandParts[0], args...)
	if err := t.setCmdSysProcAttr(execCmd, user); err != nil {
		errChan <- err
		return
	}
	if len(envFile) > 0 {
		envVars, err := t.loadEnvFile(envFile)
		if err != nil {
			errChan <- err
			return
		}
		execCmd.Env = envVars
	}
	var err error
	t.logger, err = NewRotatingFileWriter(logFile, maxLogAge)
	if err != nil {
		errChan <- err
		return
	}
	defer t.logger.Close()
	execCmd.Stdout = t.logger
	execCmd.Stderr = t.logger

	t.stateMu.Lock()
	t.startedAt = time.Now()
	t.currState = RunningState
	t.stateMu.Unlock()

	errChan <- execCmd.Run()
}

func (t *TreeImpl) setCmdSysProcAttr(cmd *exec.Cmd, targetUser string) error {
	currUser, err := user.Current()
	if err != nil || currUser.Username == targetUser {
		return err
	}

	// Look up the user details
	u, err := user.Lookup(targetUser)
	if err != nil {
		return err
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("failed to parse UID: %w", err)
	}

	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return fmt.Errorf("failed to parse GID: %w", err)
	}

	// Set the SysProcAttr to run as the target user
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
	}
	return nil
}

func (t *TreeImpl) runWait(cancel context.CancelFunc, errChan chan error) error {
	for {
		select {
		case <-t.stopChan:
			cancel()
		case err := <-errChan:
			return err
		}
	}
}

func (t *TreeImpl) loadEnvFile(filename string) ([]string, error) {
	res := []string{}
	fp, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	scanner := bufio.NewScanner(fp)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if !strings.Contains(line, "=") {
			return nil, fmt.Errorf("invalid environment file format on line %d", lineNum)
		}
		res = append(res, line)
	}
	return res, nil
}

func (t *TreeImpl) Stop(ctx context.Context) error {
	t.configMu.RLock()
	name := t.config.Name
	t.configMu.RUnlock()
	slog.Info("stopping tree", "name", name)
	t.stateMu.Lock()
	t.fullStop = true
	t.stateMu.Unlock()
	select {
	case t.stopChan <- true:
	default:
	}
	return nil
}

func (t *TreeImpl) Status(ctx context.Context) (*Status, error) {
	t.configMu.RLock()
	cfg := t.config
	t.configMu.RUnlock()

	t.stateMu.Lock()
	currState := t.currState
	startedAt := t.startedAt
	lastChangedAt := t.lastChangedAt
	t.stateMu.Unlock()

	status := &Status{
		For:        &cfg,
		State:      currState,
		Uptime:     0,
		LastChange: lastChangedAt,
	}
	if currState == RunningState {
		status.Uptime = time.Since(startedAt)
	}
	return status, nil
}

func (t *TreeImpl) Restart(ctx context.Context) error {
	select {
	case t.stopChan <- true:
	default:
	}
	return nil
}

func (t *TreeImpl) Destroy(ctx context.Context) error {
	err := t.Stop(ctx)
	close(t.stopChan)
	return err
}

func (t *TreeImpl) Config() Config {
	t.configMu.RLock()
	defer t.configMu.RUnlock()
	return t.config
}

func (t *TreeImpl) RotateLog() error {
	if t.logger != nil {
		return t.logger.Rotate()
	}
	return nil
}

func (t *TreeImpl) Reload(ctx context.Context) error {
	t.configMu.Lock()
	defer t.configMu.Unlock()

	newConfig, err := LoadConfig(t.config.OriginFile)
	if err != nil {
		return err
	}

	t.config = newConfig
	t.Restart(ctx)
	return nil
}
