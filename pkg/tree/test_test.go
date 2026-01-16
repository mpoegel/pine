package tree_test

import (
	"log/slog"
	"testing"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func noErr(t *testing.T, err error) {
	if err != nil {
		t.Error(err.Error())
	}
}
