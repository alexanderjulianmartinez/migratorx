package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileState persists checkpoints and values to a JSON file.
type FileState struct {
	path string
	mu   sync.Mutex
	data map[string]interface{}
}

// NewFileState loads or creates state at the given path.
func NewFileState(path string) (*FileState, error) {
	if path == "" {
		return nil, fmt.Errorf("state path is required")
	}
	fs := &FileState{path: path, data: map[string]interface{}{}}
	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

func (s *FileState) Get(key string) (interface{}, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *FileState) Set(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	_ = s.persist()
}

func (s *FileState) MarkCompleted(stepName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[completedKey(stepName)] = true
	_ = s.persist()
}

func (s *FileState) IsCompleted(stepName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[completedKey(stepName)]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func (s *FileState) load() error {
	if _, err := os.Stat(s.path); err != nil {
		if os.IsNotExist(err) {
			return s.persist()
		}
		return err
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, &s.data)
}

func (s *FileState) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

func completedKey(step string) string {
	return fmt.Sprintf("workflow:%s:completed", step)
}
