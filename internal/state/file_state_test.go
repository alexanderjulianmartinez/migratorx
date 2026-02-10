package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileState_GetSetPersistence(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")

	fs, err := NewFileState(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fs.Set("k", "v")

	if v, ok := fs.Get("k"); !ok || v.(string) != "v" {
		t.Fatalf("expected key to be set")
	}

	fs2, err := NewFileState(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := fs2.Get("k"); !ok || v.(string) != "v" {
		t.Fatalf("expected persisted value")
	}
}

func TestFileState_MarkCompletedPersistence(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "state.json")

	fs, err := NewFileState(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fs.MarkCompleted("step1")

	if !fs.IsCompleted("step1") {
		t.Fatalf("expected step1 to be completed")
	}

	fs2, err := NewFileState(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fs2.IsCompleted("step1") {
		t.Fatalf("expected completed step to persist")
	}
}

func TestFileState_CreatesDirectories(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "state.json")

	fs, err := NewFileState(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fs.Set("k", "v")

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}
}