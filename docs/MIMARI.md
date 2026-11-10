# Barney — mimari notları

Bu belge tasarım kararlarını ve neden öyle yapıldıklarını özetler. Uçların sözleşmesi için [API.md](API.md).

## Temel fikir

Barney bir kişinin **yazma biçimini** (kaç mesajı art arda attığı, uzunluk, küçük/büyük harf, noktalama, emoji, cevap hızı)
sohbet geçmişinden **kodla** çıkarır. Cevap üretirken modele bu kuralları ve gerçek örnekleri verir; çıkan metni yine
**kodla** denetler. Yani biçim hesaplanır ve doğrulanır, model yalnızca içeriği yazar.

## Paketler

| Paket | Görev |
|---|---|
| `internal/style` | Stil kartı çıkarma (`Extract`) ve cevap denetimi (`Check`, `PickBurst`). Yapay zekâsız. |
| `internal/reply` | Tek çağrılık cevap: hedef mesaj sayısı kodla seçilir → model → denetim → ihlalle yeniden deneme (≤2) → bekleme süreleri kodla. Ton (`tone.go`) ve bağlam blokları. |
| `internal/council` | Konsey: Okuyucu / Stratejist / Esprili (paralel, hızlı model) → sentez (güçlü model) → Vicdan vetosu. Güvenli, komik, cesur adaylar. |
| `internal/watch` | Gözcü: olay tabanlı izleme. Yapay zekâ yalnızca izlenen kişide, yeni, sakinleşmiş, anlamlı mesajda çalışır. Kişi bazlı `Policy` (sessiz saat, ilk-N onay, hassas konu, saatlik/günlük sınır). |
| `internal/people` | Kişi deposu (`~/.config/barney/people`), `Merge` (tekilleştirme), kart önbelleği, ihtiyatlı `Find` (yanlış kişiye eşleşmemek için). |
| `internal/chat` | Danışman (serbest sohbet) ve Rol provası (rolleri ters çevrilmiş üretim). |
| `internal/memory`, `internal/history` | Kişi hakkında olgular/aşama; öneri geçmişi ve düzeltmelerden öğrenme. |
| `internal/insights` | Sohbet analizi (LLM'siz). |
| `internal/browseragent` | Yapay zekâ tarayıcı ajanı: beyaz listeli eylemler, mesaj yazamaz. |
| `internal/server` | REST + SSE + WebSocket (yalnız 127.0.0.1), okuma işleri, Gözcü ↔ uzantı köprüsü. |
| `extension/` | Chrome MV3 uzantısı: WhatsApp Web okuma, sohbet açma, taslak/gönderim, yan panel. |
| `web/` | React arayüzü (ikiliye gömülür). |

## Ekonomi: her beklemede yapay zekâ yok

İzleme (`MutationObserver`) tamamen yereldir ve sıfır maliyetlidir. Model yalnızca şu koşulların **hepsi** sağlanınca çağrılır:
kişi izleniyor, mesaj daha önce işlenmemiş, karşı taraf yazmayı bırakıp sakinleşme süresi (varsayılan 8 sn) dolmuş,
mesaj "ok/👍" gibi bir kapanış değil, günlük çağrı sınırı dolmamış.

## Güvenlik modeli

- Sunucu yalnızca `127.0.0.1` dinler; Host denetimi (DNS rebinding), `Authorization: Bearer` anahtarı, sabit zamanlı karşılaştırma.
  WebSocket için `barney.v1` alt protokolü + anahtar + yalnızca `chrome-extension://` kaynağı. Arayüzde çerez yoktur (CSRF yüzeyi yok), CSP uygulanır.
- Otomatik gönderim **varsayılan kapalı**. Açıldığında: kişi izleniyor + aday güvenilir + hassas konu değil + sessiz saat değil + ilk-N onay tamam + sınırlar dolmamış.
  Biri tutmazsa cevap "öneri" kalır, onay ister.
- Gönderimden hemen önce uzantı yeniden doğrular: sohbet değişti mi, araya yeni mesaj girdi mi, kutuda taslak var mı.
  Kutu boşken aynı yerdeki **ses düğmesine asla** basılmaz.
- Tarayıcıdan okuma/AI modu yalnızca kaydırır, sol listeden sohbet seçer, arama kutusuna yazar ve okur. Mesaj kutusuna, gönder/ses/ek düğmelerine
  dokunamaz; bu hem sunucuda hem uzantıda beyaz listeyle zorlanır, model çıktısına güvenilmez.
- Veri `~/.config/barney` altında, `0600` izinli dosyalarda kalır. Modele giden istemlerde telefon/e-posta/IBAN maskelenir (AI tarayıcı modu hariç; orada sayfa içeriği okunmak zorundadır ve arayüz bunu açıkça söyler).

## Bilinen sınırlar

- WhatsApp Web arayüzü değişebilir; seçiciler `extension/content.js` içinde `SEL` altında toplanmıştır. Mesaj paneli gerçek DOM ile doğrulandı; sol liste ve arama kutusu seçicileri en iyi tahmindir.
- Geçmişi okurken WhatsApp sekmesi açık ve görünür olmalıdır (gizli sekmelerde tarayıcı kaydırma olaylarını tetiklemez).
- Instagram DM canlı köprüsü ve telefondan erişim henüz yok.
