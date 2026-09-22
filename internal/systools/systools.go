// Package systools holds the machine-level helpers offered under Optimize that
// have nothing to do with a tunnel: picking the fastest reachable DNS resolver
// and the fastest reachable Ubuntu package mirror.
//
// Both exist for the same reason. A server in Iran often cannot reach the
// resolvers and the archive it was installed with, and a server abroad
// sometimes has a slow default. Each tool measures the real thing — a DNS
// answer, a Release file — rather than whether a port accepts a connection,
// because a port that answers says nothing about whether the service behind it
// works from here.
//
// Everything they change is backed up once, on first use, and put back by the
// matching Restore.
package systools

import (
	"os"
	"path/filepath"

	"github.com/eck-tunnel/eck/internal/app"
)

// root prefixes every absolute path these tools touch. It is empty on a real
// server; tests point it at a temporary directory.
var root = ""

func p(path string) string { return filepath.Join(root, path) }

// stateDir is where the backups live.
func stateDir() string { return p(filepath.Join(app.ConfigDir, "systools")) }

// writeFileAtomic replaces path with data in one rename, so a reader never sees
// a half-written file. When path is a symlink the link itself is replaced,
// which is what /etc/resolv.conf needs.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".eck-tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
