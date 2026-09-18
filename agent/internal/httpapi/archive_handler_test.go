package httpapi

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	fileservice "github.com/zhoujun94511/xtest-nova/agent/internal/files"
)

type archiveTestFS struct {
	entries map[string]fileservice.Entry
	local   map[string]string
}

func (f archiveTestFS) Info(path string) (fileservice.Entry, error) {
	entry, ok := f.entries[path]
	if !ok {
		return fileservice.Entry{}, os.ErrNotExist
	}
	return entry, nil
}
func (f archiveTestFS) Open(path string) (*os.File, fs.FileInfo, error) {
	file, err := os.Open(f.local[path])
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	return file, info, err
}
func (archiveTestFS) Save(string, io.Reader, fs.FileMode) (fileservice.Entry, error) {
	return fileservice.Entry{}, nil
}
func (archiveTestFS) Resolve(path string) (string, error)         { return path, nil }
func (archiveTestFS) ResolveForWrite(path string) (string, error) { return path, nil }

func TestArchiveDirectoryStreamsSelectedTree(t *testing.T) {
	local := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(local, []byte("nova"), 0600); err != nil {
		t.Fatal(err)
	}
	filesystem := archiveTestFS{
		entries: map[string]fileservice.Entry{
			"/sdcard/xtest-nova":     {Name: "xtest-nova", Path: "/sdcard/xtest-nova", IsDirectory: true, Files: []fileservice.Entry{{Name: "run", Path: "/sdcard/xtest-nova/run", IsDirectory: true}}},
			"/sdcard/xtest-nova/run": {Name: "run", Path: "/sdcard/xtest-nova/run", IsDirectory: true, Files: []fileservice.Entry{{Name: "report.txt", Path: "/sdcard/xtest-nova/run/report.txt", Size: 4}}},
		},
		local: map[string]string{"/sdcard/xtest-nova/run/report.txt": local},
	}
	api := &API{files: filesystem}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /archive/{path...}", api.archiveDirectory)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/archive/sdcard/xtest-nova", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 2 || reader.File[1].Name != "xtest-nova/run/report.txt" {
		t.Fatalf("unexpected archive entries: %+v", reader.File)
	}
}

func TestArchiveDirectoryBuildFailureReturnsErrorBeforeSuccessHeaders(t *testing.T) {
	filesystem := archiveTestFS{
		entries: map[string]fileservice.Entry{
			"/sdcard/xtest-nova": {Name: "xtest-nova", Path: "/sdcard/xtest-nova", IsDirectory: true, Files: []fileservice.Entry{{Name: "missing.txt", Path: "/sdcard/xtest-nova/missing.txt", Size: 4}}},
		},
		local: map[string]string{"/sdcard/xtest-nova/missing.txt": filepath.Join(t.TempDir(), "missing.txt")},
	}
	api := &API{files: filesystem}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /archive/{path...}", api.archiveDirectory)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/archive/sdcard/xtest-nova", nil))
	if response.Code == http.StatusOK {
		t.Fatalf("archive build failure returned success: headers=%v body=%q", response.Header(), response.Body.String())
	}
	if response.Header().Get("Content-Type") == "application/zip" {
		t.Fatalf("archive headers were sent before build completed: %v", response.Header())
	}
}
