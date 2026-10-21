// Package update performs optional, cached stable-release checks.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/whiteducksoftware/azctx/internal/storage"
	"github.com/whiteducksoftware/azctx/internal/version"
)

const interval = 24 * time.Hour

// Checker exposes network, clock, terminal and cache boundaries for hermetic use.
// Zero values use production defaults.
type Checker struct {
	Client     *http.Client
	URL        string
	Current    string
	CacheDir   string
	AzureDir   string
	Now        func() time.Time
	IsTerminal func() bool
	Output     io.Writer
}

type cachedRelease struct {
	CheckedAt time.Time `json:"checkedAt"`
	Tag       string    `json:"tag"`
}

// Check uses the process terminal and standard per-user cache location.
func Check(owner, repo string) error {
	return (Checker{}).Check(owner, repo)
}

func (c Checker) enabled() bool {
	tty := c.IsTerminal
	if tty == nil {
		tty = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
	}
	_, ci := os.LookupEnv("CI")
	return tty() && !ci && os.Getenv("AZCTX_NO_UPDATE_CHECK") != "1"
}

func (c Checker) cachePath() (string, error) {
	if c.CacheDir == "" {
		var err error
		c.CacheDir, err = os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("resolve update cache: %w", err)
		}
	}
	return storage.Path(c.CacheDir, c.AzureDir, "update.json")
}

// Check returns diagnostics for debug logging only; callers must not fail a
// successful subscription switch because an optional update check failed.
func (c Checker) Check(owner, repo string) error {
	if !c.enabled() {
		return nil
	}
	name, err := c.cachePath()
	if err != nil {
		return err
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	release, acquired, err := storage.Lock(name+".lock", c.AzureDir)
	if err != nil || !acquired {
		return err
	}
	defer release()
	cached, err := readCache(name)
	if err != nil {
		return err
	}
	if !cached.CheckedAt.IsZero() && now.Sub(cached.CheckedAt) < interval {
		return c.notice(owner, repo, cached.Tag)
	}
	// Cache attempts too, so offline/proxy failures do not recur every run.
	cached = cachedRelease{CheckedAt: now}
	if err := save(name, c.AzureDir, cached); err != nil {
		return err
	}
	tag, err := c.latest(owner, repo)
	if err != nil {
		return err
	}
	cached.Tag = tag
	if err := save(name, c.AzureDir, cached); err != nil {
		return err
	}
	return c.notice(owner, repo, tag)
}

func readCache(name string) (cachedRelease, error) {
	data, err := os.ReadFile(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return cachedRelease{}, fmt.Errorf("read update cache: %w", err)
	}
	var cached cachedRelease
	if json.Unmarshal(data, &cached) != nil {
		return cachedRelease{}, nil
	}
	return cached, nil
}

func save(name, azureDir string, cached cachedRelease) error {
	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	return storage.Write(name, azureDir, data)
}

func (c Checker) latest(owner, repo string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	endpoint := c.URL
	if endpoint == "" {
		endpoint = "https://github.com/" + owner + "/" + repo + "/releases/latest"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %w", err)
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to check for updates: GitHub returned %s", response.Status)
	}
	if !strings.Contains(response.Request.URL.Path, "/releases/tag/") {
		return "", nil
	}
	tag := path.Base(response.Request.URL.Path)
	if _, prerelease, valid := releaseParts(tag); !valid || prerelease {
		return "", nil
	}
	return tag, nil
}

func (c Checker) notice(owner, repo, tag string) error {
	current := c.Current
	if current == "" {
		current = version.Get().Version
	}
	output := c.Output
	if output == nil {
		output = os.Stderr
	}
	if newerRelease(current, tag) {
		releaseURL := "https://github.com/" + owner + "/" + repo + "/releases/tag/" + url.PathEscape(tag)
		if _, err := fmt.Fprintf(output, "A new version of azctx is available: %s → %s\n%s\n", current, tag, releaseURL); err != nil {
			return fmt.Errorf("failed to print update notice: %w", err)
		}
	}
	return nil
}

// Compare CalVer components numerically; prerelease update tags are ignored.
func newerRelease(current, latest string) bool {
	currentParts, currentPrerelease, currentOK := releaseParts(current)
	latestParts, latestPrerelease, latestOK := releaseParts(latest)
	if !currentOK || !latestOK || latestPrerelease {
		return false
	}
	comparison := slices.Compare(latestParts, currentParts)
	return comparison > 0 || comparison == 0 && currentPrerelease
}

func releaseParts(tag string) ([]int, bool, bool) {
	tag, _, _ = strings.Cut(strings.TrimPrefix(tag, "v"), "+")
	base, _, prerelease := strings.Cut(tag, "-")
	parts := strings.Split(base, ".")
	if len(parts) > 3 {
		return nil, false, false
	}
	numbers := make([]int, 3)
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return nil, false, false
		}
		numbers[index] = value
	}
	return numbers, prerelease, true
}
