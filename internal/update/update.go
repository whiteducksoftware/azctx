package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/whiteducksoftware/azctx/internal/version"
)

// Check retains the synchronous, uncached post-switch check until layer 06.
func Check(owner, repo string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+owner+"/"+repo+"/releases/latest", nil)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to check for updates: GitHub returned %s", response.Status)
	}
	var release struct {
		TagName string `json:"tag_name"` //nolint:tagliatelle // GitHub's wire format uses snake_case.
		HTMLURL string `json:"html_url"` //nolint:tagliatelle // GitHub's wire format uses snake_case.
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}
	current := version.Get().Version
	if newerRelease(current, release.TagName) {
		if _, err := fmt.Fprintf(os.Stderr, "A new version of azctx is available: %s → %s\n%s\n", current, release.TagName, release.HTMLURL); err != nil {
			return fmt.Errorf("failed to print update notice: %w", err)
		}
	}
	return nil
}

// GitHub's latest-release endpoint returns stable releases. Compare numeric
// release components; a prerelease of the same version precedes that release.
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
