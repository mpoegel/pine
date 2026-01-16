package tree

import "time"

type Status struct {
	For *Config

	State      State
	LastChange time.Time
	Uptime     time.Duration
}

type State string

const (
	RunningState    State = "running"
	StoppedState    State = "stopped"
	RestartingState State = "restarting"
)
