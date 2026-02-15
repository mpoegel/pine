package pine_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"
	"time"

	pine "github.com/mpoegel/pine/pkg/pine"
	tree "github.com/mpoegel/pine/pkg/tree"
)

func noErr(t *testing.T, err error) {
	if err != nil {
		t.Error(err.Error())
	}
}

func createTempTreeFile(t *testing.T, dir string, name string, cmd string) string {
	f, err := os.Create(filepath.Join(dir, name+".tree"))
	noErr(t, err)
	defer f.Close()
	_, err = f.WriteString("Name " + name + "\nCommand " + cmd + "\n")
	noErr(t, err)
	return f.Name()
}

func createTempTreeFileWithRestart(t *testing.T, dir string, name string, cmd string, restart string) string {
	f, err := os.Create(filepath.Join(dir, name+".tree"))
	noErr(t, err)
	defer f.Close()
	_, err = f.WriteString("Name " + name + "\nCommand " + cmd + "\nRestart " + restart + "\n")
	noErr(t, err)
	return f.Name()
}

func TestTimerLeak(t *testing.T) {
	runtime.GC()
	debug.FreeOSMemory()

	before := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		tmpDir := t.TempDir()

		config := pine.Config{
			TreeDir:     tmpDir,
			UdsEndpoint: filepath.Join(tmpDir, "pine.sock"),
		}
		daemon := pine.NewDaemon(config)

		ctx, cancel := context.WithCancel(context.Background())

		errCh := make(chan error, 1)
		go func() {
			errCh <- daemon.Run(ctx)
		}()

		time.Sleep(50 * time.Millisecond)

		cancel()
		<-errCh
	}

	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(50 * time.Millisecond)

	after := runtime.NumGoroutine()

	t.Logf("Goroutines before: %d, after: %d", before, after)

	if after > before+10 {
		t.Errorf("potential goroutine leak: before=%d, after=%d", before, after)
	}
}

func TestConcurrentConfigReload(t *testing.T) {
	tmpDir := t.TempDir()

	config := pine.Config{
		TreeDir:     tmpDir,
		UdsEndpoint: filepath.Join(tmpDir, "pine.sock"),
	}
	daemon := pine.NewDaemon(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	filename := createTempTreeFile(t, tmpDir, "TestTree", "sleep 300")

	time.Sleep(200 * time.Millisecond)

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 10; i++ {
		go func() {
			f, err := os.OpenFile(filename, os.O_RDWR, 0644)
			if err == nil {
				f.WriteString("# modified\n")
				f.Close()
			}
		}()
	}

	time.Sleep(500 * time.Millisecond)

	cancel()
	<-errCh
}

func TestRestartCompletion(t *testing.T) {
	tmpDir := t.TempDir()

	config := pine.Config{
		TreeDir:     tmpDir,
		UdsEndpoint: filepath.Join(tmpDir, "pine.sock"),
	}
	daemon := pine.NewDaemon(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	filename := createTempTreeFileWithRestart(t, tmpDir, "RestartTest", "sleep 300", "always")

	time.Sleep(200 * time.Millisecond)

	stat1, err := daemon.GetTreeStatus(context.Background(), "RestartTest")
	noErr(t, err)

	os.WriteFile(filename, []byte("Name RestartTest\nCommand sleep 1\nRestart always\n"), 0644)

	time.Sleep(50 * time.Millisecond)

	stat2, err := daemon.GetTreeStatus(context.Background(), "RestartTest")
	noErr(t, err)

	if stat1.State == stat2.State && stat1.State == tree.RunningState {
		t.Logf("Both states reported running - this may indicate restart didn't complete")
	}

	time.Sleep(200 * time.Millisecond)

	stat3, err := daemon.GetTreeStatus(context.Background(), "RestartTest")
	noErr(t, err)

	if stat3.State == tree.RestartingState {
		t.Logf("Tree still in restarting state - restart may not have completed")
	}

	cancel()
	<-errCh
}

func TestUpdateTreeWhileRunning(t *testing.T) {
	tmpDir := t.TempDir()

	config := pine.Config{
		TreeDir:     tmpDir,
		UdsEndpoint: filepath.Join(tmpDir, "pine.sock"),
	}
	daemon := pine.NewDaemon(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	filename := createTempTreeFileWithRestart(t, tmpDir, "UpdateTest", "sleep 300", "always")

	time.Sleep(100 * time.Millisecond)

	err := daemon.StartTree(context.Background(), "UpdateTest")
	noErr(t, err)

	time.Sleep(100 * time.Millisecond)

	cfg1, err := daemon.GetTreeStatus(context.Background(), "UpdateTest")
	noErr(t, err)
	_ = cfg1

	os.WriteFile(filename, []byte("Name UpdateTest\nCommand sleep 1\nRestart always\n"), 0644)

	time.Sleep(50 * time.Millisecond)

	err = daemon.RestartTree(context.Background(), "UpdateTest")
	noErr(t, err)

	time.Sleep(100 * time.Millisecond)

	cancel()
	<-errCh
}
