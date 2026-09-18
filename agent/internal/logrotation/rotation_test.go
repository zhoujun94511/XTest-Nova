package logrotation

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotateCompressesAndRetainsBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	options := Options{MaxBytes: 4, Backups: 2, MaxAge: 24 * time.Hour, Compress: true}
	for _, content := range []string{"first", "second", "third"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Rotate(path, options, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(path + ".3.gz"); !os.IsNotExist(err) {
		t.Fatalf("unexpected third backup: %v", err)
	}
	file, err := os.Open(path + ".1.gz")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	_ = file.Close()
	if string(data) != "third" {
		t.Fatalf("latest backup = %q", data)
	}
}

func TestRotateRejectsInvalidLimits(t *testing.T) {
	if err := Rotate("unused", Options{}, time.Now()); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestWriterRotatesWhileRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	writer, err := NewWriter(path, Options{MaxBytes: 8, Backups: 2, Compress: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write([]byte("123456")); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path + ".1.gz"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abcdef" {
		t.Fatalf("active=%q", data)
	}
}
