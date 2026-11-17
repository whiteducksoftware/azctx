package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/storage"
)

type previousState struct {
	PreviousSubscriptionID string   `json:"previousSubscriptionId"`
	RecentSubscriptionIDs  []string `json:"recentSubscriptionIds,omitempty"`
}

func path(azureDir string) (string, error) {
	return storage.ConfigPath(azureDir, "state.json")
}

func read(azureDir string) (previousState, error) {
	name, err := path(azureDir)
	if err != nil {
		return previousState{}, err
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return previousState{}, fmt.Errorf("read subscription state: %w", err)
	}
	return decode(data)
}

func decode(data []byte) (previousState, error) {
	var value previousState
	if err := json.Unmarshal(data, &value); err != nil {
		return previousState{}, fmt.Errorf("read subscription state: %w", err)
	}
	if value.PreviousSubscriptionID != "" {
		if err := profile.ValidateGUID(value.PreviousSubscriptionID); err != nil {
			return previousState{}, fmt.Errorf("invalid previous subscription: %w", err)
		}
	}
	for _, id := range value.RecentSubscriptionIDs {
		if err := profile.ValidateGUID(id); err != nil {
			return previousState{}, fmt.Errorf("invalid recent subscription: %w", err)
		}
	}
	return value, nil
}

// Recent reads most-recent-first IDs; legacy and missing state have no recency.
func Recent(azureDir string) ([]string, error) {
	value, err := read(azureDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return value.RecentSubscriptionIDs, err
}

// Load reads the previous subscription ID from azctx's own configuration.
func Load(azureDir string) (string, error) {
	value, err := read(azureDir)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("no previous subscription found: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("read previous subscription: %w", err)
	}
	if value.PreviousSubscriptionID == "" {
		return "", fmt.Errorf("no previous subscription found: %w", os.ErrNotExist)
	}
	if err := profile.ValidateGUID(value.PreviousSubscriptionID); err != nil {
		return "", fmt.Errorf("invalid previous subscription: %w", err)
	}
	return value.PreviousSubscriptionID, nil
}

// Record saves the current ID before a switch and returns a rollback for a
// failed Azure CLI invocation. A successful switch must not call the rollback.
func Record(azureDir, id string) (func() error, error) {
	if err := profile.ValidateGUID(id); err != nil {
		return nil, err
	}
	return record(azureDir, func(value *previousState) {
		value.PreviousSubscriptionID = id
	})
}

// Save stores the default subscription ID before switching away from it.
func Save(azureDir, id string) error {
	_, err := Record(azureDir, id)
	return err
}

// RecordSelection prepares previous and MRU IDs before az runs, with rollback.
// Empty previous leaves an existing previous ID intact (e.g. no active default).
func RecordSelection(azureDir, previous, target string) (func() error, error) {
	if err := profile.ValidateGUID(target); err != nil {
		return nil, err
	}
	if previous != "" {
		if err := profile.ValidateGUID(previous); err != nil {
			return nil, err
		}
	}
	return record(azureDir, func(value *previousState) {
		if previous != "" {
			value.PreviousSubscriptionID = previous
		}
		recent := []string{target}
		for _, id := range value.RecentSubscriptionIDs {
			if !strings.EqualFold(id, target) {
				recent = append(recent, id)
			}
		}
		value.RecentSubscriptionIDs = recent
	})
}

func record(azureDir string, mutate func(*previousState)) (func() error, error) {
	name, err := path(azureDir)
	if err != nil {
		return nil, err
	}
	before, readErr := os.ReadFile(name)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, fmt.Errorf("read subscription state before switch: %w", readErr)
	}
	var value previousState
	if readErr == nil {
		value, err = decode(before)
		if err != nil {
			return nil, err
		}
	}
	mutate(&value)
	if err := write(azureDir, value); err != nil {
		return nil, err
	}
	return func() error {
		if errors.Is(readErr, os.ErrNotExist) {
			return storage.Remove(name, azureDir)
		}
		return storage.Write(name, azureDir, before)
	}, nil
}

func write(azureDir string, value previousState) error {
	name, err := path(azureDir)
	if err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return storage.Write(name, azureDir, data)
}
