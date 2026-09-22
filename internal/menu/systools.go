// The Best DNS and Best apt mirror screens under Optimize.

package menu

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/eck-tunnel/eck/internal/optimize"
	"github.com/eck-tunnel/eck/internal/systools"
	"github.com/eck-tunnel/eck/internal/tui"
)

func bestDNSMenu() {
	for {
		tui.Clear()
		tui.Title("Best DNS")
		fmt.Println()
		opts := []tui.Option{
			{Title: "Auto", Desc: "foreign resolvers first, Iranian ones only if none is fast"},
			{Title: "Pick from the list", Desc: "measure all of them and choose"},
			{Title: "Restore", Desc: "put the original resolv.conf back"},
		}
		switch tui.ChooseOpt("Choose:", opts) {
		case 0:
			dnsAuto()
		case 1:
			dnsManual()
		case 2:
			msg, err := systools.RestoreDNS()
			reportResult(msg, err)
		default:
			return
		}
		tui.PressEnter()
	}
}

func printDNSResults(res []systools.DNSResult) {
	for i, r := range res {
		if r.Err != nil {
			fmt.Printf("  %2d) %-16s %-10s %sfailed: %v%s\n", i+1, r.IP, r.Name, tui.Gray, r.Err, tui.Reset)
			continue
		}
		fmt.Printf("  %2d) %-16s %-10s %5d ms\n", i+1, r.IP, r.Name, r.Median.Milliseconds())
	}
	fmt.Println()
}

func dnsAuto() {
	tui.Info("Measuring resolvers with real lookups...")
	res := systools.BestDNS()
	printDNSResults(res)
	var pick []string
	for _, r := range res {
		if r.Err == nil && len(pick) < 2 {
			pick = append(pick, r.IP)
		}
	}
	if len(pick) == 0 {
		tui.Error("No resolver answered correctly from this server — nothing changed.")
		return
	}
	reportResult(fmt.Sprintf("DNS set to %v", pick), systools.SetDNS(pick...))
}

func dnsManual() {
	tui.Info("Measuring every resolver...")
	res := systools.MeasureDNS(append(append([]systools.Resolver{}, systools.ForeignDNS...), systools.IranDNS...))
	printDNSResults(res)
	n := tui.PromptInt("Resolver number (0 to cancel)", 0)
	if n < 1 || n > len(res) {
		return
	}
	r := res[n-1]
	if r.Err != nil && !tui.Confirm(r.IP+" failed the test here. Use it anyway", false) {
		return
	}
	reportResult("DNS set to "+r.IP, systools.SetDNS(r.IP))
}

func bestMirrorMenu() {
	for {
		tui.Clear()
		tui.Title("Best apt mirror")
		fmt.Println()
		info, err := systools.CheckMirrorSupport()
		if err != nil {
			tui.Error(err.Error())
			tui.PressEnter()
			return
		}
		tui.Info("Ubuntu " + info.Codename)
		fmt.Println()
		opts := []tui.Option{
			{Title: "Auto", Desc: "foreign mirrors first, Iranian ones only if none is fast"},
			{Title: "Pick from the list", Desc: "measure all of them and choose"},
			{Title: "Restore", Desc: "put the original package sources back"},
		}
		switch tui.ChooseOpt("Choose:", opts) {
		case 0:
			mirrorAuto(info.Codename)
		case 1:
			mirrorManual(info.Codename)
		case 2:
			target, err := systools.RestoreMirror()
			reportResult("restored "+target, err)
		default:
			return
		}
		tui.PressEnter()
	}
}

func printMirrorResults(res []systools.MirrorResult) {
	for i, r := range res {
		if r.Err != nil {
			fmt.Printf("  %2d) %-45s %sfailed: %v%s\n", i+1, r.URL, tui.Gray, r.Err, tui.Reset)
			continue
		}
		fmt.Printf("  %2d) %-45s %6d ms\n", i+1, r.URL, r.Took.Milliseconds())
	}
	fmt.Println()
}

func mirrorAuto(code string) {
	tui.Info("Fetching the " + code + " Release file from each mirror...")
	res := systools.BestMirror(code)
	printMirrorResults(res)
	if len(res) == 0 || res[0].Err != nil {
		tui.Error("No mirror answered correctly from this server — nothing changed.")
		return
	}
	applyMirror(res[0].URL, code)
}

func mirrorManual(code string) {
	tui.Info("Measuring every mirror...")
	res := systools.MeasureMirrors(append(append([]string{}, systools.ForeignMirrors...), systools.IranMirrors...), code)
	printMirrorResults(res)
	n := tui.PromptInt("Mirror number (0 to cancel)", 0)
	if n < 1 || n > len(res) {
		return
	}
	r := res[n-1]
	if r.Err != nil && !tui.Confirm(r.URL+" failed the test here. Use it anyway", false) {
		return
	}
	applyMirror(r.URL, code)
}

func applyMirror(url, code string) {
	target, err := systools.SetMirror(url, code)
	if err != nil {
		tui.Error("Could not change the mirror: " + err.Error())
		return
	}
	tui.Success("Mirror set to " + url + " (" + target + ")")
	if tui.Confirm("Run apt-get update now to check it", true) {
		cmd := exec.Command("apt-get", "update")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			tui.Error("apt-get update failed — Restore puts the original sources back.")
		}
	}
}

func reportResult(msg string, err error) {
	if err != nil {
		tui.Error(err.Error())
		return
	}
	tui.Success(msg)
}

func highLoadLabel() string {
	if optimize.HighLoadApplied() {
		return "applied"
	}
	return "not applied"
}

func highLoadMenu() {
	tui.Clear()
	tui.Title("High-load tuning — many users, fewer drops")
	fmt.Println()
	if line, warn := optimize.ConntrackReport(); line != "" {
		if warn {
			tui.Error(line)
		} else {
			tui.Info(line)
		}
		fmt.Println()
	}
	opts := []tui.Option{
		{Title: "Apply", Desc: "conntrack table sized to RAM, faster dead-peer detection, service file limits"},
		{Title: "Remove", Desc: "delete these settings (defaults return after a reboot)"},
	}
	log := func(line string) { tui.Info("• " + line) }
	switch tui.ChooseOpt("Choose:", opts) {
	case 0:
		fmt.Println()
		optimize.ApplyHighLoad(log)
	case 1:
		fmt.Println()
		optimize.RemoveHighLoad(log)
	default:
		return
	}
	tui.PressEnter()
}
