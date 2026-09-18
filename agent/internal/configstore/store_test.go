package configstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestReplacePersistsAndSupportsConcurrentReaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	var readers sync.WaitGroup
	for i := 0; i < 16; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for j := 0; j < 100; j++ {
				_ = store.Get()
			}
		}()
	}
	for i := 0; i < 20; i++ {
		if err := store.Replace(map[string]any{"revision": i, "label": fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	readers.Wait()
	reloadedStore, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := reloadedStore.Get()
	if reloaded["label"] != "19" {
		t.Fatalf("unexpected persisted value: %#v", reloaded)
	}
}

func TestNewReportsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"broken":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("New accepted malformed JSON")
	}
}

func TestNewReportsReadErrors(t *testing.T) {
	path := t.TempDir()
	if _, err := New(path); err == nil {
		t.Fatal("New accepted a directory as a config file")
	} else if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("New reported directory read error as not-exist: %v", err)
	}
}
