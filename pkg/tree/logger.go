package tree

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type RotatingFileWriter struct {
	mu   sync.Mutex
	file *os.File
	path string
}

var _ io.WriteCloser = (*RotatingFileWriter)(nil)

func NewRotatingFileWriter(path string) (*RotatingFileWriter, error) {
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

	return &RotatingFileWriter{
		file: f,
		path: path,
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

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	w.file = f
	return nil
}

func stampedFilename(filename string) string {
	return filename + "." + time.Now().Format("20060102-150405")
}
