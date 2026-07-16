package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// RuntimeState contains scheduler bookkeeping, not user preferences. Keeping
// it outside IdleTrigger.toml prevents normal evaluation from rewriting the
// annotated user configuration.
type RuntimeState struct {
	LastOccurrences map[string]string `json:"last_occurrences,omitempty"`
}

func LoadRuntimeState(path string) (RuntimeState, error) {
	state := RuntimeState{LastOccurrences: make(map[string]string)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read automation state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return RuntimeState{LastOccurrences: make(map[string]string)}, fmt.Errorf("parse automation state: %w", err)
	}
	if state.LastOccurrences == nil {
		state.LastOccurrences = make(map[string]string)
	}
	return state, nil
}

func SaveRuntimeState(path string, state RuntimeState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode automation state: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".IdleTrigger-state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create automation state: %w", err)
	}
	temporaryPath := temporary.Name()
	ok := false
	defer func() {
		_ = temporary.Close()
		if !ok {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write automation state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync automation state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close automation state: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace automation state: %w", err)
	}
	ok = true
	return nil
}
