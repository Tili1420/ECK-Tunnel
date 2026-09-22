package app

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
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

// The installer checks signatures with the Python verifier embedded in it, so
// it works on Ubuntu releases whose OpenSSL cannot (1.1.1 has no -rawin). Run
// that exact code against a signature made here, and against a tampered file.
func TestInstallerPythonVerifier(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	b, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Skip("install.sh not beside the source")
	}
	m := regexp.MustCompile(`(?s)<<'PY'\n(.*?)\nPY\n`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no embedded Python verifier in install.sh")
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()
	msg := []byte("abc  eck_linux_amd64.tar.gz\n")
	os.WriteFile(filepath.Join(dir, "SHA256SUMS"), msg, 0o644)
	os.WriteFile(filepath.Join(dir, "SHA256SUMS.sig"), []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))+"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "bad"), []byte("evil eck_linux_amd64.tar.gz\n"), 0o644)
	run := func(file string) int {
		cmd := exec.Command(py, "-", filepath.Join(dir, file), filepath.Join(dir, "SHA256SUMS.sig"), base64.StdEncoding.EncodeToString(pub))
		cmd.Stdin = bytes.NewReader(m[1])
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			t.Fatal(err)
		}
		return 0
	}
	if rc := run("SHA256SUMS"); rc != 0 {
		t.Fatalf("genuine signature rejected (rc %d)", rc)
	}
	if rc := run("bad"); rc != 1 {
		t.Fatalf("tampered file accepted (rc %d)", rc)
	}
}
