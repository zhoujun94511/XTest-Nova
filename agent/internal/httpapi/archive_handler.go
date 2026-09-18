package httpapi

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/zhoujun94511/xtest-nova/agent/internal/files"
)

const (
	maxArchiveEntries = 10000
	maxArchiveBytes   = int64(2 << 30)
	maxArchiveDepth   = 32
)

type archiveEntry struct {
	path      string
	name      string
	size      int64
	directory bool
}

func (a *API) archiveDirectory(w http.ResponseWriter, r *http.Request) {
	root := "/" + r.PathValue("path")
	entries, total, err := a.archiveEntries(r.Context(), root)
	if err != nil {
		fail(w, httpStatusForFile(err), err)
		return
	}
	base := path.Base(strings.TrimSuffix(root, "/"))
	if base == "." || base == "/" || base == "" {
		base = "device-files"
	}
	temporaryPath, archiveSize, err := a.buildArchive(r.Context(), base, entries)
	if err != nil {
		fail(w, httpStatusForFile(err), err)
		return
	}
	defer func() { _ = os.Remove(temporaryPath) }()
	archive, err := os.Open(temporaryPath)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	defer func() { _ = archive.Close() }()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(base+".zip"))
	w.Header().Set("X-XTest-Archive-Entries", fmt.Sprint(len(entries)))
	w.Header().Set("X-XTest-Archive-Bytes", fmt.Sprint(total))
	w.Header().Set("Content-Length", fmt.Sprint(archiveSize))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, archive)
}

func (a *API) buildArchive(ctx context.Context, base string, entries []archiveEntry) (temporaryPath string, size int64, err error) {
	temporary, err := os.CreateTemp("", "xtest-nova-archive-*.zip")
	if err != nil {
		return "", 0, err
	}
	temporaryPath = temporary.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := temporary.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
		}
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		return "", 0, err
	}
	writer := zip.NewWriter(temporary)
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			_ = writer.Close()
			return "", 0, err
		}
		name := path.Join(base, entry.name)
		if entry.directory {
			if _, err = writer.Create(name + "/"); err != nil {
				_ = writer.Close()
				return "", 0, err
			}
			continue
		}
		entryWriter, createErr := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if createErr != nil {
			_ = writer.Close()
			return "", 0, createErr
		}
		file, _, openErr := a.files.Open(entry.path)
		if openErr != nil {
			_ = writer.Close()
			return "", 0, openErr
		}
		copied, copyErr := io.Copy(entryWriter, io.LimitReader(file, entry.size+1))
		closeErr := file.Close()
		if copyErr != nil {
			_ = writer.Close()
			return "", 0, copyErr
		}
		if closeErr != nil {
			_ = writer.Close()
			return "", 0, closeErr
		}
		if copied != entry.size {
			_ = writer.Close()
			return "", 0, fmt.Errorf("archive source size changed for %s", entry.path)
		}
	}
	if err = writer.Close(); err != nil {
		return "", 0, err
	}
	if err = temporary.Sync(); err != nil {
		return "", 0, err
	}
	info, err := temporary.Stat()
	if err != nil {
		return "", 0, err
	}
	size = info.Size()
	if err = temporary.Close(); err != nil {
		return "", 0, err
	}
	closed = true
	return temporaryPath, size, nil
}

func (a *API) archiveEntries(ctx context.Context, root string) ([]archiveEntry, int64, error) {
	rootEntry, err := a.files.Info(root)
	if err != nil {
		return nil, 0, err
	}
	if !rootEntry.IsDirectory {
		return nil, 0, errors.New("archive path is not a directory")
	}
	entries := make([]archiveEntry, 0)
	var total int64
	var walk func(files.Entry, int) error
	walk = func(directory files.Entry, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > maxArchiveDepth {
			return fmt.Errorf("archive exceeds %d directory levels", maxArchiveDepth)
		}
		for _, child := range directory.Files {
			if len(entries) >= maxArchiveEntries {
				return fmt.Errorf("archive exceeds %d entries", maxArchiveEntries)
			}
			relative := strings.TrimPrefix(strings.TrimPrefix(child.Path, root), "/")
			entries = append(entries, archiveEntry{path: child.Path, name: relative, size: child.Size, directory: child.IsDirectory})
			if child.IsDirectory {
				resolved, infoErr := a.files.Info(child.Path)
				if infoErr != nil {
					return infoErr
				}
				if err := walk(resolved, depth+1); err != nil {
					return err
				}
				continue
			}
			if child.Size < 0 || total > maxArchiveBytes-child.Size {
				return fmt.Errorf("archive exceeds %d bytes", maxArchiveBytes)
			}
			total += child.Size
			if total > maxArchiveBytes {
				return fmt.Errorf("archive exceeds %d bytes", maxArchiveBytes)
			}
		}
		return nil
	}
	if err := walk(rootEntry, 0); err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}
