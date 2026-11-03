package update

import (
	"fmt"
	"os"

	"go.szostok.io/version"
	"go.szostok.io/version/term"
	"go.szostok.io/version/upgrade"
)

func Check(owner, repo string) error {
	// Check the current version and print a ghDetector if a new version is available
	ghDetector, currentVersion := upgrade.NewGitHubDetector(owner, repo), version.Get().Version
	updateCheckInfo, err := ghDetector.LookForLatestRelease(upgrade.LookForLatestReleaseInput{CurrentVersion: currentVersion})
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	if updateCheckInfo.Found {
		out, err := ghDetector.Render(updateCheckInfo.ReleaseInfo, term.IsSmart(os.Stderr))
		if err != nil {
			return fmt.Errorf("failed to render update notice: %w", err)
		}

		_, err = fmt.Fprint(os.Stderr, out)
		if err != nil {
			return fmt.Errorf("failed to print update notice: %w", err)
		}
		_, _ = os.Stdout.Write([]byte("\n"))
	}

	return nil
}
