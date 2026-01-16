package pine

import (
	"context"
	"errors"
	"log/slog"
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
