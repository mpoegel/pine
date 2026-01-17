package pine

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"path"
	"path/filepath"
	"sync"

	fsnotify "github.com/fsnotify/fsnotify"
	tree "github.com/mpoegel/pine/pkg/tree"
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
		d.loadTree(ctx, filename)
	}

	d.wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					d.updateTree(ctx, event.Name)
				} else if event.Has(fsnotify.Create) {
					d.loadTree(ctx, event.Name)
				} else if event.Has(fsnotify.Remove) {
					d.removeTree(ctx, event.Name)
				}
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
	defer d.treeLock.Unlock()
	t, err := tree.NewTree(filename)
	if err != nil {
		slog.Warn("failed to create new tree", "filename", filename, "err", err)
		return
	}

	name := t.Config().Name
	if ot, ok := d.trees[name]; ok {
		slog.Warn("conflicting tree names", "filename", filename, "name", name, "existing", ot.Config().OriginFile)
		t.Destroy(ctx)
		return
	}
	d.trees[name] = t

	d.wg.Go(func() {
		err = t.Start(ctx)
		slog.Info("tree finished", "name", name, "filename", filename, "err", err)
	})
}

func (d *Daemon) updateTree(ctx context.Context, filename string) {
	slog.Info("updating tree", "filename", filename)
	// TODO
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
