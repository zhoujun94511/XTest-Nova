package files

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Entry struct {
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	IsDirectory bool    `json:"isDirectory"`
	Size        int64   `json:"size,omitempty"`
	Mode        string  `json:"mode,omitempty"`
	Files       []Entry `json:"files,omitempty"`
}
type Service struct{ roots []string }

const maxUploadSize int64 = 512 << 20

func New(roots ...string) *Service {
	clean := make([]string, 0, len(roots))
	for _, root := range roots {
		clean = append(clean, filepath.Clean(root))
	}
	return &Service{roots: clean}
}
func (s *Service) Resolve(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("path required")
	}
	clean := filepath.Clean("/" + strings.TrimPrefix(value, "/"))
	for _, root := range s.roots {
		rel, err := filepath.Rel(root, clean)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return clean, nil
		}
	}
	return "", fmt.Errorf("path is outside allowed roots")
}
func (s *Service) Info(value string) (Entry, error) {
	path, err := s.Resolve(value)
	if err != nil {
		return Entry{}, err
	}
	if err = s.ensureExistingPath(path); err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Entry{}, err
	}
	result := Entry{Name: info.Name(), Path: path, IsDirectory: info.IsDir(), Size: info.Size(), Mode: info.Mode().String()}
	if info.IsDir() {
		children, err := os.ReadDir(path)
		if err != nil {
			return Entry{}, err
		}
		result.Files = make([]Entry, 0, len(children))
		for _, child := range children {
			item, statErr := child.Info()
			if statErr == nil {
				result.Files = append(result.Files, Entry{Name: child.Name(), Path: filepath.Join(path, child.Name()), IsDirectory: child.IsDir(), Size: item.Size(), Mode: item.Mode().String()})
			}
		}
	}
	return result, nil
}
func (s *Service) Open(value string) (*os.File, fs.FileInfo, error) {
	path, err := s.Resolve(value)
	if err != nil {
		return nil, nil, err
	}
	if err = s.ensureExistingPath(path); err != nil {
		return nil, nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("path is a directory")
	}
	return file, info, nil
}
func (s *Service) Save(value string, source io.Reader, mode fs.FileMode) (Entry, error) {
	return s.save(value, source, mode, maxUploadSize)
}

// ResolveForWrite validates the nearest existing parent before creating any
// missing directories, then verifies the resulting parent again.
func (s *Service) ResolveForWrite(value string) (string, error) {
	path, err := s.Resolve(value)
	if err != nil {
		return "", err
	}
	if err = s.prepareParent(path); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) save(value string, source io.Reader, mode fs.FileMode, limit int64) (Entry, error) {
	path, err := s.ResolveForWrite(value)
	if err != nil {
		return Entry{}, err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".nova-upload-*")
	if err != nil {
		return Entry{}, err
	}
	name := temp.Name()
	defer func() { _ = os.Remove(name) }()
	written, copyErr := io.Copy(temp, io.LimitReader(source, limit+1))
	if copyErr != nil {
		_ = temp.Close()
		return Entry{}, copyErr
	}
	if written > limit {
		_ = temp.Close()
		return Entry{}, fmt.Errorf("upload exceeds %d byte limit", limit)
	}
	if err = temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return Entry{}, err
	}
	if err = temp.Close(); err != nil {
		return Entry{}, err
	}
	if err = os.Rename(name, path); err != nil {
		return Entry{}, err
	}
	return s.Info(path)
}

func (s *Service) prepareParent(path string) error {
	parent := filepath.Dir(path)
	existing := parent
	for {
		_, err := os.Lstat(existing)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		next := filepath.Dir(existing)
		if next == existing {
			return fmt.Errorf("no existing parent for path")
		}
		existing = next
	}
	if err := s.ensureExistingPath(existing); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	return s.ensureExistingPath(parent)
}

func (s *Service) ensureExistingPath(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	for _, root := range s.roots {
		resolvedRoot, rootErr := filepath.EvalSymlinks(root)
		if rootErr != nil {
			continue
		}
		rel, relErr := filepath.Rel(resolvedRoot, resolved)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("resolved path is outside allowed roots")
}
