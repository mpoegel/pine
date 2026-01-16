package tree_test

import (
	"testing"

	tree "github.com/mpoegel/pine/pkg/tree"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := tree.LoadConfig("testdata/sleep.tree")
	noErr(t, err)

	if cfg.Name != "Sleeper" {
		t.Errorf("unexpected name: '%s'", cfg.Name)
	}
	if cfg.Command != "sleep infinity" {
		t.Errorf("unexpected command: '%s'", cfg.Command)
	}
}
