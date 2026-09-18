package artifactcatalog

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	packagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+$`)
	kindPattern    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,39}$`)
	sessionPattern = regexp.MustCompile(`^\d{8}_\d{6}(?:_\d+)?$`)
)

const maxScannedSessions = 5000
const maxFilesPerSession = 500

type Session struct {
	Package  string    `json:"package"`
	Kind     string    `json:"kind"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Modified time.Time `json:"modified"`
	Files    []File    `json:"files"`
}

type File struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type Catalog struct{ root string }

func New(root string) *Catalog { return &Catalog{root: filepath.Clean(root)} }

func (c *Catalog) List(packageName, kind string, limit int) ([]Session, error) {
	if packageName != "" && !packagePattern.MatchString(packageName) {
		return nil, errors.New("invalid package filter")
	}
	if kind != "" && !kindPattern.MatchString(kind) {
		return nil, errors.New("invalid artifact kind filter")
	}
	if limit < 1 || limit > 200 {
		return nil, errors.New("limit must be between 1 and 200")
	}
	packages, err := c.childDirectories(c.root)
	if os.IsNotExist(err) {
		return []Session{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]Session, 0)
	scanned := 0
scan:
	for _, pkg := range packages {
		if !packagePattern.MatchString(pkg) || packageName != "" && pkg != packageName {
			continue
		}
		kinds, readErr := c.childDirectories(filepath.Join(c.root, pkg))
		if readErr != nil {
			continue
		}
		for _, artifactKind := range kinds {
			if !kindPattern.MatchString(artifactKind) || kind != "" && artifactKind != kind {
				continue
			}
			sessions, sessionErr := c.childDirectories(filepath.Join(c.root, pkg, artifactKind))
			if sessionErr != nil {
				continue
			}
			for _, name := range sessions {
				if scanned >= maxScannedSessions {
					break scan
				}
				if !sessionPattern.MatchString(name) {
					continue
				}
				scanned++
				path := filepath.Join(c.root, pkg, artifactKind, name)
				info, statErr := os.Lstat(path)
				if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
					continue
				}
				files, filesErr := listFiles(path)
				if filesErr != nil {
					continue
				}
				result = append(result, Session{Package: pkg, Kind: artifactKind, Name: name, Path: filepath.ToSlash(path), Modified: info.ModTime().UTC(), Files: files})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Modified.After(result[j].Modified) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (c *Catalog) childDirectories(parent string) ([]string, error) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && !strings.Contains(entry.Name(), string(filepath.Separator)) {
			result = append(result, entry.Name())
		}
	}
	return result, nil
}

func listFiles(directory string) ([]File, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	result := make([]File, 0, len(entries))
	for _, entry := range entries {
		if len(result) >= maxFilesPerSession {
			break
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		result = append(result, File{Name: entry.Name(), Path: filepath.ToSlash(path), Size: info.Size()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
