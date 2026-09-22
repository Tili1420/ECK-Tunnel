package systools

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	// ForeignMirrors are tried first.
	ForeignMirrors = []string{
		"http://archive.ubuntu.com/ubuntu",
		"http://de.archive.ubuntu.com/ubuntu",
		"http://nl.archive.ubuntu.com/ubuntu",
		"http://fr.archive.ubuntu.com/ubuntu",
		"http://mirror.hetzner.com/ubuntu/packages",
	}
	// IranMirrors are only measured when no foreign mirror is fast enough.
	IranMirrors = []string{
		"http://mirror.arvancloud.ir/ubuntu",
		"http://ir.archive.ubuntu.com/ubuntu",
		"http://mirror.aminidc.com/ubuntu",
		"http://repo.iut.ac.ir/ubuntu",
		"http://mirror.faraso.org/ubuntu",
		"http://mirror.iranserver.com/ubuntu",
		"http://ubuntu.hostiran.ir/ubuntu",
		"http://mirrors.pardisco.co/ubuntu",
	}
)

// MirrorGoodEnough is the Release-file fetch time under which the foreign
// mirrors are accepted without also measuring the Iranian ones.
const MirrorGoodEnough = 1500 * time.Millisecond

// OSInfo is what the mirror tool needs from /etc/os-release.
type OSInfo struct {
	ID       string // "ubuntu"
	Codename string // "jammy", "noble", ...
}

// ReadOSInfo parses /etc/os-release.
func ReadOSInfo() (OSInfo, error) {
	f, err := os.Open(p("/etc/os-release"))
	if err != nil {
		return OSInfo{}, err
	}
	defer f.Close()
	var info OSInfo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			info.ID = v
		case "VERSION_CODENAME":
			info.Codename = v
		case "UBUNTU_CODENAME":
			if info.Codename == "" {
				info.Codename = v
			}
		}
	}
	return info, sc.Err()
}

// The two places Ubuntu keeps its own archive list. 24.04 and later ship the
// deb822 file and leave sources.list as a comment; 22.04 and earlier use the
// one-line sources.list. Writing the deb822 file on 22.04 — which is what the
// tool this replaces did — leaves the original list in force as well, so apt
// then reads both.
const (
	deb822Sources = "/etc/apt/sources.list.d/ubuntu.sources"
	legacySources = "/etc/apt/sources.list"
)

// sourcesTarget returns the file that holds Ubuntu's own archive list here and
// whether it is the deb822 format.
func sourcesTarget() (string, bool) {
	if _, err := os.Stat(p(deb822Sources)); err == nil {
		return p(deb822Sources), true
	}
	return p(legacySources), false
}

func renderDeb822(mirror, code string) string {
	return fmt.Sprintf(`# Set by ECK-Tunnel (sudo eck → Optimize → Best apt mirror).
Types: deb
URIs: %[1]s/
Suites: %[2]s %[2]s-updates %[2]s-backports
Components: main restricted universe multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

Types: deb
URIs: %[1]s/
Suites: %[2]s-security
Components: main restricted universe multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
`, mirror, code)
}

func renderLegacy(mirror, code string) string {
	const comps = "main restricted universe multiverse"
	var b strings.Builder
	b.WriteString("# Set by ECK-Tunnel (sudo eck → Optimize → Best apt mirror).\n")
	for _, suite := range []string{code, code + "-updates", code + "-backports", code + "-security"} {
		fmt.Fprintf(&b, "deb %s/ %s %s\n", mirror, suite, comps)
	}
	return b.String()
}

// MirrorResult is one mirror's measurement.
type MirrorResult struct {
	URL  string
	Took time.Duration
	Err  error
}

var mirrorClient = &http.Client{Timeout: 5 * time.Second}

// measureMirror fetches the suite's Release file and checks it really is that
// suite's: a captive page or an empty mirror answers 200 too.
func measureMirror(url, code string) MirrorResult {
	res := MirrorResult{URL: url}
	start := time.Now()
	resp, err := mirrorClient.Get(url + "/dists/" + code + "/Release")
	if err != nil {
		res.Err = err
		return res
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		res.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		return res
	}
	head, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		res.Err = err
		return res
	}
	if !strings.Contains(string(head), "Codename: "+code) {
		res.Err = errors.New("not an Ubuntu " + code + " Release file")
		return res
	}
	res.Took = time.Since(start)
	return res
}

// MeasureMirrors measures every mirror in parallel, working ones first,
// fastest first.
func MeasureMirrors(urls []string, code string) []MirrorResult {
	out := make([]MirrorResult, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = measureMirror(u, code)
		}()
	}
	wg.Wait()
	sortMirrors(out)
	return out
}

func sortMirrors(r []MirrorResult) {
	sort.SliceStable(r, func(i, j int) bool {
		if (r[i].Err == nil) != (r[j].Err == nil) {
			return r[i].Err == nil
		}
		return r[i].Took < r[j].Took
	})
}

// BestMirror measures the foreign mirrors and, only if none is fast enough,
// the Iranian ones too. It returns every result, best first.
func BestMirror(code string) []MirrorResult {
	res := MeasureMirrors(ForeignMirrors, code)
	if len(res) > 0 && res[0].Err == nil && res[0].Took <= MirrorGoodEnough {
		return res
	}
	res = append(res, MeasureMirrors(IranMirrors, code)...)
	sortMirrors(res)
	return res
}

// CheckMirrorSupport says why this machine cannot use the tool, or nil. The
// mirrors above carry amd64 and i386 only; other architectures are served from
// ports.ubuntu.com, which none of them mirror.
func CheckMirrorSupport() (OSInfo, error) {
	info, err := ReadOSInfo()
	if err != nil {
		return info, fmt.Errorf("cannot read /etc/os-release: %w", err)
	}
	if info.ID != "ubuntu" {
		return info, fmt.Errorf("this tool is for Ubuntu; this server is %q", info.ID)
	}
	if info.Codename == "" {
		return info, errors.New("could not tell which Ubuntu release this is")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "386" {
		return info, fmt.Errorf("%s servers use ports.ubuntu.com, which these mirrors do not carry", runtime.GOARCH)
	}
	return info, nil
}

func mirrorBackupPath(target string) string {
	return stateDir() + "/apt-" + filepath.Base(target) + ".orig"
}

// SetMirror points Ubuntu's own archive list at mirror, in whichever format
// this release uses. The original file is backed up once.
func SetMirror(mirror, code string) (string, error) {
	target, deb822 := sourcesTarget()
	backup := mirrorBackupPath(target)
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		data, err := os.ReadFile(target)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err := os.MkdirAll(stateDir(), 0o700); err != nil {
			return "", err
		}
		if err := writeFileAtomic(backup, data, 0o600); err != nil {
			return "", fmt.Errorf("backing up %s: %w", target, err)
		}
	}
	body := renderLegacy(mirror, code)
	if deb822 {
		body = renderDeb822(mirror, code)
	}
	if err := writeFileAtomic(target, []byte(body), 0o644); err != nil {
		return "", err
	}
	return target, nil
}

// RestoreMirror puts the original archive list back.
func RestoreMirror() (string, error) {
	target, _ := sourcesTarget()
	backup := mirrorBackupPath(target)
	data, err := os.ReadFile(backup)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.New("no backup found — the mirror was never changed by ECK-Tunnel")
	}
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(target, data, 0o644); err != nil {
		return "", err
	}
	os.Remove(backup)
	return target, nil
}
