package version

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const releasesURL = "https://api.github.com/repos/ydakilux/projectswitcher/releases/latest"

// CheckLatest queries the GitHub "latest release" API and returns the
// latest published version string (without the leading "v"), or "" if
// unavailable, disabled, or not newer than the current build.
//
// It never blocks for more than a few seconds, and it swallows all errors
// silently — this is a best-effort, non-critical background check. Set
// PW_NO_UPDATE_CHECK=1 to disable it entirely.
func CheckLatest() string {
	if os.Getenv("PW_NO_UPDATE_CHECK") == "1" {
		return ""
	}

	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet, releasesURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pw-projectswitcher")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}

	latest := strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
	if latest == "" {
		return ""
	}

	if compareSemver(latest, Version) <= 0 {
		return ""
	}

	return latest
}

// compareSemver compares two semver-like version strings ("1.2.3",
// optionally with a "-pre.release" suffix). It returns -1, 0, or 1 as a < b,
// a == b, or a > b. Pre-release versions are treated as lower than the
// corresponding release (e.g. "1.2.3-rc1" < "1.2.3"), and malformed/missing
// numeric parts are treated as 0.
func compareSemver(a, b string) int {
	aCore, aPre := splitPrerelease(a)
	bCore, bPre := splitPrerelease(b)

	aParts := semverParts(aCore)
	bParts := semverParts(bCore)

	for i := 0; i < 3; i++ {
		if aParts[i] != bParts[i] {
			if aParts[i] < bParts[i] {
				return -1
			}
			return 1
		}
	}

	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "" && bPre != "":
		return 1
	case aPre != "" && bPre == "":
		return -1
	default:
		return strings.Compare(aPre, bPre)
	}
}

func splitPrerelease(v string) (core, pre string) {
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func semverParts(core string) [3]int {
	var parts [3]int
	segs := strings.SplitN(core, ".", 3)
	for i := 0; i < len(segs) && i < 3; i++ {
		n, err := strconv.Atoi(segs[i])
		if err != nil {
			n = 0
		}
		parts[i] = n
	}
	return parts
}
