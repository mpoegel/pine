package tree_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	tree "github.com/mpoegel/pine/pkg/tree"
)

func TestReloadRaceCondition(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "reload-test-*.tree")
	noErr(t, err)
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("Name ReloadRace\nCommand sleep 300\n")
	tmpFile.Close()

	treeImpl, err := tree.NewTree(tmpFile.Name())
	noErr(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- treeImpl.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				treeImpl.Reload(context.Background())
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
	cancel()
	<-errChan
}

func TestRestartDuringReload(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "restart-reload-*.tree")
	noErr(t, err)
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString("Name RestartReload\nCommand sleep 300\nRestart always\nRestartDelay 10ms\n")
	tmpFile.Close()

	treeImpl, err := tree.NewTree(tmpFile.Name())
	noErr(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- treeImpl.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 20; i++ {
		treeImpl.Reload(context.Background())
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	<-errChan
}
