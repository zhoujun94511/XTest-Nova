package artifacts

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSessionIsUniqueAndPruned(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 4, 12, 34, 56, 0, time.Local)
	first, err := NewSession(root, "com.example.app", "Monkey", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSession(root, "com.example.app", "Monkey", now)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Base(second) != "20260904_123456_2" {
		t.Fatalf("unexpected sessions: %s %s", first, second)
	}
	if err = PruneSessions(filepath.Dir(first), 1, second); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("old session was not pruned: %v", err)
	}
}

func TestPruneSessionsKeepsRequestedOldestWithoutExceedingLimit(t *testing.T) {
	root := t.TempDir()
	base := PackageKind(root, "com.example.app", "Monkey")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	oldest := filepath.Join(base, "20260904_010000")
	for _, name := range []string{"20260904_010000", "20260904_020000", "20260904_030000"} {
		if err := os.Mkdir(filepath.Join(base, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := PruneSessions(base, 1, oldest); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(oldest) {
		t.Fatalf("unexpected retained sessions: %+v", entries)
	}
}

func TestPruneSessionsPreservesForeignDirectories(t *testing.T) {
	root := t.TempDir()
	base := PackageKind(root, "com.example.app", "Monkey")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manual-investigation", "20260904_010000", "20260904_020000_2"} {
		if err := os.Mkdir(filepath.Join(base, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := PruneSessions(base, 1, filepath.Join(base, "20260904_020000_2")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "manual-investigation")); err != nil {
		t.Fatalf("foreign directory was pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "20260904_010000")); !os.IsNotExist(err) {
		t.Fatalf("old session was not pruned: %v", err)
	}
}

func TestDirectoryTransactionPublishesOnlyOnCommitAndCleansRollback(t *testing.T) {
	root := t.TempDir()
	final := filepath.Join(root, "session")
	transaction, err := BeginDirectory(final)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(final); !os.IsNotExist(err) {
		t.Fatalf("final directory became visible before commit: %v", err)
	}
	if err = WriteFileAtomic(filepath.Join(transaction.Staging, "report.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staging := transaction.Staging
	if err = transaction.Abort(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("rollback left staging directory: %v", err)
	}
	if _, err = os.Stat(final); !os.IsNotExist(err) {
		t.Fatalf("rollback published final directory: %v", err)
	}

	transaction, err = BeginDirectory(final)
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteFileAtomic(filepath.Join(transaction.Staging, "report.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(final, "report.json")); err != nil {
		t.Fatalf("committed file unavailable: %v", err)
	}
}

func TestSessionTransactionKeepsTimestampNamingAndCleansReservation(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 4, 12, 34, 56, 0, time.Local)
	first, err := NewSessionTransaction(root, "com.example.app", "Monkey", now)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first.Final) != "20260904_123456" {
		t.Fatalf("final path = %s", first.Final)
	}
	reservation := first.Final + ".reserve"
	if err = first.Abort(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(reservation); !os.IsNotExist(err) {
		t.Fatalf("rollback left reservation: %v", err)
	}

	first, err = NewSessionTransaction(root, "com.example.app", "Monkey", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Commit(); err != nil {
		t.Fatal(err)
	}
	second, err := NewSessionTransaction(root, "com.example.app", "Monkey", now)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Abort() }()
	if filepath.Base(second.Final) != "20260904_123456_2" {
		t.Fatalf("second final path = %s", second.Final)
	}
}
