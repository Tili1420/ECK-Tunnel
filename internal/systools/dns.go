package systools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// Resolver is one DNS server a user can pick.
type Resolver struct {
	IP   string
	Name string
}

var (
	// ForeignDNS are tried first: they answer honestly everywhere they reach.
	ForeignDNS = []Resolver{
		{"1.1.1.1", "cloudflare"}, {"1.0.0.1", "cloudflare"},
		{"8.8.8.8", "google"}, {"8.8.4.4", "google"},
		{"9.9.9.9", "quad9"},
	}
	// IranDNS are only measured when no foreign resolver is fast enough.
	IranDNS = []Resolver{
		{"178.22.122.100", "shecan"}, {"185.51.200.2", "shecan"},
		{"10.202.10.11", "radar"}, {"10.202.10.10", "radar"},
		{"78.157.42.100", "electro"}, {"78.157.42.101", "electro"},
		{"185.55.226.26", "begzar"}, {"185.55.225.25", "begzar"},
	}
)

// DNSGoodEnough is the median answer time under which the foreign resolvers
// are accepted without also measuring the Iranian ones.
const DNSGoodEnough = 150 * time.Millisecond

// dnsProbeNames are looked up in turn. Real names the server will need anyway,
// so the answer is one the resolver is likely to have warm, as it would be in
// use.
var dnsProbeNames = []string{"archive.ubuntu.com", "github.com", "www.google.com"}

// censoredNet is where Iran's filter points a hijacked name. A resolver whose
// answer lands here is reachable but lying, and must not win on speed.
var _, censoredNet, _ = net.ParseCIDR("10.10.34.0/24")

// DNSResult is one resolver's measurement. Err is set when it failed; Median
// is then meaningless.
type DNSResult struct {
	Resolver
	Median time.Duration
	Err    error
}

// queryFunc resolves name against server; replaced in tests.
var queryFunc = func(ctx context.Context, server, name string) ([]net.IP, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
		},
	}
	addrs, err := r.LookupIPAddr(ctx, name)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

// measureDNS asks one resolver for every probe name. All must answer, with at
// least one address and none of them the filter's, or the resolver fails.
func measureDNS(r Resolver) DNSResult {
	res := DNSResult{Resolver: r}
	var times []time.Duration
	for _, name := range dnsProbeNames {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		start := time.Now()
		ips, err := queryFunc(ctx, r.IP, name)
		took := time.Since(start)
		cancel()
		if err != nil {
			res.Err = fmt.Errorf("%s: %w", name, err)
			return res
		}
		if len(ips) == 0 {
			res.Err = fmt.Errorf("%s: empty answer", name)
			return res
		}
		for _, ip := range ips {
			if censoredNet.Contains(ip) {
				res.Err = fmt.Errorf("%s: answered with the filter address %s", name, ip)
				return res
			}
		}
		times = append(times, took)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	res.Median = times[len(times)/2]
	return res
}

// MeasureDNS measures every resolver in parallel and returns the results with
// the working ones first, fastest first.
func MeasureDNS(list []Resolver) []DNSResult {
	out := make([]DNSResult, len(list))
	var wg sync.WaitGroup
	for i, r := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = measureDNS(r)
		}()
	}
	wg.Wait()
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Err == nil) != (out[j].Err == nil) {
			return out[i].Err == nil
		}
		return out[i].Median < out[j].Median
	})
	return out
}

// BestDNS measures the foreign resolvers and, only if none is good enough,
// the Iranian ones too. It returns every result, best first.
func BestDNS() []DNSResult {
	res := MeasureDNS(ForeignDNS)
	if len(res) > 0 && res[0].Err == nil && res[0].Median <= DNSGoodEnough {
		return res
	}
	res = append(res, MeasureDNS(IranDNS)...)
	sort.SliceStable(res, func(i, j int) bool {
		if (res[i].Err == nil) != (res[j].Err == nil) {
			return res[i].Err == nil
		}
		return res[i].Median < res[j].Median
	})
	return res
}

// resolvBackup is what /etc/resolv.conf was before the first change: either a
// symlink (the systemd-resolved default on Ubuntu) or a plain file.
type resolvBackup struct {
	Link    string `json:"link,omitempty"`
	Content string `json:"content,omitempty"`
	// Missing records that there was no resolv.conf at all, so Restore removes
	// the file rather than leaving an empty one behind.
	Missing bool `json:"missing,omitempty"`
}

func resolvPath() string       { return p("/etc/resolv.conf") }
func resolvBackupPath() string { return stateDir() + "/resolv.conf.backup.json" }

// backupResolv records the original resolv.conf, once. A later change must not
// overwrite it, or Restore would bring back ECK-Tunnel's own file.
func backupResolv() error {
	if _, err := os.Stat(resolvBackupPath()); err == nil {
		return nil
	}
	var b resolvBackup
	if link, err := os.Readlink(resolvPath()); err == nil {
		b.Link = link
	} else {
		data, err := os.ReadFile(resolvPath())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		b.Content = string(data)
		b.Missing = errors.Is(err, os.ErrNotExist)
	}
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.Marshal(b)
	return writeFileAtomic(resolvBackupPath(), data, 0o600)
}

// renderResolv is the resolv.conf that points at the chosen servers.
func renderResolv(servers []string) string {
	var b strings.Builder
	b.WriteString("# Set by ECK-Tunnel (sudo eck → Optimize → Best DNS).\n")
	b.WriteString("# Optimize → Best DNS → Restore puts the original back.\n")
	for _, s := range servers {
		b.WriteString("nameserver " + s + "\n")
	}
	b.WriteString("options timeout:2 attempts:2\n")
	return b.String()
}

// SetDNS points the server at the given resolvers (one or two). The original
// resolv.conf is backed up first; if it was systemd-resolved's symlink the link
// is replaced by a plain file, which systemd-resolved then leaves alone.
func SetDNS(servers ...string) error {
	if len(servers) == 0 {
		return errors.New("no DNS server given")
	}
	for _, s := range servers {
		if net.ParseIP(s) == nil {
			return fmt.Errorf("%q is not an IP address", s)
		}
	}
	if err := backupResolv(); err != nil {
		return fmt.Errorf("backing up resolv.conf: %w", err)
	}
	return writeFileAtomic(resolvPath(), []byte(renderResolv(servers)), 0o644)
}

// restartResolved is called after a restore that brings the systemd-resolved
// link back; replaced in tests.
var restartResolved = func() { _ = exec.Command("systemctl", "restart", "systemd-resolved").Run() }

// RestoreDNS puts back the resolv.conf that was there before the first
// SetDNS. With no backup it falls back to the Ubuntu default link.
func RestoreDNS() (string, error) {
	data, err := os.ReadFile(resolvBackupPath())
	if errors.Is(err, os.ErrNotExist) {
		stub := p("/run/systemd/resolve/stub-resolv.conf")
		if _, err := os.Stat(stub); err != nil {
			return "", errors.New("no backup was taken and systemd-resolved is not running — nothing to restore")
		}
		if err := replaceWithLink(resolvPath(), "../run/systemd/resolve/stub-resolv.conf"); err != nil {
			return "", err
		}
		restartResolved()
		return "no backup found — reset to the systemd-resolved default", nil
	}
	if err != nil {
		return "", err
	}
	var b resolvBackup
	if err := json.Unmarshal(data, &b); err != nil {
		return "", fmt.Errorf("the backup at %s is damaged: %w", resolvBackupPath(), err)
	}
	if b.Link != "" {
		if err := replaceWithLink(resolvPath(), b.Link); err != nil {
			return "", err
		}
		restartResolved()
	} else if b.Missing {
		if err := os.Remove(resolvPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	} else if err := writeFileAtomic(resolvPath(), []byte(b.Content), 0o644); err != nil {
		return "", err
	}
	os.Remove(resolvBackupPath())
	return "original resolv.conf restored", nil
}

func replaceWithLink(path, target string) error {
	tmp := path + ".eck-link"
	os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
