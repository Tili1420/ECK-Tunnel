<div dir="rtl">

# ECK-Tunnel

> Based on BackPack by Amin Mohammadi (AminMGMT)
> https://github.com/AminMGMT/BackPack

**ECK-Tunnel** موتور تانل بین سرور ایران و سرور خارج است: یک فایل اجرایی لینوکس با منوی ترمینال و پنل وب.

## نصب

روی **هر دو سرور** با کاربر root (اوبونتو ۲۲ یا جدیدتر):

</div>

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Tili1420/ECK-Tunnel/main/install.sh)
```

<div dir="rtl">

بعداً هر وقت خواستید منو را با `sudo eck` باز کنید.

## راه‌اندازی سریع

۱. روی **سرور ایران**: `sudo eck` ← **1. Setup Iran** ← نوع اتصال، پورت تانل و پورت‌های کاربران را انتخاب کنید ← **توکن را کپی کنید**.

۲. روی **سرور خارج**: `sudo eck` ← **2. Setup Kharej** ← همان نوع اتصال، آی‌پی سرور ایران، همان پورت تانل و **همان توکن**.

۳. وضعیت را در `Manage → Status` ببینید. اگر مشکلی بود `Manage → Health Check` زیر هر خطا راه‌حلش را می‌نویسد.

برای مسیرهای پرفیلتر اول **TCP + Stealth** را امتحان کنید، بعد **TCP + PCK**، **WSS** یا **xDi (ICMP)**.

## لایسنس

ECK-Tunnel نسخهٔ تغییریافتهٔ BackPack است و تحت لایسنس **AGPL-3.0** منتشر می‌شود. فایل‌های [LICENSE](LICENSE) و [NOTICE](NOTICE) را ببینید.

</div>
