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
	config Config

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
	t.fullStop = false
	for !t.fullStop {
		t.currState = RestartingState
		if t.runCount > 0 {
			timer := time.NewTimer(t.config.RestartDelay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				return nil
			}
		}
		slog.Info("starting tree", "name", t.config.Name)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		errChan := make(chan error)
		go t.run(ctx, errChan)
		err = t.runWait(cancel, errChan)
		close(errChan)
		if t.config.Restart == NeverRestart || (t.config.Restart == LimitedRestart && t.runCount >= t.config.RestartAttempts) {
			t.currState = StoppedState
			return err
		}
	}
	t.currState = StoppedState
	return err
}

func (t *TreeImpl) run(ctx context.Context, errChan chan error) {
	t.runCount++
	commandParts := strings.Split(t.config.Command, " ")
	args := []string{}
	if len(commandParts) > 1 {
		args = commandParts[1:]
	}
	cmd := exec.CommandContext(ctx, commandParts[0], args...)
	if err := t.setCmdSysProcAttr(cmd); err != nil {
		errChan <- err
		return
	}
	if len(t.config.EnvironmentFile) > 0 {
		envVars, err := t.loadEnvFile(t.config.EnvironmentFile)
		if err != nil {
			errChan <- err
			return
		}
		cmd.Env = envVars
	}
	var err error
	t.logger, err = NewRotatingFileWriter(t.config.LogFile, t.config.MaxLogAge)
	if err != nil {
		errChan <- err
		return
	}
	defer t.logger.Close()
	cmd.Stdout = t.logger
	cmd.Stderr = t.logger
	t.startedAt = time.Now()
	t.currState = RunningState
	errChan <- cmd.Run()
}

func (t *TreeImpl) setCmdSysProcAttr(cmd *exec.Cmd) error {
	currUser, err := user.Current()
	if err != nil || currUser.Username == t.config.User {
		return err
	}

	// Look up the user details
	u, err := user.Lookup(t.config.User)
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
	slog.Info("stopping tree", "name", t.config.Name)
	t.fullStop = true
	select {
	case t.stopChan <- true:
	default:
	}
	return nil
}

func (t *TreeImpl) Status(ctx context.Context) (*Status, error) {
	status := &Status{
		For:        &t.config,
		State:      t.currState,
		Uptime:     0,
		LastChange: t.lastChangedAt,
	}
	if t.currState == RunningState {
		status.Uptime = time.Since(t.startedAt)
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
	return t.config
}

func (t *TreeImpl) RotateLog() error {
	if t.logger != nil {
		return t.logger.Rotate()
	}
	return nil
}

func (t *TreeImpl) Reload(ctx context.Context) error {
	newConfig, err := LoadConfig(t.config.OriginFile)
	if err != nil {
		return err
	}

	t.config = newConfig
	t.Restart(ctx)
	return nil
}
