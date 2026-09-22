package systools

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withRoot points every path at a fresh temporary tree for one test.
func withRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := root
	root = dir
	t.Cleanup(func() { root = old })
	for _, d := range []string{"/etc/apt/sources.list.d", "/run/systemd/resolve"} {
		if err := os.MkdirAll(p(d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(p(path), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(p(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReadOSInfo(t *testing.T) {
	withRoot(t)
	write(t, "/etc/os-release", "NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_CODENAME=jammy\nUBUNTU_CODENAME=jammy\n")
	info, err := ReadOSInfo()
	if err != nil || info.ID != "ubuntu" || info.Codename != "jammy" {
		t.Fatalf("got %+v, %v", info, err)
	}
}

// On 22.04 there is no deb822 file: the one-line sources.list must be the one
// rewritten, never a new ubuntu.sources beside it.
func TestSetMirrorJammyUsesSourcesList(t *testing.T) {
	withRoot(t)
	orig := "deb http://archive.ubuntu.com/ubuntu jammy main\n"
	write(t, legacySources, orig)

	target, err := SetMirror("http://mirror.example/ubuntu", "jammy")
	if err != nil {
		t.Fatal(err)
	}
	if target != p(legacySources) {
		t.Fatalf("wrote %s, want sources.list", target)
	}
	if _, err := os.Stat(p(deb822Sources)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created ubuntu.sources on a release that does not use it")
	}
	got := read(t, legacySources)
	for _, want := range []string{
		"deb http://mirror.example/ubuntu/ jammy main restricted universe multiverse",
		"deb http://mirror.example/ubuntu/ jammy-security main restricted universe multiverse",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("sources.list missing %q:\n%s", want, got)
		}
	}

	// A second change must not overwrite the backup with ECK-Tunnel's own file.
	if _, err := SetMirror("http://other.example/ubuntu", "jammy"); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreMirror(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, legacySources); got != orig {
		t.Fatalf("restore gave %q, want the original %q", got, orig)
	}
}

func TestSetMirrorNobleUsesDeb822(t *testing.T) {
	withRoot(t)
	write(t, legacySources, "# moved to ubuntu.sources\n")
	write(t, deb822Sources, "Types: deb\nURIs: http://archive.ubuntu.com/ubuntu/\n")

	target, err := SetMirror("http://mirror.example/ubuntu", "noble")
	if err != nil {
		t.Fatal(err)
	}
	if target != p(deb822Sources) {
		t.Fatalf("wrote %s, want ubuntu.sources", target)
	}
	got := read(t, deb822Sources)
	if !strings.Contains(got, "Suites: noble noble-updates noble-backports") || !strings.Contains(got, "URIs: http://mirror.example/ubuntu/") {
		t.Fatalf("unexpected deb822 file:\n%s", got)
	}
	if read(t, legacySources) != "# moved to ubuntu.sources\n" {
		t.Fatal("sources.list was touched on a deb822 release")
	}
}

func TestRestoreMirrorWithoutBackup(t *testing.T) {
	withRoot(t)
	write(t, legacySources, "x\n")
	if _, err := RestoreMirror(); err == nil {
		t.Fatal("restore without a backup should fail, not wipe the file")
	}
}

func TestMeasureMirrorChecksTheReleaseFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good/dists/jammy/Release":
			w.Write([]byte("Origin: Ubuntu\nCodename: jammy\n"))
		case "/captive/dists/jammy/Release":
			w.Write([]byte("<html>login</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res := MeasureMirrors([]string{srv.URL + "/missing", srv.URL + "/captive", srv.URL + "/good"}, "jammy")
	if res[0].URL != srv.URL+"/good" || res[0].Err != nil {
		t.Fatalf("best = %+v, want the good mirror", res[0])
	}
	if res[1].Err == nil || res[2].Err == nil {
		t.Fatalf("captive and missing mirrors must fail: %+v", res[1:])
	}
}

func fakeDNS(t *testing.T, answers map[string][]net.IP) {
	t.Helper()
	old := queryFunc
	queryFunc = func(_ context.Context, server, _ string) ([]net.IP, error) {
		ips, ok := answers[server]
		if !ok {
			return nil, errors.New("timeout")
		}
		return ips, nil
	}
	t.Cleanup(func() { queryFunc = old })
}

func TestMeasureDNSRejectsTheFilterAddress(t *testing.T) {
	fakeDNS(t, map[string][]net.IP{
		"1.1.1.1": {net.ParseIP("104.16.1.1")},
		"2.2.2.2": {net.ParseIP("10.10.34.35")},
	})
	res := MeasureDNS([]Resolver{{"2.2.2.2", "liar"}, {"3.3.3.3", "dead"}, {"1.1.1.1", "honest"}})
	if res[0].IP != "1.1.1.1" || res[0].Err != nil {
		t.Fatalf("best = %+v, want the honest resolver", res[0])
	}
	for _, r := range res[1:] {
		if r.Err == nil {
			t.Fatalf("%s should have failed", r.IP)
		}
	}
}

func TestSetAndRestoreDNSKeepsTheSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	withRoot(t)
	write(t, "/run/systemd/resolve/stub-resolv.conf", "nameserver 127.0.0.53\n")
	link := "../run/systemd/resolve/stub-resolv.conf"
	if err := os.Symlink(link, resolvPath()); err != nil {
		t.Fatal(err)
	}
	restarted := false
	old := restartResolved
	restartResolved = func() { restarted = true }
	t.Cleanup(func() { restartResolved = old })

	if err := SetDNS("1.1.1.1", "8.8.8.8"); err != nil {
		t.Fatal(err)
	}
	if err := SetDNS("9.9.9.9"); err != nil { // must not replace the backup
		t.Fatal(err)
	}
	if got := read(t, "/etc/resolv.conf"); !strings.Contains(got, "nameserver 9.9.9.9") {
		t.Fatalf("resolv.conf = %q", got)
	}
	if _, err := RestoreDNS(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(resolvPath()); err != nil || got != link {
		t.Fatalf("restored link = %q, %v; want %q", got, err, link)
	}
	if !restarted {
		t.Fatal("systemd-resolved was not restarted after restoring its link")
	}
}

func TestSetAndRestoreDNSPlainFile(t *testing.T) {
	withRoot(t)
	orig := "nameserver 4.2.2.4\n"
	write(t, "/etc/resolv.conf", orig)
	if err := SetDNS("1.1.1.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreDNS(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, "/etc/resolv.conf"); got != orig {
		t.Fatalf("restored %q, want %q", got, orig)
	}
	if _, err := os.Stat(filepath.Join(stateDir(), "resolv.conf.backup.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("backup left behind after a restore")
	}
}

func TestSetDNSRejectsNonIP(t *testing.T) {
	withRoot(t)
	if err := SetDNS("dns.example"); err == nil {
		t.Fatal("a hostname must be refused: resolv.conf takes addresses only")
	}
}

func TestRestoreDNSRemovesAFileThatWasNotThere(t *testing.T) {
	withRoot(t)
	if err := SetDNS("1.1.1.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreDNS(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(resolvPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("restore left a resolv.conf where there was none")
	}
}
