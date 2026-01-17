package tree

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Name       string
	OriginFile string // required
	Command    string // required
	User       string

	EnvironmentFile string
	LogFile         string
	MaxLogAge       int

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
		User:            "op",
		MaxLogAge:       7,
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
		case "User":
			cfg.Command = value
		case "EnvironmentFile":
			cfg.EnvironmentFile = value
		case "LogFile":
			cfg.LogFile = value
		case "MaxLogAge":
			if cfg.MaxLogAge, err = strconv.Atoi(value); err != nil {
				return cfg, errors.New("invalid max log age")
			}
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

	return cfg, ValidateConfig(&cfg)
}

func ValidateConfig(cfg *Config) error {
	if len(cfg.OriginFile) == 0 {
		return errors.New("missing origin file")
	}
	if len(cfg.Command) == 0 {
		return errors.New("missing command")
	}
	if len(cfg.Name) == 0 {
		cfg.Name = strings.TrimSuffix(filepath.Base(cfg.OriginFile), filepath.Ext(cfg.OriginFile))
	}
	if len(cfg.LogFile) == 0 {
		cfg.LogFile = fmt.Sprintf("/var/log/homelab/%s.log", cfg.Name)
	}
	if cfg.MaxLogAge < 1 {
		return errors.New("invalid max log age")
	}
	return nil
}
