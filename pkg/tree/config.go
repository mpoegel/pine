package tree

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Name       string
	OriginFile string
	Command    string

	EnvironmentFile string
	StdoutFile      string
	StderrFile      string

	Restart         RestartLevel
	RestartAttempts int
	RestartDelay    time.Duration
}

type RestartLevel string

const (
	AlwaysRestart  RestartLevel = "always"
	NeverRestart   RestartLevel = "never"
	LimitedRestart RestartLevel = "limited"
)

func LoadConfig(filename string) (Config, error) {
	cfg := Config{
		OriginFile: filename,
		// defaults
		Restart:         NeverRestart,
		RestartAttempts: 3,
		RestartDelay:    3 * time.Second,
	}
	fp, err := os.Open(filename)
	if err != nil {
		return cfg, err
	}
	defer fp.Close()

	scanner := bufio.NewScanner(fp)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 1 {
			return cfg, fmt.Errorf("invalid config syntax on line %d", lineNum)
		}

		param := strings.Trim(parts[0], " \t")
		value := strings.Trim(parts[1], " \t")
		switch param {
		default:
			return cfg, fmt.Errorf("unknown parameter '%s' on line %d", param, lineNum)
		case "Name":
			cfg.Name = value
		case "Command":
			cfg.Command = value
		case "EnvironmentFile":
			cfg.EnvironmentFile = value
		case "StdoutFile":
			cfg.EnvironmentFile = value
		case "StderrFile":
			cfg.StderrFile = value
		case "Restart":
			switch value {
			case "always":
				cfg.Restart = AlwaysRestart
			case "never":
				cfg.Restart = NeverRestart
			case "limited":
				cfg.Restart = LimitedRestart
			default:
				return cfg, fmt.Errorf("unknown restart value '%s' on line %d", value, lineNum)
			}
		case "RestartAttempts":
			if cfg.RestartAttempts, err = strconv.Atoi(value); err != nil {
				return cfg, fmt.Errorf("invalid restart attempts '%s' on line %d", value, lineNum)
			}
		case "RestartDelay":
			if cfg.RestartDelay, err = time.ParseDuration(value); err != nil {
				return cfg, fmt.Errorf("invalid restart delay '%s' on line %d", value, lineNum)
			}
		}
	}

	return cfg, nil
}
