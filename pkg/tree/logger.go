package tree

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	logFileTimestampLayout = "20060102-150405"
)

type RotatingFileWriter struct {
	mu     sync.Mutex
	file   *os.File
	path   string
	maxAge time.Duration
}

var _ io.WriteCloser = (*RotatingFileWriter)(nil)

func NewRotatingFileWriter(path string, maxAge int) (*RotatingFileWriter, error) {
	if stat, err := os.Stat(path); err == nil && stat.Size() > 0 {
		rotated := stampedFilename(path)
		if err := os.Rename(path, rotated); err != nil {
			return nil, fmt.Errorf("initial rotation failed: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	maxAgeDur := time.Duration(maxAge*24) * time.Hour
	if err := removeOldFiles(path, maxAgeDur); err != nil {
		slog.Error("could not remove old logs", "log", path, "err", err)
	}

	return &RotatingFileWriter{
		file:   f,
		path:   path,
		maxAge: maxAgeDur,
	}, nil
}

func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Write(p)
}

func (w *RotatingFileWriter) Close() error {
	return w.file.Close()
}

func (w *RotatingFileWriter) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.file.Close(); err != nil {
		return err
	}

	rotated := stampedFilename(w.path)
	if err := os.Rename(w.path, rotated); err != nil {
		return err
	}

	if err := removeOldFiles(w.path, w.maxAge); err != nil {
		slog.Error("could not remove old logs", "log", w.path, "err", err)
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	w.file = f
	return nil
}

func stampedFilename(filename string) string {
	return filename + "." + time.Now().Format(logFileTimestampLayout)
}

func removeOldFiles(path string, maxAge time.Duration) error {
	filenames, err := filepath.Glob(path + "*.*")
	if err != nil {
		return err
	}

	for _, filename := range filenames {
		stat, err := os.Stat(filename)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			continue
		}
		if stat.Size() == 0 {
			if err := os.Remove(filename); err != nil {
				slog.Warn("failed to remove empty log file", "filename", filename, "err", err)
			} else {
				slog.Info("removed empty log file", "filename", filename)
			}
			continue
		}
		stamp := strings.TrimPrefix(filepath.Ext(filename), ".")
		ts, err := time.Parse(logFileTimestampLayout, stamp)
		if err != nil {
			slog.Warn("ignoring file with invalid timestamp", "filename", filename, "stamp", stamp)
			continue
		}
		if ts.Add(maxAge).Before(time.Now()) {
			if err := os.Remove(filename); err != nil {
				slog.Warn("failed to remove old log file", "filename", filename, "err", err)
			} else {
				slog.Info("removed old log file", "filename", filename)
			}
		}
	}

	return nil
}
