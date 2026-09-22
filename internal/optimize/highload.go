package optimize

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// High-load tuning: what the base Optimize leaves out, for a server carrying
// many users at once.
//
// The base table sizes buffers, backlogs and port ranges. Three things still
// fail under load, and each looks like "the tunnel keeps dropping" to a user:
//
//   - The connection-tracking table. Any firewall rule (ufw, x-ui's fail2ban,
//     an iptables ban) loads nf_conntrack, whose default ceiling is sized for a
//     desktop. When it fills, the kernel drops new packets and logs
//     "nf_conntrack: table full" — users see random connection failures while
//     the tunnel itself looks healthy.
//
//   - Dead-peer detection. With the kernel defaults a connection whose far end
//     vanished is kept for about two hours before the first keepalive, and a
//     connection with data in flight retries for about fifteen minutes. On a
//     route that drops silently, that is how long a stuck carrier holds
//     traffic before anything reconnects. Tighter values let the tunnel notice
//     in about a minute and a half and rebuild the connection.
//
//   - The open-file ceiling of systemd services. limits.d applies to login
//     sessions only; xray under x-ui is started by systemd and never reads it.
//     A default for every unit covers services this program does not own.
//
// This lives in its own files so it can be applied and removed on its own,
// without touching what the base Optimize wrote.

var (
	highLoadSysctlFile  = "/etc/sysctl.d/99-eck-highload.conf"
	highLoadModulesFile = "/etc/modules-load.d/eck-conntrack.conf"
	highLoadModprobe    = "/etc/modprobe.d/eck-conntrack.conf"
	highLoadSystemdFile = "/etc/systemd/system.conf.d/99-eck-limits.conf"
	procRoot            = "/proc"
	sysRoot             = "/sys"
)

const highLoadSystemd = `# Managed by eck — High-load tuning. Raises the open-file ceiling of every
# systemd service (xray, x-ui, nginx...), which limits.d does not reach.
# Takes effect for each service when it next starts.
[Manager]
DefaultLimitNOFILE=1048576:1048576
`

// ConntrackMax sizes the connection-tracking table from RAM: 256 entries per
// MB, never below 262144 or above 2097152. A full entry is about 320 bytes, so
// even a completely full table costs under a tenth of memory.
func ConntrackMax(memMB int) int {
	n := memMB * 256
	if n < 262144 {
		n = 262144
	}
	if n > 2097152 {
		n = 2097152
	}
	return n
}

// HighLoadTuning is the sysctl table for a machine with memMB of RAM.
// withConntrack adds the netfilter rows; they only exist once nf_conntrack is
// loaded.
func HighLoadTuning(memMB int, withConntrack bool) [][2]string {
	rows := [][2]string{
		{"fs.file-max", "2097152"},
		{"fs.nr_open", "2097152"},
		// First keepalive after 60 s idle, then every 10 s; 6 misses (~2 min)
		// declares the peer dead instead of the default two-plus hours.
		{"net.ipv4.tcp_keepalive_time", "60"},
		{"net.ipv4.tcp_keepalive_intvl", "10"},
		{"net.ipv4.tcp_keepalive_probes", "6"},
		// Give up on unacknowledged data after ~100 s instead of ~15 min, so
		// a carrier on a silently dead path is replaced rather than waited on.
		{"net.ipv4.tcp_retries2", "8"},
		// Keeps accepting real users during a SYN flood.
		{"net.ipv4.tcp_syncookies", "1"},
	}
	if withConntrack {
		rows = append(rows,
			[2]string{"net.netfilter.nf_conntrack_max", strconv.Itoa(ConntrackMax(memMB))},
			// The default keeps an idle established entry for five days, which
			// is how the table fills with connections long gone.
			[2]string{"net.netfilter.nf_conntrack_tcp_timeout_established", "86400"},
			[2]string{"net.netfilter.nf_conntrack_tcp_timeout_time_wait", "30"},
			[2]string{"net.netfilter.nf_conntrack_tcp_timeout_close_wait", "60"},
			[2]string{"net.netfilter.nf_conntrack_tcp_timeout_fin_wait", "30"},
		)
	}
	return rows
}

// MemTotalMB reads the machine's RAM from /proc/meminfo; 1024 if unreadable.
func MemTotalMB() int {
	f, err := os.Open(procRoot + "/meminfo")
	if err != nil {
		return 1024
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			if kb, err := strconv.Atoi(fields[1]); err == nil {
				return kb / 1024
			}
		}
	}
	return 1024
}

// conntrackLoaded reports whether nf_conntrack is in the kernel right now.
func conntrackLoaded() bool {
	_, err := os.Stat(procRoot + "/sys/net/netfilter/nf_conntrack_max")
	return err == nil
}

func readInt(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	return n, err == nil
}

// ConntrackReport describes the table's state in one line, and whether it
// needs attention. Empty when conntrack is not loaded — then nothing is tracked
// and the table cannot fill.
func ConntrackReport() (string, bool) {
	if !conntrackLoaded() {
		return "", false
	}
	count, _ := readInt(procRoot + "/sys/net/netfilter/nf_conntrack_count")
	max, _ := readInt(procRoot + "/sys/net/netfilter/nf_conntrack_max")
	line := fmt.Sprintf("Connection tracking: %d of %d entries in use", count, max)
	warn := max > 0 && count*100/max >= 80
	if out, err := exec.Command("dmesg").Output(); err == nil {
		if n := strings.Count(string(out), "nf_conntrack: table full"); n > 0 {
			line += fmt.Sprintf(" — the table has overflowed %d time(s) since boot (dropped packets)", n)
			warn = true
		}
	}
	return line, warn
}

// ApplyHighLoad writes and applies the high-load tuning.
func ApplyHighLoad(logf func(string)) {
	if runtime.GOOS != "linux" {
		logf("High-load tuning is only supported on Linux — skipping.")
		return
	}
	if line, warn := ConntrackReport(); line != "" {
		if warn {
			line = "⚠ " + line
		}
		logf(line)
	}

	mem := MemTotalMB()
	ct := conntrackLoaded()
	rows := HighLoadTuning(mem, ct)

	var b strings.Builder
	b.WriteString("# Managed by eck — High-load tuning (many users, fewer drops)\n")
	for _, kv := range rows {
		fmt.Fprintf(&b, "%s = %s\n", kv[0], kv[1])
	}
	if err := os.WriteFile(highLoadSysctlFile, []byte(b.String()), 0o644); err != nil {
		logf("Could not write " + highLoadSysctlFile + ": " + err.Error())
		return
	}
	logf("Wrote " + highLoadSysctlFile)

	if ct {
		max := ConntrackMax(mem)
		// Load the module before sysctl.d is applied at boot, or its keys do
		// not exist yet and the limit silently stays at the default.
		writeOrLog(logf, highLoadModulesFile, "# Managed by eck — High-load tuning\nnf_conntrack\n")
		// A hash table a quarter of the entry limit keeps lookups short.
		hash := strconv.Itoa(max / 4)
		writeOrLog(logf, highLoadModprobe, "# Managed by eck — High-load tuning\noptions nf_conntrack hashsize="+hash+"\n")
		if err := os.WriteFile(sysRoot+"/module/nf_conntrack/parameters/hashsize", []byte(hash), 0o600); err == nil {
			logf("Connection-tracking hash table set to " + hash + " buckets")
		}
		logf(fmt.Sprintf("Connection-tracking table raised to %d entries (%d MB RAM)", max, mem))
	} else {
		logf("No firewall connection tracking is loaded — nothing to raise there.")
	}

	applied := 0
	for _, kv := range rows {
		if err := exec.Command("sysctl", "-w", kv[0]+"="+kv[1]).Run(); err == nil {
			applied++
		}
	}
	logf(fmt.Sprintf("Applied %d/%d kernel parameters live.", applied, len(rows)))

	if err := os.MkdirAll(dirOf(highLoadSystemdFile), 0o755); err == nil {
		writeOrLog(logf, highLoadSystemdFile, highLoadSystemd)
		logf("Services (xray, x-ui...) get the higher open-file limit when they next restart, or after a reboot.")
	}
	logf("High-load tuning complete.")
}

// RemoveHighLoad deletes everything ApplyHighLoad wrote. Live kernel values
// stay until the next reboot, when the defaults come back.
func RemoveHighLoad(logf func(string)) {
	removed := 0
	for _, f := range []string{highLoadSysctlFile, highLoadModulesFile, highLoadModprobe, highLoadSystemdFile} {
		if err := os.Remove(f); err == nil {
			removed++
			logf("Removed " + f)
		}
	}
	if removed == 0 {
		logf("High-load tuning was not applied — nothing to remove.")
		return
	}
	logf("Removed. The kernel keeps the current values until the next reboot.")
}

// HighLoadApplied reports whether the high-load sysctl file is present.
func HighLoadApplied() bool {
	_, err := os.Stat(highLoadSysctlFile)
	return err == nil
}

func writeOrLog(logf func(string), path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		logf("Could not write " + path + ": " + err.Error())
	}
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i > 0 {
		return path[:i]
	}
	return "."
}
