package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/storage"
)

type previousState struct {
	PreviousSubscriptionID string `json:"previousSubscriptionId"`
}

func path(azureDir string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve azctx state: %w", err)
	}
	return storage.Path(base, azureDir, "state.json")
}

// Load reads the previous subscription ID from azctx's own configuration.
func Load(azureDir string) (string, error) {
	name, err := path(azureDir)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("no previous subscription found: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("read previous subscription: %w", err)
	}
	var state previousState
	if err := json.Unmarshal(data, &state); err != nil {
		return "", fmt.Errorf("read previous subscription: %w", err)
	}
	if err := profile.ValidateGUID(state.PreviousSubscriptionID); err != nil {
		return "", fmt.Errorf("invalid previous subscription: %w", err)
	}
	return state.PreviousSubscriptionID, nil
}

// Record saves the current ID before a switch and returns a rollback for a
// failed Azure CLI invocation. A successful switch must not call the rollback.
func Record(azureDir, id string) (func() error, error) {
	name, err := path(azureDir)
	if err != nil {
		return nil, err
	}
	previous, readErr := os.ReadFile(name)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, fmt.Errorf("read previous state before switch: %w", readErr)
	}
	if err := Save(azureDir, id); err != nil {
		return nil, err
	}
	return func() error {
		if errors.Is(readErr, os.ErrNotExist) {
			return storage.Remove(name, azureDir)
		}
		return storage.Write(name, azureDir, previous)
	}, nil
}

// Save stores the default subscription ID before switching away from it.
func Save(azureDir, id string) error {
	if err := profile.ValidateGUID(id); err != nil {
		return err
	}
	name, err := path(azureDir)
	if err != nil {
		return err
	}
	data, err := json.Marshal(previousState{PreviousSubscriptionID: id})
	if err != nil {
		return err
	}
	return storage.Write(name, azureDir, data)
}
