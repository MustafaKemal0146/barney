// WhatsApp Web için DOM'dan bağımsız saf işlevler (node ile test edilir).
// Hem içerik betiği (globalThis.BarneyPure) hem node testi (module.exports) kullanır.
(function (root) {
  "use strict";

  // data-pre-plain-text: "[14:32, 25/03/2026] Ayşe: " ya da "[2:32 PM, 3/25/2026] Ayşe: "
  const PRE = /^\[(\d{1,2})[:.](\d{2})(?:[:.]\d{2})?\s*([AaPp][Mm])?,\s*(\d{1,4})[./-](\d{1,2})[./-](\d{1,4})\]\s*(.*?):\s*$/;

  function parsePrePlain(s) {
    if (!s) return null;
    const m = PRE.exec(s.replace(/[‎‏  ]/g, " ").trim());
    if (!m) return null;
    let h = +m[1];
    const min = +m[2];
    const ap = m[3] && m[3].toLowerCase();
    if (ap === "pm" && h < 12) h += 12;
    if (ap === "am" && h === 12) h = 0;
    let a = +m[4], b = +m[5], c = +m[6];
    let year, month, day;
    if (a > 31) { year = a; month = b; day = c; }       // 2026-03-25
    else {
      year = c < 100 ? 2000 + c : c;
      if (a > 12) { day = a; month = b; }                // 25/03/2026
      else if (b > 12) { month = a; day = b; }          // 3/25/2026
      else if (ap) { month = a; day = b; }              // AM/PM biçimi genelde ay/gün
      else { day = a; month = b; }                       // belirsiz: gün/ay (Türkiye)
    }
    const ts = Math.floor(new Date(year, month - 1, day, h, min).getTime() / 1000);
    return { sender: m[7], ts: Number.isFinite(ts) ? ts : 0 };
  }

  function hash(str) {
    let h = 5381;
    for (let i = 0; i < str.length; i++) h = ((h << 5) + h + str.charCodeAt(i)) | 0;
    return (h >>> 0).toString(36);
  }

  // Son KARŞI TARAF mesajının anahtarı. Okunan pencere kayınca (ben mesaj atınca, WhatsApp eski
  // satırları DOM'dan atınca) DEĞİŞMEMELİ: bu yüzden penceredeki toplam sayıya değil, mesajın kendi
  // kimliğine bakılır. Kimlik yoksa: son karşı taraf bloğunun uzunluğu + içerik özeti.
  function incomingKey(history) {
    let li = -1;
    for (let i = history.length - 1; i >= 0; i--) if (history[i].from === "them") { li = i; break; }
    if (li < 0) return "";
    const last = history[li];
    if (last.id) return "i:" + hash(String(last.id));
    let run = 0;
    for (let i = li; i >= 0 && history[i].from === "them"; i--) run++;
    return run + ":" + hash((last.ts || 0) + "|" + last.text);
  }

  function lastIsIncoming(history) {
    return history.length > 0 && history[history.length - 1].from === "them";
  }

  // Sohbet değiştiğinde ilk okuma "temel çizgi"dir: eski cevapsız mesaja tepki verilmez.
  function shouldNotify(prev, chatId, key, history) {
    if (!key || !lastIsIncoming(history)) return { notify: false, next: { chatId, key: key || (prev && prev.key) || "" } };
    if (!prev || prev.chatId !== chatId) return { notify: false, next: { chatId, key } };
    if (prev.key === key) return { notify: false, next: prev };
    return { notify: true, next: { chatId, key } };
  }

  // Mesajı insan gibi yazarken karakter başına gecikme (ms).
  function typingDelayMs(cps, rnd) {
    return (1000 / cps) * (0.6 + 0.8 * rnd);
  }

  // Mesajın yönü. Gerçek WhatsApp Web DOM'unda: yalnızca grubun ilk mesajında kuyruk simgesi
  // (tail-out/tail-in) var; giden mesajlarda meta içinde okundu/iletildi işareti var; giden
  // grubun ilk mesajında aria-label "Siz:". Belirsizse güvenli taraf "me" (kendi mesajımıza
  // cevap vermektense sessiz kalmak yeğdir) — yalnız göndericisi sohbet başlığıyla aynıysa "them".
  function direction(sig) {
    if (sig.tailOut) return "me";
    if (sig.tailIn) return "them";
    if (sig.ticks) return "me";
    if (sig.youLabel) return "me";
    const norm = (x) => (x || "").replace(/\s+/g, " ").trim().toLowerCase();
    if (sig.sender) return norm(sig.sender) === norm(sig.partner) ? "them" : "me";
    return "them"; // gönderici bilgisi yok (medya): tikler yoksa karşı taraf
  }

  // ---- geçmiş okuma yardımcıları (saf)

  const MONTHS = { ocak: 1, şubat: 2, subat: 2, mart: 3, nisan: 4, mayıs: 5, mayis: 5, haziran: 6, temmuz: 7, ağustos: 8, agustos: 8, eylül: 9, eylul: 9, ekim: 10, kasım: 11, kasim: 11, aralık: 12, aralik: 12,
    january: 1, february: 2, march: 3, april: 4, may: 5, june: 6, july: 7, august: 8, september: 9, october: 10, november: 11, december: 12 };
  const WEEKDAYS = { pazar: 0, pazartesi: 1, salı: 2, sali: 2, çarşamba: 3, carsamba: 3, perşembe: 4, persembe: 4, cuma: 5, cumartesi: 6,
    sunday: 0, monday: 1, tuesday: 2, wednesday: 3, thursday: 4, friday: 5, saturday: 6 };

  // Tarih ayracı metnini ("Bugün", "Dün", "Pazartesi", "9 Kasım 2026", "09.11.2026") gün başı unix saniyesine çevirir.
  function parseDayLabel(label, now) {
    if (!label) return null;
    const t = label.replace(/[‎‏  ]/g, " ").trim().toLowerCase();
    if (!t || t.length > 40) return null;
    const base = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const day = (d) => Math.floor(d.getTime() / 1000);
    if (t === "bugün" || t === "bugun" || t === "today") return day(base);
    if (t === "dün" || t === "dun" || t === "yesterday") return day(new Date(base.getTime() - 86400000));
    if (t in WEEKDAYS) {
      let diff = (base.getDay() - WEEKDAYS[t] + 7) % 7;
      if (diff === 0) diff = 7;
      return day(new Date(base.getTime() - diff * 86400000));
    }
    let m = /^(\d{1,2})\s+([a-zçğıöşü]+)\s+(\d{4})$/.exec(t);
    if (m && MONTHS[m[2]]) return day(new Date(+m[3], MONTHS[m[2]] - 1, +m[1]));
    m = /^([a-z]+)\s+(\d{1,2}),?\s+(\d{4})$/.exec(t);
    if (m && MONTHS[m[1]]) return day(new Date(+m[3], MONTHS[m[1]] - 1, +m[2]));
    m = /^(\d{1,2})[./-](\d{1,2})[./-](\d{4})$/.exec(t);
    if (m) {
      let a = +m[1], b = +m[2];
      if (b > 12) { const x = a; a = b; b = x; } // aa/gg/yyyy
      return day(new Date(+m[3], b - 1, a));
    }
    return null;
  }

  // "16:23" ya da "4:23 PM" → gün içi dakika.
  function parseClock(text) {
    const m = /^(\d{1,2})[:.](\d{2})\s*([AaPp][Mm])?$/.exec((text || "").trim());
    if (!m) return null;
    let h = +m[1];
    const ap = m[3] && m[3].toLowerCase();
    if (ap === "pm" && h < 12) h += 12;
    if (ap === "am" && h === 12) h = 0;
    return h * 60 + +m[2];
  }

  // DOM sırasındaki öğeleri (ayraç ve mesaj) zaman damgasına çevirir. öğe: {sep: dayStartTs} ya da
  // {pre: ts|0, clock: "16:23"|"", ...}. Zamanı bulunamayanlar en yakın komşunun zamanını alır.
  function stampRows(items) {
    let day = 0, last = 0;
    const out = [];
    for (const it of items) {
      if (it.sep) { day = it.sep; continue; }
      let ts = it.pre || 0;
      if (!ts && day && it.clock) {
        const mins = parseClock(it.clock);
        if (mins != null) ts = day + mins * 60;
      }
      if (ts) { last = ts; if (!day) day = Math.floor(ts / 86400) * 86400; }
      out.push({ ...it, ts: ts || 0 });
    }
    // komşudan doldur: önce ileri (önceki mesajın zamanı), sonra geri (sonrakinin zamanı)
    let prev = 0;
    for (const o of out) { if (o.ts) prev = o.ts; else if (prev) o.ts = prev; }
    let next = 0;
    for (let i = out.length - 1; i >= 0; i--) { if (out[i].ts) next = out[i].ts; else if (next) out[i].ts = next; }
    return out;
  }

  function chunk(arr, n) {
    const out = [];
    for (let i = 0; i < arr.length; i += n) out.push(arr.slice(i, i + n));
    return out;
  }

  function mostCommon(list) {
    const c = new Map();
    let best = "", n = 0;
    for (const x of list) {
      if (!x) continue;
      const v = (c.get(x) || 0) + 1;
      c.set(x, v);
      if (v > n) { n = v; best = x; }
    }
    return best;
  }

  const api = { parseDayLabel, parseClock, stampRows, chunk, mostCommon, direction, parsePrePlain, hash, incomingKey, lastIsIncoming, shouldNotify, typingDelayMs };
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.BarneyPure = api;
})(typeof globalThis !== "undefined" ? globalThis : this);
