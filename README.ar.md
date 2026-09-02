# NexaGate

لوحة تحكم خفيفة بالإصدار `v0.3.0` تدعم Hysteria2 وVLESS XHTTP Reality وVLESS XHTTP TLS وRAW Reality وWebSocket TLS.

## البدء السريع

يتحقق المثبّت من المتطلبات ويثبتها تلقائياً:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/MehrooExplains/nexagate/main/install.sh)
```

بعد التثبيت افتح لوحة HTTPS واستخدم روابط المستخدمين أو رموز QR. تمر حركة TCP عبر Psiphon وحركة UDP عبر WARP. تتم ترقية الإعدادات القديمة تلقائياً وبأمان.

للدليل الكامل راجع [الإنجليزية](README.md) أو [الفارسية](README.fa.md).

## شرح مفصل

يفصل NexaGate مسارات الخروج عمداً: يمر TCP داخل النفق عبر Psiphon، بينما يمر UDP وDNS عبر Cloudflare WARP. يمنع جدار الحماية fail-closed كلاً من Xray وHysteria وDNS-relay من استخدام اتصال الإنترنت العادي للخادم بصمت عند تعطل النفق.

> الإصدار: `0.3.0`. اختبر المشروع أولاً على خادم جديد قابل للاستعادة. لا يوجد بروتوكول يضمن الوصول تحت جميع أنواع حجب الشبكات.

### البنية والمنافذ

```text
العملاء
  ├─ UDP/443  ─ Hysteria2
  ├─ TCP/443  ─ VLESS + XHTTP + REALITY
  ├─ TCP/8444 ─ VLESS + RAW + REALITY + Vision
  └─ TCP/2053 ─ Nginx TLS/HTTP2
                    ├─ VLESS + XHTTP + TLS
                    └─ VLESS + WebSocket + TLS

الإدارة: TCP/8443 → Nginx HTTPS → اللوحة على 127.0.0.1:9080
```

| المنفذ | الاستخدام |
|---|---|
| UDP `443` | Hysteria2 + Salamander |
| TCP `443` | VLESS XHTTP + REALITY |
| TCP `8444` | VLESS RAW + REALITY + Vision |
| TCP `2053` | VLESS XHTTP TLS وWebSocket TLS |
| TCP `8443` | لوحة HTTPS |
| TCP `80` | HTTP-01 للشهادات |

يمكن استخدام TCP وUDP على `443` معاً لأنهما نوعان مختلفان من النقل. افتح TCP `80,443,2053,8443,8444` وUDP `443` في جدار حماية الخادم ومجموعة الأمان لدى المزود.

### التثبيت

يلزم خادم Linux جديد يعمل بـ systemd، ومعمارية `amd64`/`x86_64`، وIPv4 عام، وصلاحية root أو `sudo`. يدعم المشروع `apt` و`dnf` و`yum`، مع أفضل تغطية اختبار على Debian/Ubuntu. يرفع المثبّت الصلاحيات عند الحاجة ويتحقق من المتطلبات ولا يثبت إلا الحزم الناقصة.

يسأل التثبيت فقط عن نوع الشهادة وبريد Let's Encrypt واسم النطاق في وضع النطاق. تُنشأ كلمة مرور مدير قوية وعشوائية تلقائياً وتظهر مرة واحدة عند نهاية التثبيت، ثم تحفظ في `/root/nexagate-initial-credentials.txt` المقصور على root. غيّرها من **Panel Settings** بعد الدخول ثم احذف الملف.

### ماسح هدف REALITY

افتح **Panel Settings → REALITY Target Scanner**. يفحص الإدخال الفارغ المرشحات المضمنة؛ ويمكن فحص نطاق واحد أو CIDR IPv4 عام ببادئة `/28` أو أصغر، بحد أقصى 16 عنواناً. يقيس الماسح TLS وALPN والشهادة وتفضيل X25519 وزمن الاستجابة من الخادم، ويرفض الشبكات الخاصة وذات الاستخدام الخاص.

نتائج IP/CIDR معلوماتية فقط؛ يظهر زر **Use** لنطاق تم التحقق منه فقط لأن REALITY يحتاج إلى اسم مضيف SNI مطابق. تعني نتيجة `feasible` توافق TLS فقط ولا تضمن التوفر في جميع الشبكات.

### اللوحة والأمان

- تعرض اللوحة CPU والذاكرة وswap والتخزين وسرعة/استهلاك الشبكة والمقابس ومدة التشغيل وموارد اللوحة؛ ويظل IP الخادم مخفياً حتى الضغط على رمز العين.
- تتوفر إدارة المستخدمين وروابط الاشتراك وQR والاختيار التلقائي لـ Psiphon/WARP وتحديث بنقرة واحدة مع checksum وrollback والشهادات عبر [CertDuo](https://github.com/MehrooExplains/certduo).
- تستخدم كلمات مرور المدير PBKDF2-HMAC-SHA-256 مع salt عشوائي؛ الجلسات Secure وHTTP-only وSameSite strict مع حماية CSRF وتقييد محاولات الدخول.
- تحتوي روابط الاتصال على بيانات اعتماد؛ لا تنشرها. عند التسريب عطّل المستخدم أو احذفه.

```bash
sudo nexagate doctor
sudo nexagate backup create --encrypt --password
sudo nexagate backup list
```
