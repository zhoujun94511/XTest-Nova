package logrotation

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Writer struct {
	mu      sync.Mutex
	path    string
	options Options
	file    *os.File
	size    int64
}

func NewWriter(path string, options Options) (*Writer, error) {
	if err := Rotate(path, options, time.Now()); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &Writer{path: path, options: options, file: file, size: info.Size()}, nil
}

func (w *Writer) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(data)) > w.options.MaxBytes {
		if err := w.file.Close(); err != nil {
			return 0, err
		}
		w.file = nil
		if err := rotateBackups(w.path, w.options); err != nil {
			return 0, err
		}
		file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return 0, err
		}
		w.file, w.size = file, 0
	}
	n, err := w.file.Write(data)
	w.size += int64(n)
	return n, err
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

type Options struct {
	MaxBytes int64
	Backups  int
	MaxAge   time.Duration
	Compress bool
}

func Rotate(path string, options Options, now time.Time) error {
	if options.MaxBytes < 1 || options.Backups < 1 {
		return fmt.Errorf("invalid log rotation limits")
	}
	if err := removeExpired(path, options, now); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) || (err == nil && info.Size() < options.MaxBytes) {
		return nil
	}
	if err != nil {
		return err
	}
	return rotateBackups(path, options)
}

func rotateBackups(path string, options Options) error {
	var err error
	extension := ""
	if options.Compress {
		extension = ".gz"
	}
	_ = os.Remove(fmt.Sprintf("%s.%d%s", path, options.Backups, extension))
	for index := options.Backups - 1; index >= 1; index-- {
		oldPath := fmt.Sprintf("%s.%d%s", path, index, extension)
		newPath := fmt.Sprintf("%s.%d%s", path, index+1, extension)
		if err = os.Rename(oldPath, newPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if !options.Compress {
		return os.Rename(path, path+".1")
	}
	temporary := path + ".rotate"
	if err = os.Rename(path, temporary); err != nil {
		return err
	}
	if err = compressFile(temporary, path+".1.gz"); err != nil {
		_ = os.Rename(temporary, path)
		return err
	}
	return os.Remove(temporary)
}

func removeExpired(path string, options Options, now time.Time) error {
	if options.MaxAge <= 0 {
		return nil
	}
	matches, err := filepath.Glob(path + ".*")
	if err != nil {
		return err
	}
	for _, candidate := range matches {
		info, statErr := os.Stat(candidate)
		if statErr == nil && now.Sub(info.ModTime()) > options.MaxAge {
			if removeErr := os.Remove(candidate); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
		}
	}
	return nil
}

func compressFile(source, destination string) (result error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := input.Close(); result == nil {
			result = closeErr
		}
	}()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := output.Close(); result == nil {
			result = closeErr
		}
		if result != nil {
			_ = os.Remove(destination)
		}
	}()
	writer := gzip.NewWriter(output)
	if _, err = io.Copy(writer, input); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}
