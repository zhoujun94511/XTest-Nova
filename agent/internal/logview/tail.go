package logview

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

type Reader struct{ paths map[string]string }

type Result struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"`
}

func New(paths map[string]string) *Reader {
	cloned := make(map[string]string, len(paths))
	for name, path := range paths {
		cloned[name] = path
	}
	return &Reader{paths: cloned}
}

func (r *Reader) Tail(name string, limit int, contains string) (Result, error) {
	path, ok := r.paths[name]
	if !ok {
		return Result{}, errors.New("unknown log name")
	}
	if limit < 1 || limit > 2000 {
		return Result{}, errors.New("lines must be between 1 and 2000")
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return Result{Name: name, Path: path, Lines: []string{}}, nil
	}
	if err != nil {
		return Result{}, err
	}
	window := make([]string, 0, limit)
	total := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if contains != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(contains)) {
			continue
		}
		total++
		if len(window) == limit {
			copy(window, window[1:])
			window[len(window)-1] = line
		} else {
			window = append(window, line)
		}
	}
	scanErr := scanner.Err()
	closeErr := file.Close()
	if scanErr != nil {
		return Result{}, scanErr
	}
	if closeErr != nil {
		return Result{}, closeErr
	}
	return Result{Name: name, Path: path, Lines: window, Truncated: total > len(window)}, nil
}
