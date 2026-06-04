// Package update implements a GitHub Releases-based self-update check for the
// GUI binary. It queries the latest published release, compares it against the
// compiled-in version, and (when newer) downloads and atomically replaces the
// running executable via minio/selfupdate.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/minio/selfupdate"

	"discord-szx/internal/config"
	"discord-szx/internal/version"
)

// AssetName is the release asset matched and downloaded for self-update. It
// must stay in sync with the artifact name produced by the release workflow.
const AssetName = "d2p.exe"

const releasesLatestURL = "https://api.github.com/repos/%s/%s/releases/latest"

// Release describes the subset of a GitHub release that we care about.
type Release struct {
	// Version is the release version without a leading "v" (e.g. "0.2.0").
	Version string
	// Tag is the raw git tag (e.g. "v0.2.0").
	Tag string
	// HTMLURL is the human-facing release page.
	HTMLURL string
	// AssetURL is the direct download URL for AssetName.
	AssetURL string
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckLatest fetches the latest release and reports whether it is newer than
// the running build. The boolean is true only when a strictly newer release
// with a usable asset is available.
func CheckLatest(ctx context.Context) (*Release, bool, error) {
	url := fmt.Sprintf(releasesLatestURL, config.RepoOwner, config.RepoName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", config.AppName)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitHub API ответил %s", resp.Status)
	}

	var gr ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, false, err
	}
	// Only stable releases are offered. The /releases/latest endpoint never
	// returns drafts or pre-releases by design; the Draft/Prerelease guard is
	// a defensive check, not a disabled beta channel. Supporting beta channels
	// would require switching to GET /releases and channel-aware filtering.
	if gr.Draft || gr.Prerelease || strings.TrimSpace(gr.TagName) == "" {
		return nil, false, nil
	}

	rel := &Release{
		Tag:     gr.TagName,
		Version: strings.TrimPrefix(gr.TagName, "v"),
		HTMLURL: gr.HTMLURL,
	}
	for _, a := range gr.Assets {
		if strings.EqualFold(a.Name, AssetName) {
			rel.AssetURL = a.BrowserDownloadURL
			break
		}
	}
	if rel.AssetURL == "" {
		// A release exists but lacks the binary we know how to apply.
		return rel, false, nil
	}

	if !version.IsNewer(config.Version, rel.Version) {
		return rel, false, nil
	}
	return rel, true, nil
}

// Apply downloads the release asset and atomically replaces the running
// executable. The replacement is rolled back automatically on failure.
func Apply(ctx context.Context, rel *Release) error {
	if rel == nil || rel.AssetURL == "" {
		return fmt.Errorf("нет ассета для обновления")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.AssetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", config.AppName)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("скачивание вернуло %s", resp.Status)
	}

	if err := selfupdate.Apply(resp.Body, selfupdate.Options{}); err != nil {
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			return fmt.Errorf("обновление не удалось и откат не сработал: %v (откат: %v)", err, rerr)
		}
		return fmt.Errorf("обновление не удалось: %w", err)
	}
	return nil
}

// Restart launches the (freshly replaced) executable and exits the current
// process. It should be called only after a successful Apply.
func Restart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	proc, err := os.StartProcess(exe, []string{exe}, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return err
	}
	_ = proc.Release()
	os.Exit(0)
	return nil
}
