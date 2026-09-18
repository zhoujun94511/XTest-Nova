package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic replaces path only after the complete content is durable in
// a temporary file in the same directory.
func WriteFileAtomic(path string, content []byte, mode os.FileMode) (err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
		}
		_ = os.Remove(temporary)
	}()
	if err = file.Chmod(mode); err != nil {
		return err
	}
	if _, err = file.Write(content); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	closed = true
	return os.Rename(temporary, path)
}

// DirectoryTransaction builds a directory beside its final destination and
// publishes it with one rename after all files are complete.
type DirectoryTransaction struct {
	Final   string
	Staging string
	reserve string
}

func BeginDirectory(final string) (*DirectoryTransaction, error) {
	parent := filepath.Dir(final)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(final); err == nil {
		return nil, os.ErrExist
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(final)+".staging-*")
	if err != nil {
		return nil, err
	}
	return &DirectoryTransaction{Final: final, Staging: staging}, nil
}

func (t *DirectoryTransaction) Commit() error {
	if t == nil || t.Staging == "" {
		return fmt.Errorf("directory transaction is not active")
	}
	if t.reserve != "" {
		if err := os.Remove(t.reserve); err != nil {
			return err
		}
		t.reserve = ""
	}
	if err := os.Rename(t.Staging, t.Final); err != nil {
		return err
	}
	t.Staging = ""
	return nil
}

func (t *DirectoryTransaction) Abort() error {
	if t == nil || t.Staging == "" {
		if t != nil && t.reserve != "" {
			err := os.Remove(t.reserve)
			t.reserve = ""
			return err
		}
		return nil
	}
	err := os.RemoveAll(t.Staging)
	t.Staging = ""
	if t.reserve != "" {
		if reserveErr := os.Remove(t.reserve); err == nil && reserveErr != nil {
			err = reserveErr
		}
	}
	t.reserve = ""
	return err
}
