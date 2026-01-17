package pine

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"sync"
	"time"

	fsnotify "github.com/fsnotify/fsnotify"
	tree "github.com/mpoegel/pine/pkg/tree"
)

const (
	flushInterval = 5 * time.Second
)

type Daemon struct {
	config Config

	treeLock sync.RWMutex
	trees    map[string]tree.Tree

	wg sync.WaitGroup
}

func NewDaemon(config Config) *Daemon {
	return &Daemon{
		config:   config,
		treeLock: sync.RWMutex{},
		trees:    map[string]tree.Tree{},
		wg:       sync.WaitGroup{},
	}
}

func (d *Daemon) Run(ctx context.Context) error {
	slog.Info("running daemon")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if d.config.UnprivilegedMode {
		currUser, err := user.Current()
		if err != nil {
			return err
		}
		tree.DefaultUser = currUser.Username
	}

	ln, err := net.Listen("unix", d.config.UdsEndpoint)
	if err != nil {
		return err
	}
	d.wg.Go(func() {
		slog.Info("starting http server", "endpoint", d.config.UdsEndpoint)
		httpServer := NewHttpServer(d)
		err := httpServer.Start(ctx, ln)
		slog.Info("http server finished", "err", err)
		ln.Close()
	})
	d.wg.Go(func() {
		d.rotateTreeLogFiles(ctx)
	})

	if err := d.findTrees(ctx); err != nil {
		return err
	}

	<-ctx.Done()
	d.stop(context.Background())
	d.wg.Wait()
	slog.Info("daemon finished")
	return nil
}

func (d *Daemon) findTrees(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	watcher.Add(d.config.TreeDir)

	files, err := filepath.Glob(path.Join(d.config.TreeDir, "*"))
	if err != nil {
		return errors.Join(err, watcher.Close())
	}
	for _, filename := range files {
		if stat, err := os.Stat(filename); err == nil && stat.IsDir() {
			continue
		}
		d.loadTree(ctx, filename)
	}

	d.wg.Go(func() {
		updateQueueLock := sync.Mutex{}
		updateQueue := map[string]bool{}
		flushTimer := time.NewTimer(flushInterval)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					updateQueueLock.Lock()
					updateQueue[event.Name] = true
					updateQueueLock.Unlock()
				} else if event.Has(fsnotify.Create) {
					d.loadTree(ctx, event.Name)
				} else if event.Has(fsnotify.Remove) {
					d.removeTree(ctx, event.Name)
				}
			case <-flushTimer.C:
				updateQueueLock.Lock()
				for filename, hasUpdate := range updateQueue {
					if hasUpdate {
						d.updateTree(ctx, filename)
						updateQueue[filename] = false
					}
				}
				updateQueueLock.Unlock()
				flushTimer.Reset(flushInterval)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Warn("tree watcher error", "err", err)
			}
		}
	})
	return nil
}

func (d *Daemon) loadTree(ctx context.Context, filename string) {
	slog.Info("adding new tree", "filename", filename)

	d.treeLock.Lock()
	t, err := tree.NewTree(filename)
	if err != nil {
		slog.Warn("failed to create new tree", "filename", filename, "err", err)
		d.treeLock.Unlock()
		return
	}

	name := t.Config().Name
	if ot, ok := d.trees[name]; ok {
		slog.Warn("conflicting tree names", "filename", filename, "name", name, "existing", ot.Config().OriginFile)
		t.Destroy(ctx)
		d.treeLock.Unlock()
		return
	}
	d.trees[name] = t
	d.treeLock.Unlock()

	d.StartTree(ctx, name)
}

func (d *Daemon) updateTree(ctx context.Context, filename string) {
	slog.Info("updating tree", "filename", filename)

	newConfig, err := tree.LoadConfig(filename)
	if err != nil {
		slog.Warn("cannot update tree", "err", err)
		return
	}
	name := newConfig.Name

	d.treeLock.RLock()
	t, ok := d.trees[name]
	if !ok {
		slog.Warn("tree not found to update", "name", name, "filename", filename)
		return
	}

	t.Reload(ctx)
	d.treeLock.RUnlock()
}

func (d *Daemon) removeTree(ctx context.Context, filename string) {
	slog.Info("removing tree", "filename", filename)

	cfg, err := tree.LoadConfig(filename)
	if err != nil {
		slog.Warn("tree config is invalid, not removing any trees", "err", err)
		return
	}

	d.treeLock.Lock()
	defer d.treeLock.Unlock()

	t, ok := d.trees[cfg.Name]
	if !ok {
		slog.Warn("tree not found to remove", "name", cfg.Name)
		return
	}

	t.Destroy(ctx)
	delete(d.trees, cfg.Name)
}

func (d *Daemon) stop(ctx context.Context) {
	d.treeLock.Lock()
	defer d.treeLock.Unlock()

	slog.Info("shutting down daemon")

	for _, t := range d.trees {
		t.Destroy(ctx)
	}
}

func (d *Daemon) StartTree(ctx context.Context, name string) error {
	d.treeLock.RLock()
	defer d.treeLock.RUnlock()
	t, ok := d.trees[name]
	if !ok {
		return errors.New("tree not found")
	} else {
		d.wg.Go(func() {
			err := t.Start(ctx)
			slog.Info("tree finished", "name", name, "err", err)
		})
	}
	return nil
}

func (d *Daemon) StopTree(ctx context.Context, name string) error {
	d.treeLock.RLock()
	defer d.treeLock.RUnlock()
	t, ok := d.trees[name]
	if !ok {
		return errors.New("tree not found")
	} else {
		t.Stop(ctx)
	}
	return nil
}

func (d *Daemon) RestartTree(ctx context.Context, name string) error {
	d.treeLock.RLock()
	defer d.treeLock.RUnlock()
	t, ok := d.trees[name]
	if !ok {
		return errors.New("tree not found")
	} else if t.Config().Restart == tree.NeverRestart {
		return errors.New("cannot restart")
	} else {
		t.Restart(ctx)
	}
	return nil
}

func (d *Daemon) GetTreeStatus(ctx context.Context, name string) (*tree.Status, error) {
	d.treeLock.RLock()
	defer d.treeLock.RUnlock()
	t, ok := d.trees[name]
	if !ok {
		return nil, errors.New("tree not found")
	} else {
		return t.Status(ctx)
	}
}

func (d *Daemon) rotateTreeLogFiles(ctx context.Context) {
	timer := timerUntilMidnight()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			slog.Info("rotating tree logs")
			d.treeLock.RLock()
			defer d.treeLock.RUnlock()
			for name, t := range d.trees {
				if err := t.RotateLog(); err != nil {
					slog.Warn("failed to rotate logfile", "name", name, "err", err)
				}
			}
			timer = timerUntilMidnight()
		}
	}
}

func (d *Daemon) ListTrees(ctx context.Context) ([]*tree.Status, error) {
	res := []*tree.Status{}
	var err error
	d.treeLock.RLock()
	defer d.treeLock.RUnlock()
	for _, t := range d.trees {
		status, statusErr := t.Status(ctx)
		if statusErr != nil {
			err = errors.Join(err)
		} else {
			res = append(res, status)
		}
	}
	return res, err
}

func (d *Daemon) RotateTreeLog(ctx context.Context, name string) error {
	d.treeLock.RLock()
	defer d.treeLock.RUnlock()
	t, ok := d.trees[name]
	if !ok {
		return errors.New("tree not found")
	} else {
		return t.RotateLog()
	}
}
