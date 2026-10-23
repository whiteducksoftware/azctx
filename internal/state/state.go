package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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

// ErrChanged reports that a rollback found state committed by another writer.
// The newer state is kept instead of being overwritten with older bytes.
var ErrChanged = errors.New("subscription state changed by another process; not restored")

var errReleased = errors.New("subscription state lock already released")

const lockRetry = 50 * time.Millisecond

// lockWait paces attempts on the nonblocking retained lock; tests replace it.
var lockWait = func(ctx context.Context) error {
	timer := time.NewTimer(lockRetry)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Transaction holds the retained private state.lock across a complete switch,
// so read-modify-write, Azure CLI and rollback cannot interleave with another
// azctx switch. The lock file is never unlinked; the OS releases it on exit.
type Transaction struct {
	azureDir string
	release  func()
}

// Begin waits for the state lock until it is acquired or ctx is canceled.
func Begin(ctx context.Context, azureDir string) (*Transaction, error) {
	name, err := storage.ConfigPath(azureDir, "state.lock")
	if err != nil {
		return nil, err
	}
	release, err := storage.WaitLock(ctx, name, azureDir, lockWait)
	if err != nil {
		return nil, fmt.Errorf("lock subscription state: %w", err)
	}
	return &Transaction{azureDir: azureDir, release: release}, nil
}

// Release unlocks the state; it is safe to call more than once.
func (tx *Transaction) Release() {
	if tx.release != nil {
		tx.release()
		tx.release = nil
	}
}

// Record saves the current ID before a switch and returns a rollback for a
// failed Azure CLI invocation. A successful switch must not call the rollback.
func (tx *Transaction) Record(id string) (func() error, error) {
	mutate, err := previousUpdate(id)
	if err != nil {
		return nil, err
	}
	return tx.record(mutate)
}

// RecordSelection prepares previous and MRU IDs before az runs, with rollback.
// Empty previous leaves an existing previous ID intact (e.g. no active default).
func (tx *Transaction) RecordSelection(previous, target string) (func() error, error) {
	mutate, err := selectionUpdate(previous, target)
	if err != nil {
		return nil, err
	}
	return tx.record(mutate)
}

func (tx *Transaction) record(mutate func(*previousState)) (func() error, error) {
	if tx.release == nil {
		return nil, errReleased
	}
	restore, err := record(tx.azureDir, mutate)
	if err != nil {
		return nil, err
	}
	return func() error {
		if tx.release == nil {
			return errReleased
		}
		return restore()
	}, nil
}

// Record locks the state for one previous-ID update; its rollback locks again.
func Record(azureDir, id string) (func() error, error) {
	mutate, err := previousUpdate(id)
	if err != nil {
		return nil, err
	}
	return locked(azureDir, mutate)
}

// Save stores the default subscription ID before switching away from it.
func Save(azureDir, id string) error {
	_, err := Record(azureDir, id)
	return err
}

// RecordSelection locks the state for one update; its rollback locks again.
// Switches use Begin instead to keep the lock across Azure CLI and rollback.
func RecordSelection(azureDir, previous, target string) (func() error, error) {
	mutate, err := selectionUpdate(previous, target)
	if err != nil {
		return nil, err
	}
	return locked(azureDir, mutate)
}

func previousUpdate(id string) (func(*previousState), error) {
	if err := profile.ValidateGUID(id); err != nil {
		return nil, err
	}
	return func(value *previousState) { value.PreviousSubscriptionID = id }, nil
}

func selectionUpdate(previous, target string) (func(*previousState), error) {
	if err := profile.ValidateGUID(target); err != nil {
		return nil, err
	}
	if previous != "" {
		if err := profile.ValidateGUID(previous); err != nil {
			return nil, err
		}
	}
	return func(value *previousState) {
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
	}, nil
}

func locked(azureDir string, mutate func(*previousState)) (func() error, error) {
	tx, err := Begin(context.Background(), azureDir)
	if err != nil {
		return nil, err
	}
	defer tx.Release()
	restore, err := record(azureDir, mutate)
	if err != nil {
		return nil, err
	}
	return func() error {
		tx, err := Begin(context.Background(), azureDir)
		if err != nil {
			return err
		}
		defer tx.Release()
		return restore()
	}, nil
}

// record writes mutated state and returns a restore of the exact prior bytes
// (or absence) that applies only while the file still holds this write.
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
	after, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if err := storage.Write(name, azureDir, after); err != nil {
		return nil, err
	}
	return func() error {
		current, err := os.ReadFile(name)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read subscription state before restore: %w", err)
		}
		if err != nil || !bytes.Equal(current, after) {
			return ErrChanged
		}
		if errors.Is(readErr, os.ErrNotExist) {
			return storage.Remove(name, azureDir)
		}
		return storage.Write(name, azureDir, before)
	}, nil
}
