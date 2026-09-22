# Server layout (file locations)

Everything lives in a tidy, predictable layout. You can also see this any time
from **Manage → File Locations** in the CLI.

| Path | What |
|------|------|
| `/root/ECK-Tunnel` | The release bundle and downloaded archives. |
| `/root/ECK-Tunnel/backups` | [Backup](backup-restore.md) `.tar.gz` files. |
| `/etc/eck` | Tunnel configs (one `.toml` per tunnel) and runtime state. |
| `/usr/local/bin/eck` | The binary itself. |
| `eck-<name>.service` | A systemd unit per tunnel. |
| `eck-monitor.service` | The [monitor service](monitor-service.md). |

The install directory is recorded in `/etc/eck/install_path`, which is what
the uninstaller reads to know what to remove.

---

<div dir="rtl">

## خلاصهٔ فارسی

همه‌چیز در یک ساختار مرتب و قابل‌پیش‌بینی است. همین را هر وقت خواستی از
`Manage → File Locations` در CLI هم می‌بینی.

`‎/root/ECK-Tunnel` بستهٔ ریلیز و آرشیوهای دانلودشده ·
`‎/root/ECK-Tunnel/backups` فایل‌های [پشتیبان](backup-restore.md) ·
`‎/etc/eck` کانفیگ تونل‌ها (برای هر تونل یک فایل `.toml`) و وضعیت اجرا ·
`‎/usr/local/bin/eck` خودِ باینری ·
`eck-<name>.service` یک یونیت systemd به‌ازای هر تونل ·
`eck-monitor.service` [سرویس مانیتور](monitor-service.md).

مسیر نصب در `/etc/eck/install_path` ثبت می‌شود و حذف‌کنندهٔ داخلی از روی
همان می‌فهمد چه چیزی را پاک کند.

</div>

---
[← Back to the docs index](README.md)

---

*Last verified against ECK-Tunnel v1.8.2.*
