package manage

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/eck-tunnel/eck/internal/app"
)

// A self-hosted release mirror, so updates do not depend on GitHub.
//
// GitHub stays the first place the updater looks. When it cannot be reached —
// blocked, rate limited, or gone — the updater falls back to a plain HTTP(S)
// folder the operator controls: a directory on one of their own servers, an
// object-storage bucket, anything that serves static files. The layout is the
// one scripts/make-release.sh writes:
//
//	<mirror>/VERSION                     the latest stable tag, e.g. v1.9.0
//	<mirror>/install.sh                  the installer
//	<mirror>/<tag>/eck_linux_<arch>.tar.gz
//	<mirror>/<tag>/SHA256SUMS
//	<mirror>/<tag>/SHA256SUMS.sig
//
// Trusting the mirror is not required. Every archive is still checked against
// SHA256SUMS, and SHA256SUMS against the Ed25519 signature made with the key
// pinned in this binary (app.ReleasePublicKey). A mirror — or anyone between
// it and the server — that changed a file cannot produce a signature for it.

// mirrorFile holds the mirror's base URL, one line. A var so tests can move it.
var mirrorFile = app.ConfigDir + "/update-mirror"

// UpdateMirror returns the configured mirror base URL without a trailing
// slash, or "" when none is set. ECK_MIRROR in the environment overrides the
// file, which is how the installer hands its mirror to a first run.
func UpdateMirror() string {
	if v := strings.TrimSpace(os.Getenv("ECK_MIRROR")); v != "" {
		return strings.TrimRight(v, "/")
	}
	b, err := os.ReadFile(mirrorFile)
	if err != nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(string(b)), "/")
}

// SetUpdateMirror stores the mirror URL; an empty string removes it.
func SetUpdateMirror(raw string) error {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		if err := os.Remove(mirrorFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%q is not an http:// or https:// address", raw)
	}
	if err := os.MkdirAll(app.ConfigDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(mirrorFile, []byte(raw+"\n"), 0o644)
}

// fetchTry is one place to fetch a release file from: a name for the log, the
// client to use and the full URL.
type fetchTry struct {
	name   string
	client *http.Client
	url    string
}

// releaseTries lists where a release file can be fetched, in order: GitHub
// directly, GitHub through the tunnel relay, then the mirror. githubURL is the
// file on GitHub; mirrorPath is the same file relative to the mirror base.
func releaseTries(githubURL, mirrorPath string, timeout time.Duration) []fetchTry {
	var out []fetchTry
	for _, s := range sources(timeout) {
		out = append(out, fetchTry{name: s.name, client: s.client, url: githubURL})
	}
	if m := UpdateMirror(); m != "" && mirrorPath != "" {
		out = append(out, fetchTry{
			name:   "mirror " + m,
			client: &http.Client{Timeout: timeout},
			url:    m + "/" + strings.TrimLeft(mirrorPath, "/"),
		})
	}
	return out
}
