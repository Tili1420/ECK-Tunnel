package app

import (
	"os"
	"regexp"
	"testing"
)

// install.sh verifies release signatures itself, before this binary exists on
// the machine, so it carries its own copy of the release key. The two copies
// must be the same key or the installer would refuse every genuine release.
func TestInstallerPinsTheSameReleaseKey(t *testing.T) {
	b, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Skip("install.sh not beside the source: " + err.Error())
	}
	m := regexp.MustCompile(`(?m)^RELEASE_PUBKEY="([^"]+)"`).FindSubmatch(b)
	if m == nil {
		t.Fatal("install.sh has no RELEASE_PUBKEY line")
	}
	if string(m[1]) != ReleasePublicKey {
		t.Fatalf("install.sh pins %s, the binary pins %s", m[1], ReleasePublicKey)
	}
}
