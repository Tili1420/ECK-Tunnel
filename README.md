# ECK-Tunnel

> Based on BackPack by Amin Mohammadi (AminMGMT)
> https://github.com/AminMGMT/BackPack

**ECK-Tunnel** is a tunnel engine for Iran ⇄ abroad (kharej) server pairs: one
self-contained Linux binary with an interactive CLI and a web panel.
[راهنمای فارسی](README_FA.md)

```
  end users ──▶  IRAN server  ══ tunnel ══▶  KHAREJ server  ──▶  real service
                 "Setup Iran"                 "Setup Kharej"
```

## Install

As root, on **both** servers (Ubuntu 22.04 or newer):

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Tili1420/ECK-Tunnel/main/install.sh)
```

Reopen the menu any time with `sudo eck`.

## Quick start

1. On the **Iran** server: `sudo eck` → **1. Setup Iran** → pick a transport,
   tunnel port and exposed ports → **copy the token**.
2. On the **Kharej** server: `sudo eck` → **2. Setup Kharej** → same transport,
   Iran IP, same tunnel port, **same token**.
3. `Manage → Status` on either side; `Manage → Health Check` if anything is off.

For heavily filtered routes start with **TCP + Stealth** (Noise-encrypted, no
fingerprint), then try **TCP + PCK**, **WSS**, or **xDi (ICMP)**.
Every transport: [docs/transports.md](docs/transports.md) ·
tutorials: [tutorial/README.md](tutorial/README.md).

## Build from source

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o eck .
```

## License

ECK-Tunnel is a modified version of BackPack and is released under the
**GNU Affero General Public License v3.0** — see [LICENSE](LICENSE) and
[NOTICE](NOTICE). The BackPack name and logo belong to their author and are not
used by this project ([TRADEMARK.md](TRADEMARK.md)).
