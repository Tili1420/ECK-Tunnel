package manage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withMirrorFile(t *testing.T) {
	t.Helper()
	old := mirrorFile
	mirrorFile = filepath.Join(t.TempDir(), "update-mirror")
	t.Cleanup(func() { mirrorFile = old })
	t.Setenv("ECK_MIRROR", "")
}

func TestSetUpdateMirrorValidatesAndTrims(t *testing.T) {
	withMirrorFile(t)
	for _, bad := range []string{"ftp://x", "mirror.example", "https://"} {
		if err := SetUpdateMirror(bad); err == nil {
			t.Errorf("SetUpdateMirror(%q) accepted", bad)
		}
	}
	if err := SetUpdateMirror("  https://dl.example.com/eck/  "); err != nil {
		t.Fatal(err)
	}
	if got := UpdateMirror(); got != "https://dl.example.com/eck" {
		t.Fatalf("UpdateMirror = %q", got)
	}
	if err := SetUpdateMirror(""); err != nil {
		t.Fatal(err)
	}
	if got := UpdateMirror(); got != "" {
		t.Fatalf("mirror not removed: %q", got)
	}
}

func TestEnvMirrorOverridesFile(t *testing.T) {
	withMirrorFile(t)
	if err := SetUpdateMirror("https://file.example"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ECK_MIRROR", "http://env.example/")
	if got := UpdateMirror(); got != "http://env.example" {
		t.Fatalf("UpdateMirror = %q, want the environment's", got)
	}
}

// The mirror is the last resort, after every way of reaching GitHub.
func TestReleaseTriesPutsTheMirrorLast(t *testing.T) {
	withMirrorFile(t)
	gh := "https://github.com/o/r/releases/download/v1/SHA256SUMS"

	if tries := releaseTries(gh, "v1/SHA256SUMS", time.Second); len(tries) == 0 || strings.HasPrefix(tries[len(tries)-1].name, "mirror") {
		t.Fatal("a mirror try appeared with no mirror configured")
	}

	if err := SetUpdateMirror("https://dl.example.com/eck"); err != nil {
		t.Fatal(err)
	}
	tries := releaseTries(gh, "v1/SHA256SUMS", time.Second)
	last := tries[len(tries)-1]
	if last.url != "https://dl.example.com/eck/v1/SHA256SUMS" {
		t.Fatalf("mirror url = %q", last.url)
	}
	if tries[0].url != gh {
		t.Fatalf("first try = %q, want GitHub", tries[0].url)
	}
	// The GitHub API has no mirror equivalent.
	for _, tr := range releaseTries("https://api.github.com/x", "", time.Second) {
		if strings.HasPrefix(tr.name, "mirror") {
			t.Fatal("mirror offered for a file it does not carry")
		}
	}
}
