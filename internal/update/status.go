package update

import (
	"fmt"
	"time"
)

// Status is a read-only, offline update snapshot. Stale results are not a claim
// about the current latest release.
type Status struct {
	State         string `json:"state"`
	LatestVersion string `json:"latestVersion"`
	Stale         bool   `json:"stale"`
}

// CachedStatus never checks the network, creates a cache, or acquires a lock.
func (c Checker) CachedStatus() (Status, error) {
	status := Status{State: "not_checked"}
	name, err := c.cachePath()
	if err != nil {
		return status, err
	}
	cached, err := readCache(name, true)
	if err != nil {
		return Status{State: "unavailable"}, err
	}
	if cached.CheckedAt.IsZero() {
		return status, nil
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	status.Stale = now.Sub(cached.CheckedAt) >= interval || cached.CheckedAt.After(now)
	parts, prerelease, valid := releaseParts(cached.Tag)
	if !valid || prerelease {
		status.State = "check_failed"
		return status, nil
	}
	// Project only numeric release components; never echo arbitrary cache data.
	status.LatestVersion = fmt.Sprintf("%d.%d.%d", parts[0], parts[1], parts[2])
	current := c.Current
	if _, _, valid := releaseParts(current); !valid {
		status.State = "unknown_version"
	} else if newerRelease(current, status.LatestVersion) {
		status.State = "available"
	} else {
		status.State = "current"
	}
	return status, nil
}
