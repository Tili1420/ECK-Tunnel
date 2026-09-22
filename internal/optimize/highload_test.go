package optimize

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestConntrackMaxScalesWithRAMWithinBounds(t *testing.T) {
	for _, c := range []struct{ mem, want int }{
		{512, 262144},    // small VPS: the floor
		{2048, 524288},   // 2 GB: 256 per MB
		{4096, 1048576},  // 4 GB
		{65536, 2097152}, // 64 GB: the ceiling
	} {
		if got := ConntrackMax(c.mem); got != c.want {
			t.Errorf("ConntrackMax(%d) = %d, want %d", c.mem, got, c.want)
		}
	}
}

func TestHighLoadTuningConntrackRowsOnlyWhenLoaded(t *testing.T) {
	has := func(rows [][2]string, key string) (string, bool) {
		for _, kv := range rows {
			if kv[0] == key {
				return kv[1], true
			}
		}
		return "", false
	}
	without := HighLoadTuning(2048, false)
	if _, ok := has(without, "net.netfilter.nf_conntrack_max"); ok {
		t.Fatal("conntrack rows written without the module: sysctl would fail on every boot")
	}
	with := HighLoadTuning(2048, true)
	if v, ok := has(with, "net.netfilter.nf_conntrack_max"); !ok || v != strconv.Itoa(ConntrackMax(2048)) {
		t.Fatalf("nf_conntrack_max = %q, %v", v, ok)
	}
	if v, _ := has(with, "net.ipv4.tcp_keepalive_time"); v != "60" {
		t.Fatalf("tcp_keepalive_time = %q, want 60", v)
	}
}

func TestHighLoadDoesNotOverlapTheBaseTable(t *testing.T) {
	base := map[string]bool{}
	for _, kv := range FullTuning() {
		base[kv[0]] = true
	}
	for _, kv := range HighLoadTuning(4096, true) {
		if base[kv[0]] {
			t.Errorf("%s is set by both files; the later one would silently win", kv[0])
		}
	}
}

func TestMemTotalMB(t *testing.T) {
	dir := t.TempDir()
	old := procRoot
	procRoot = dir
	t.Cleanup(func() { procRoot = old })
	os.WriteFile(filepath.Join(dir, "meminfo"), []byte("MemTotal:        4028580 kB\nMemFree: 1 kB\n"), 0o644)
	if got := MemTotalMB(); got != 3934 {
		t.Fatalf("MemTotalMB = %d, want 3934", got)
	}
}

func TestRemoveHighLoad(t *testing.T) {
	dir := t.TempDir()
	saved := []*string{&highLoadSysctlFile, &highLoadModulesFile, &highLoadModprobe, &highLoadSystemdFile}
	olds := make([]string, len(saved))
	for i, p := range saved {
		olds[i] = *p
		*p = filepath.Join(dir, filepath.Base(*p))
		os.WriteFile(*p, []byte("x"), 0o644)
	}
	t.Cleanup(func() {
		for i, p := range saved {
			*p = olds[i]
		}
	})
	if !HighLoadApplied() {
		t.Fatal("HighLoadApplied false with the file present")
	}
	RemoveHighLoad(func(string) {})
	for _, p := range saved {
		if _, err := os.Stat(*p); !os.IsNotExist(err) {
			t.Errorf("%s left behind", *p)
		}
	}
}
