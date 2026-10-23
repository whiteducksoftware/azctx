// Package history keeps a bounded, identity-only audit of completed switches.
package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/whiteducksoftware/azctx/internal/profile"
	"github.com/whiteducksoftware/azctx/internal/storage"
)

const (
	Limit    = 100
	maxBytes = 1 << 20
)

var ErrCorrupt = errors.New("corrupt history ignored")

type Identity struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenantId"`
	Environment string `json:"environment"`
}

type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Previous  *Identity `json:"previous"`
	New       Identity  `json:"new"`
	Source    string    `json:"source"`
}

// Log is also the stable command JSON envelope; Entries is never null.
type Log struct {
	SchemaVersion int     `json:"schemaVersion"`
	Entries       []Entry `json:"entries"`
}

func empty() Log { return Log{SchemaVersion: 1, Entries: []Entry{}} }

// Load is read-only, does not require a profile, and ignores no errors silently.
func Load(azureDir string) (Log, error) {
	path, err := storage.ConfigPath(azureDir, "history.json")
	if err != nil {
		return empty(), err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty(), nil
	}
	if err != nil {
		return empty(), err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return empty(), err
	}
	if info.Size() > maxBytes {
		return empty(), ErrCorrupt
	}
	return decode(file)
}

func decode(reader io.Reader) (Log, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var value Log
	if err := decoder.Decode(&value); err != nil {
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			return empty(), err
		}
		return empty(), ErrCorrupt
	}
	if value.SchemaVersion != 1 || value.Entries == nil || len(value.Entries) > Limit {
		return empty(), ErrCorrupt
	}
	if decoder.Decode(new(any)) != io.EOF {
		return empty(), ErrCorrupt
	}
	for _, entry := range value.Entries {
		if err := validate(entry); err != nil {
			return empty(), ErrCorrupt
		}
	}
	return value, nil
}

// Record is best-effort after a completed switch. A corrupt log is replaced;
// warning is returned separately so callers can report recovery even on success.
func Record(azureDir string, entry Entry) (warning, err error) {
	if err := validate(entry); err != nil {
		return nil, err
	}
	lockPath, err := storage.ConfigPath(azureDir, "history.lock")
	if err != nil {
		return nil, err
	}
	release, acquired, err := storage.Lock(lockPath, azureDir)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, errors.New("history is busy; switch was not recorded")
	}
	defer release()
	value, err := Load(azureDir)
	if errors.Is(err, ErrCorrupt) {
		warning = err
	} else if err != nil {
		return nil, err
	}
	entry.Timestamp = entry.Timestamp.UTC()
	value.Entries = append([]Entry{entry}, value.Entries...)
	if len(value.Entries) > Limit {
		value.Entries = value.Entries[:Limit]
	}
	var data bytes.Buffer
	if err := json.NewEncoder(&data).Encode(value); err != nil {
		return warning, err
	}
	if data.Len() > maxBytes {
		return warning, errors.New("history exceeds size limit")
	}
	path, err := storage.ConfigPath(azureDir, "history.json")
	if err != nil {
		return warning, err
	}
	return warning, storage.Write(path, azureDir, data.Bytes())
}

func validate(entry Entry) error {
	if entry.Timestamp.IsZero() || (entry.Source != "tty" && entry.Source != "script") {
		return errors.New("invalid history entry")
	}
	for _, identity := range []*Identity{entry.Previous, &entry.New} {
		if identity == nil {
			continue
		}
		if profile.ValidateGUID(identity.ID) != nil || profile.ValidateGUID(identity.TenantID) != nil {
			return errors.New("invalid history identity")
		}
	}
	return nil
}
