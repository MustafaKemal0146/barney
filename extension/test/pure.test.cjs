const test = require("node:test");
const assert = require("node:assert/strict");
const P = require("../wa-pure.js");

test("parsePrePlain: TR biçimi", () => {
  const r = P.parsePrePlain("[14:32, 25/03/2026] Ayşe: ");
  assert.equal(r.sender, "Ayşe");
  assert.equal(r.ts, Math.floor(new Date(2026, 2, 25, 14, 32).getTime() / 1000));
});
test("parsePrePlain: AM/PM ve ay/gün", () => {
  const r = P.parsePrePlain("[2:05 PM, 3/25/2026] John Doe: ");
  assert.equal(r.sender, "John Doe");
  assert.equal(r.ts, Math.floor(new Date(2026, 2, 25, 14, 5).getTime() / 1000));
});
test("parsePrePlain: gizli karakterler ve saniye", () => {
  const r = P.parsePrePlain("‎[14.32.10, 01.04.2026] Ayşe: ");
  assert.equal(r.sender, "Ayşe");
});
test("parsePrePlain: geçersiz girdi null", () => {
  assert.equal(P.parsePrePlain(""), null);
  assert.equal(P.parsePrePlain("rastgele"), null);
});
test("incomingKey: yalnız karşı tarafa bakar, yeni mesajla değişir", () => {
  const h = [{ from: "me", text: "napıyon", ts: 1 }, { from: "them", text: "selam", ts: 2 }];
  const k1 = P.incomingKey(h);
  assert.ok(k1.startsWith("1:"));
  assert.equal(P.incomingKey([...h, { from: "me", text: "iyi", ts: 3 }]), k1); // kendi mesajım anahtarı değiştirmez
  assert.notEqual(P.incomingKey([...h, { from: "them", text: "selam", ts: 4 }]), k1);
  assert.equal(P.incomingKey([{ from: "me", text: "x", ts: 1 }]), "");
});
test("incomingKey: pencere kayınca (eski satırlar düşünce) değişmez", () => {
  const old = [{ from: "them", text: "eski", ts: 1 }, { from: "me", text: "a", ts: 2 }];
  const tail = [{ from: "them", text: "yok ya", ts: 3 }, { from: "them", text: "kadardi", ts: 4 }, { from: "me", text: "neyse", ts: 5 }];
  assert.equal(P.incomingKey([...old, ...tail]), P.incomingKey(tail)); // kimliksiz yedek yol
  const w = (h) => h.map((m, i) => ({ ...m, id: "conv-msg-" + m.ts }));
  assert.equal(P.incomingKey(w([...old, ...tail])), P.incomingKey(w(tail)));
  assert.ok(P.incomingKey(w(tail)).startsWith("i:"));
  assert.notEqual(P.incomingKey(w([...tail, { from: "them", text: "kadardi", ts: 6 }])), P.incomingKey(w(tail)));
});
test("shouldNotify: temel çizgi, yeni mesaj, tekrar, sohbet değişimi", () => {
  const h1 = [{ from: "them", text: "a", ts: 1 }];
  const k1 = P.incomingKey(h1);
  let r = P.shouldNotify(null, "Ayşe", k1, h1);
  assert.equal(r.notify, false); // ilk okuma: eski mesaj
  const h2 = [...h1, { from: "them", text: "b", ts: 2 }];
  const k2 = P.incomingKey(h2);
  r = P.shouldNotify(r.next, "Ayşe", k2, h2);
  assert.equal(r.notify, true);
  r = P.shouldNotify(r.next, "Ayşe", k2, h2);
  assert.equal(r.notify, false); // aynı mesaj tekrar bildirilmez
  r = P.shouldNotify(r.next, "Mehmet", k2, h2);
  assert.equal(r.notify, false); // başka sohbete geçiş: temel çizgi
  const hMe = [...h2, { from: "me", text: "tamam", ts: 3 }];
  r = P.shouldNotify({ chatId: "Mehmet", key: k2 }, "Mehmet", k2, hMe);
  assert.equal(r.notify, false); // son mesaj benim
});
test("typingDelayMs aralığı", () => {
  assert.ok(Math.abs(P.typingDelayMs(5, 0) - 120) < 1e-9);
  assert.ok(Math.abs(P.typingDelayMs(5, 1) - 280) < 1e-9);
});
test("parsePrePlain: gerçek WhatsApp Web biçimi (gg.aa.yyyy, ad parantezli)", () => {
  const m = P.parsePrePlain("[23:11, 09.11.2026] Ali Veli: ");
  assert.equal(m.sender, "Ali Veli");
  assert.equal(new Date(m.ts * 1000).getMonth(), 10);
  assert.equal(new Date(m.ts * 1000).getDate(), 9);
  assert.equal(P.parsePrePlain("[23:13, 09.11.2026] Ayşe Demir (Ayşe): ").sender, "Ayşe Demir (Ayşe)");
});
test("direction: kuyruk, tik, Siz etiketi, gönderici; belirsizde güvenli taraf", () => {
  assert.equal(P.direction({ tailOut: true }), "me");
  assert.equal(P.direction({ tailIn: true, sender: "x", partner: "y" }), "them");
  assert.equal(P.direction({ ticks: true, sender: "Kaan", partner: "Kaan" }), "me"); // okundu tiki varsa benim
  assert.equal(P.direction({ youLabel: true }), "me");
  assert.equal(P.direction({ sender: "Ayşe Demir (Ayşe)", partner: "Ayşe Demir (Ayşe)" }), "them");
  assert.equal(P.direction({ sender: "Ali Veli", partner: "Ayşe Demir (Ayşe)" }), "me");
  assert.equal(P.direction({}), "them"); // gönderici yok, tik yok: gelen medya
});
test("parseDayLabel: Bugün/Dün/gün adı/tarih biçimleri", () => {
  const now = new Date(2026, 10, 10, 15, 0); // 10 Kasım 2026 Salı
  const d = (y, m, day) => Math.floor(new Date(y, m, day).getTime() / 1000);
  assert.equal(P.parseDayLabel("Bugün", now), d(2026, 10, 10));
  assert.equal(P.parseDayLabel("Dün", now), d(2026, 10, 9));
  assert.equal(P.parseDayLabel("Pazartesi", now), d(2026, 10, 9));
  assert.equal(P.parseDayLabel("Salı", now), d(2026, 10, 3)); // bugünse geçen hafta
  assert.equal(P.parseDayLabel("9 Kasım 2026", now), d(2026, 10, 9));
  assert.equal(P.parseDayLabel("09.11.2026", now), d(2026, 10, 9));
  assert.equal(P.parseDayLabel("Yazıyor…", now), null);
  assert.equal(P.parseDayLabel("Telefonunuzdaki eski mesajları almak için buraya tıklayın", now), null);
});
test("stampRows: ayraç + saat, pre-plain öncelikli, boşluklar komşudan dolar", () => {
  const now = new Date(2026, 10, 10, 15, 0);
  const sep = P.parseDayLabel("Dün", now);
  const pre = Math.floor(new Date(2026, 10, 9, 23, 13).getTime() / 1000);
  const rows = P.stampRows([
    { sep }, { id: "a", clock: "16:23" }, { id: "b", pre }, { id: "c", clock: "" },
  ]);
  assert.equal(rows[0].ts, sep + (16 * 60 + 23) * 60);
  assert.equal(rows[1].ts, pre);
  assert.equal(rows[2].ts, pre); // saatsiz medya: öncekinin zamanı
  const lead = P.stampRows([{ id: "x" }, { id: "y", pre }]);
  assert.equal(lead[0].ts, pre); // baştaki bilinmeyen: sonrakinin zamanı
});
test("chunk ve mostCommon", () => {
  assert.deepEqual(P.chunk([1, 2, 3, 4, 5], 2), [[1, 2], [3, 4], [5]]);
  assert.equal(P.mostCommon(["a", "b", "b", "", "a", "b"]), "b");
});
