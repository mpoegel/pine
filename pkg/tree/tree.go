package tree

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Tree interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (*Status, error)
	Restart(ctx context.Context) error
	Destroy(ctx context.Context) error
	Config() Config
}

type TreeImpl struct {
	config Config

	stopChan      chan bool
	fullStop      bool
	runCount      int
	currState     State
	startedAt     time.Time
	lastChangedAt time.Time
}

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
			time.Sleep(t.config.RestartDelay)
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
	commandParts := strings.Split(t.config.Command, " ")
	args := []string{}
	if len(commandParts) > 1 {
		args = commandParts[1:]
	}
	cmd := exec.CommandContext(ctx, commandParts[0], args...)
	if len(t.config.EnvironmentFile) > 0 {
		envVars, err := t.loadEnvFile(t.config.EnvironmentFile)
		if err != nil {
			errChan <- err
			return
		}
		cmd.Env = envVars
	}
	openFiles := []*os.File{}
	defer func() {
		for _, fp := range openFiles {
			fp.Close()
		}
	}()
	if len(t.config.StdoutFile) > 0 {
		fp, err := os.OpenFile(t.config.StdoutFile, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			errChan <- err
			return
		}
		openFiles = append(openFiles, fp)
		cmd.Stdout = fp
	}
	if t.config.StdoutFile == t.config.StderrFile {
		cmd.Stderr = cmd.Stdout
	} else if len(t.config.StderrFile) > 0 {
		fp, err := os.OpenFile(t.config.StderrFile, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			errChan <- err
			return
		}
		openFiles = append(openFiles, fp)
		cmd.Stderr = fp
	}
	t.runCount++
	t.startedAt = time.Now()
	t.currState = RunningState
	errChan <- cmd.Run()
}

func (t *TreeImpl) runWait(cancel context.CancelFunc, errChan chan error) error {
	for {
		select {
		case <-t.stopChan:
			cancel()
		case err := <-errChan:
			if err != nil {
				slog.Warn("tree exited abnormally", "name", t.config.Name, "err", err)
			} else {
				slog.Info("tree exited", "name", t.config.Name)
			}
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
	t.stopChan <- true
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
	t.stopChan <- true
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
