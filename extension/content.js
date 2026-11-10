// Barney — WhatsApp Web içerik betiği.
// İzleme olay tabanlıdır (MutationObserver + kısa debounce); yapay zekâ çağrısı yoktur, yalnızca
// "yeni karşı taraf mesajı" bildirilir. Seçiciler tek yerdedir (SEL): WhatsApp arayüzü değişirse
// yalnızca burası güncellenir.
(() => {
  "use strict";
  const P = globalThis.BarneyPure;
  if (!P || window.__barneyLoaded) return;
  window.__barneyLoaded = true;

  // Seçiciler gerçek WhatsApp Web DOM'undan alındı (data-testid tabanlı; hash'li sınıflar kullanılmaz).
  const SEL = {
    main: "[data-testid='conversation-panel-messages'], #main",
    title: "[data-testid='conversation-info-header-chat-title'], [data-testid='conversation-header'] span[title]",
    rows: "[data-testid^='conv-msg-']",
    pre: "[data-pre-plain-text]",
    text: "[data-testid='selectable-text']",
    tail: "[data-icon='tail-out']",
    tailIn: "[data-icon='tail-in']",
    meta: "[data-testid='msg-meta']",
    composer: "[data-testid='conversation-compose-box-input'], footer [contenteditable='true'][role='textbox']",
    panel: "[data-testid='conversation-panel-messages']",
    oldMsgs: "[data-testid='conversation-panel-messages'] button",
    // Sol liste/arama: DOM örneği henüz doğrulanmadı (kullanıcı yapıştırınca sabitlenecek) — en iyi tahmin.
    side: "#pane-side",
    sideTitle: "#pane-side span[title]",
    search: "#side [contenteditable='true'][role='textbox'], #side input[type='text']",
    sendBtn: "footer button[aria-label='Gönder'], footer button[aria-label='Send']",
  };
  const MAX_HISTORY = 40;
  const CPS = 5; // varsayılan yazma hızı (sunucu bekleme süresi zaten yazmayı içerir)

  const st = { enabled: true, seen: null, running: false, abort: false };
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

  // ---------------------------------------------------------------- okuma

  // Son 2 dakikada bu uzantının yazıp gönderdiği metinler.
  const mine = new Map();
  const normText = (t) => t.replace(/\s+/g, " ").trim().toLowerCase();
  function markMine(text) { mine.set(normText(text), Date.now()); }
  function isMine(text) {
    const k = normText(text), at = mine.get(k);
    if (!at) return false;
    if (Date.now() - at > 120000) { mine.delete(k); return false; }
    return true;
  }

  function readChat() {
    const main = document.querySelector(SEL.main);
    if (!main) return null;
    const titleEl = document.querySelector(SEL.title);
    const partner = titleEl ? (titleEl.getAttribute("title") || titleEl.textContent || "").trim() : "";
    if (!partner) return null;
    const rows = Array.from(document.querySelectorAll(SEL.rows)).slice(-MAX_HISTORY);
    const history = [];
    for (const row of rows) {
      const preEl = row.querySelector(SEL.pre);
      const meta = P.parsePrePlain(preEl && preEl.getAttribute("data-pre-plain-text"));
      const textEl = row.querySelector(SEL.text);
      const text = textEl ? textEl.innerText.trim() : "";
      const metaEl = row.querySelector(SEL.meta);
      const label = row.querySelector("span[aria-label$=':']");
      const from = P.direction({
        tailOut: !!row.querySelector(SEL.tail),
        tailIn: !!row.querySelector(SEL.tailIn),
        ticks: !!(metaEl && metaEl.querySelector("[aria-label]")),
        youLabel: !!label && /^(siz|you):?$/i.test(label.getAttribute("aria-label").trim()),
        sender: meta && meta.sender,
        partner,
      });
      const ts = meta ? meta.ts : 0;
      const id = row.getAttribute("data-testid") || "";
      // Az önce bizim gönderdiğimiz mesaj: tik/gönderici bilgisi henüz oluşmamış olabilir; asla "karşı taraf" sayılmaz.
      if (from === "them" && text && isMine(text) && !row.querySelector(SEL.tailIn)) { history.push({ from: "me", text, ts, id }); continue; }
      history.push(text ? { from, text, ts, id } : { from, text: "", kind: "media", ts, id });
    }
    return { id: partner, partner, history, incoming_key: P.incomingKey(history) };
  }

  // Uzantı yenilenince/kaldırılınca bu betik "yetim" kalır: chrome.* çağrıları hata fırlatır.
  // O anda her şeyi kapatırız (zamanlayıcı, gözlemci, panel); sekme yenilenince yeni betik yüklenir.
  let dead = false;
  const cleanups = [];
  function alive() {
    if (dead) return false;
    try { if (chrome.runtime && chrome.runtime.id) return true; } catch { /* aşağıda kapat */ }
    dead = true; st.enabled = false; st.abort = true; imp.abort = true;
    for (const f of cleanups) { try { f(); } catch { /* yoksay */ } }
    window.__barneyLoaded = false;
    return false;
  }

  function send(frame) {
    if (!alive()) return;
    try { return chrome.runtime.sendMessage({ t: "frame", frame: { platform: "whatsapp", ...frame } }).catch(() => {}); } catch { /* uzantı yeniden yüklendi */ }
  }

  let timer = null;
  function schedule() {
    if (timer) clearTimeout(timer);
    timer = setTimeout(check, 500); // art arda DOM değişimlerini tek okumada topla
  }

  function check() {
    timer = null;
    if (!alive() || !st.enabled || imp.busy) return;
    const chat = readChat();
    if (!chat) return;
    const r = P.shouldNotify(st.seen, chat.id, chat.incoming_key, chat.history);
    st.seen = r.next;
    const fp = chat.id + "|" + P.hash(chat.history.map((m) => m.from + ":" + (m.text || m.kind || "")).join("\n"));
    if (r.notify) { st.fp = fp; send({ type: "incoming", chat }); setStatus("Yeni mesaj, sunucu düşünüyor…"); return; }
    // Düzenlenen/silinen/benim attığım mesaj: yalnız canlı görünümü tazele (yapay zekâ çağrılmaz).
    if (fp !== st.fp) { st.fp = fp; send({ type: "live_update", chat }); }
  }

  // ---------------------------------------------------------------- yazma / gönderme

  const composer = () => document.querySelector(SEL.composer);

  async function typeText(text, rnd = Math.random) {
    const c = composer();
    if (!c) throw new Error("mesaj kutusu bulunamadı");
    c.focus();
    for (const ch of Array.from(text.replace(/\s*\n\s*/g, " "))) {
      if (st.abort || !st.enabled) throw new Error("durduruldu");
      document.execCommand("insertText", false, ch);
      await sleep(P.typingDelayMs(CPS, rnd()));
    }
  }

  // Kutu boşken aynı konumda "Sesli mesaj" düğmesi var: yalnız kutuda metin varken ve düğmenin
  // etiketi Gönder/Send iken tıklanır; asla ses düğmesine basılmaz.
  async function pressSend() {
    const c = composer();
    if (!c || !(c.innerText || "").trim()) throw new Error("kutu boş, gönderilmedi");
    let b = null;
    for (let i = 0; i < 10 && !b; i++) { b = document.querySelector(SEL.sendBtn); if (!b) await sleep(100); }
    if (b) { b.click(); return; }
    c.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", keyCode: 13, which: 13, bubbles: true }));
  }

  // Adımları (bekle → yaz → gönder) sırayla yürütür. Her adımdan önce güvenlik denetimleri yapılır.
  async function runSteps(action, { userInitiated }) {
    if (st.running) return;
    st.running = true; st.abort = false;
    try {
      for (let i = 0; i < action.steps.length; i++) {
        const step = action.steps[i];
        const typingMs = Array.from(step.text).length * (1000 / CPS);
        await sleepAbortable(Math.max(0, step.wait_sec * 1000 - typingMs));
        const why = unsafeReason(action, userInitiated);
        if (why) return abortSend(action.chat_id, why);
        markMine(step.text);
        await typeText(step.text);
        const again = unsafeReason(action, userInitiated, true);
        if (again) { clearComposer(); return abortSend(action.chat_id, again); }
        await pressSend();
        await sleep(300);
        send({ type: "sent", chat_id: action.chat_id, index: i, text: step.text });
      }
      setStatus("Gönderildi ✓");
    } catch (e) {
      abortSend(action.chat_id, String(e.message || e));
    } finally { st.running = false; }
  }

  async function sleepAbortable(ms) {
    const end = Date.now() + ms;
    while (Date.now() < end) {
      if (st.abort || !st.enabled) throw new Error("durduruldu");
      await sleep(Math.min(250, end - Date.now()));
    }
  }

  function unsafeReason(action, userInitiated, afterTyping = false) {
    if (!st.enabled) return "uzantı durduruldu";
    const chat = readChat();
    if (!chat || chat.id !== action.chat_id) return "sohbet değişti";
    // Eylem kararından sonra yeni bir karşı taraf mesajı geldiyse cevap bayat.
    if (chat.incoming_key !== action.incoming_key) return "yeni mesaj geldi, cevap bayatladı";
    const c = composer();
    if (!c) return "mesaj kutusu yok";
    const draft = (c.innerText || "").trim();
    if (!afterTyping && draft) return "mesaj kutusunda taslak var (sen yazıyorsun)";
    return "";
  }

  // Arayüzden "Kutuya yaz": metni yalnızca mesaj kutusuna yazar, göndermez. Kutuda taslak varsa dokunmaz.
  async function writeDraft(a) {
    try {
      const chat = readChat();
      if (!chat || chat.id !== a.chat_id) return setStatus("Taslak yazılmadı: sohbet değişti");
      const c = composer();
      if (!c) return setStatus("Taslak yazılmadı: mesaj kutusu yok");
      if ((c.innerText || "").trim()) return setStatus("Taslak yazılmadı: kutuda zaten yazı var");
      st.abort = false;
      await typeText(a.steps.map((s) => s.text).join(" "));
      setStatus("Taslak kutuda — göndermek sende");
    } catch (e) { setStatus(String(e.message || e)); }
  }

  function clearComposer() {
    const c = composer();
    if (!c) return;
    c.focus();
    document.execCommand("selectAll", false);
    document.execCommand("delete", false);
  }

  function abortSend(chatId, reason) {
    send({ type: "aborted", chat_id: chatId, reason });
    setStatus("Gönderim iptal: " + reason);
  }


  // ---------------------------------------------------------------- tarayıcıdan okuma / sohbet açma
  // Güvenlik: bu bölüm yalnızca kaydırır, sol listeden sohbet seçer, ARAMA kutusuna yazar ve okur.
  // Mesaj kutusuna (footer), gönder/ses/ek düğmelerine asla dokunmaz (guard).

  const imp = { abort: false, busy: false };
  const rnd = (a, b) => a + Math.random() * (b - a);
  const FORBIDDEN_LABEL = /(gönder|send|sesli mesaj|voice message|ekle|attach|emoji|sil|delete|arşivle|archive|engelle|block)/i;

  function guard(el) {
    if (!el) throw new Error("öğe bulunamadı");
    if (el.closest("footer")) throw new Error("güvenlik: mesaj kutusuna dokunulamaz");
    const lab = (el.getAttribute("aria-label") || "") + " " + (el.closest("button")?.getAttribute("aria-label") || "");
    if (FORBIDDEN_LABEL.test(lab)) throw new Error("güvenlik: yasaklı düğme (" + lab.trim() + ")");
    return el;
  }

  function emitCmd(frame) { return send(frame); }

  function dayLabelOf(span) {
    if (span.closest("[data-testid^='conv-msg-']")) return null;
    return P.parseDayLabel(span.textContent || "", new Date());
  }

  // Açık sohbetin şu an DOM'daki mesajlarını sırayla okur.
  function snapshotRows() {
    const panel = document.querySelector(SEL.panel);
    if (!panel) return [];
    const partner = (document.querySelector(SEL.title)?.textContent || "").trim();
    const items = [];
    for (const el of panel.querySelectorAll("[data-testid^='conv-msg-'], span[dir='auto']")) {
      if (el.matches("span[dir='auto']")) {
        const sep = dayLabelOf(el);
        if (sep) items.push({ sep });
        continue;
      }
      const preEl = el.querySelector(SEL.pre);
      const meta = P.parsePrePlain(preEl && preEl.getAttribute("data-pre-plain-text"));
      const textEl = el.querySelector(SEL.text);
      const text = textEl ? textEl.innerText.trim() : "";
      const metaEl = el.querySelector(SEL.meta);
      const clockEl = metaEl && Array.from(metaEl.querySelectorAll("span")).map((x) => x.textContent.trim()).find((t) => P.parseClock(t) != null);
      const label = el.querySelector("span[aria-label$=':']");
      const from = P.direction({
        tailOut: !!el.querySelector(SEL.tail), tailIn: !!el.querySelector(SEL.tailIn),
        ticks: !!(metaEl && metaEl.querySelector("[aria-label]")),
        youLabel: !!label && /^(siz|you):?$/i.test(label.getAttribute("aria-label").trim()),
        sender: meta && meta.sender, partner,
      });
      items.push({ id: el.getAttribute("data-testid"), from, text, kind: text ? "text" : "media", pre: meta ? meta.ts : 0, clock: clockEl || "", sender: meta ? meta.sender : "" });
    }
    return P.stampRows(items);
  }

  async function waitFor(fn, ms, step = 200) {
    const end = Date.now() + ms;
    while (Date.now() < end) {
      if (imp.abort) throw new Error("durduruldu");
      const v = fn();
      if (v) return v;
      await sleep(step);
    }
    return null;
  }

  async function scrapeHistory(jobId, a) {
    const panel = document.querySelector(SEL.panel);
    if (!panel) throw Object.assign(new Error("mesaj paneli bulunamadı (WhatsApp arayüzü değişmiş olabilir)"), { fallback: true });
    const partner = (document.querySelector(SEL.title)?.textContent || "").trim();
    if (!partner) throw Object.assign(new Error("sohbet adı okunamadı"), { fallback: true });
    const maxMsgs = a.all ? Infinity : Math.max(50, a.max_messages || 3000);
    const deadline = Date.now() + Math.max(15, a.max_seconds || 300) * 1000;
    const startTop = panel.scrollTop;
    const seen = new Map(); // id → kayıt
    const take = () => { for (const r of snapshotRows()) if (r.id && !seen.has(r.id)) seen.set(r.id, r); };
    let stable = 0, note = "";
    take();
    while (!imp.abort) {
      emitCmd({ type: "history_progress", job_id: jobId, loaded: seen.size, note: "okunuyor…", chat_name: partner });
      if (seen.size >= maxMsgs) { note = "istenen mesaj sayısına ulaşıldı"; break; }
      if (Date.now() > deadline) { note = "süre doldu"; break; }
      if (Array.from(document.querySelectorAll(SEL.oldMsgs)).some((b) => /eski mesajları|older messages/i.test(b.textContent || ""))) {
        note = "daha eski mesajlar telefondan senkron gerektiriyor"; break;
      }
      const before = { n: seen.size, h: panel.scrollHeight };
      panel.scrollTop = 0;
      await sleep(rnd(a.slow ? 800 : 300, a.slow ? 2000 : 700));
      try { await waitFor(() => panel.scrollHeight !== before.h, 5000, 250); } catch (e) { if (!imp.abort) throw e; }
      if (imp.abort) { take(); break; }
      take();
      if (seen.size === before.n && panel.scrollHeight === before.h) { if (++stable >= 3) { note = "sohbetin başına ulaşıldı"; break; } } else stable = 0;
    }
    if (imp.abort) note = "durduruldu, o ana kadar okunanlar kaydedildi"; // okunanlar atılmaz
    const r = finishRows(jobId, seen, a, partner);
    panel.scrollTop = a.keepTop ? startTop : panel.scrollHeight; // eski yerine/sona dön
    return { loaded: r.loaded, note: note || "tamamlandı", fallback: r.loaded < 5 };
  }

  // Toplanan satırları kronolojik sıralayıp ≤300'lük parçalarla sunucuya yollar.
  function finishRows(jobId, seen, a, partner) {
    const maxMsgs = a.all ? Infinity : Math.max(50, a.max_messages || 3000);
    let rows = Array.from(seen.values()).map((r, i) => ({ ...r, _i: i }));
    rows.sort((x, y) => x.ts - y.ts || y._i - x._i);
    if (rows.length > maxMsgs) rows = rows.slice(rows.length - maxMsgs);
    const owner = P.mostCommon(rows.filter((r) => r.from === "me").map((r) => r.sender));
    const msgs = rows.map((r) => ({ from: r.from, text: r.text, ts: r.ts, kind: r.kind }));
    const parts = P.chunk(msgs, 300);
    parts.forEach((m, i) => emitCmd({ type: "history_chunk", job_id: jobId, seq: i, partner, owner, messages: m, done: i === parts.length - 1 }));
    return { loaded: msgs.length };
  }

  function sideRows() {
    const side = document.querySelector(SEL.side);
    if (!side) return [];
    return Array.from(side.querySelectorAll("[role='row'], [role='listitem']")).map((row) => {
      const t = row.querySelector("span[title]");
      if (!t) return null;
      const spans = Array.from(row.querySelectorAll("span[dir='ltr'], span[dir='auto']")).map((x) => x.textContent.trim()).filter(Boolean);
      const unreadEl = row.querySelector("span[aria-label*='okunmamış'], span[aria-label*='unread']");
      return { row, name: (t.getAttribute("title") || t.textContent || "").trim(), last: spans.find((x) => x !== t.textContent.trim()) || "", unread: parseInt((unreadEl?.textContent || "0").replace(/\D/g, ""), 10) || 0 };
    }).filter(Boolean);
  }

  async function listChats(jobId) {
    const side = document.querySelector(SEL.side);
    if (!side) throw Object.assign(new Error("sohbet listesi bulunamadı (sol panel seçicileri doğrulanmadı)"), { fallback: true });
    const all = new Map();
    const top = side.scrollTop;
    side.scrollTop = 0;
    await sleep(500);
    for (let i = 0; i < 8 && !imp.abort; i++) {
      for (const r of sideRows()) if (!all.has(r.name)) all.set(r.name, { name: r.name, last: r.last, unread: r.unread });
      const before = side.scrollTop;
      side.scrollTop += side.clientHeight * 0.8;
      await sleep(rnd(500, 900));
      if (side.scrollTop === before) break;
    }
    side.scrollTop = top;
    const chats = Array.from(all.values()).slice(0, 80);
    emitCmd({ type: "chats", job_id: jobId, chats });
    return { loaded: chats.length, note: chats.length + " sohbet" };
  }

  async function openChat(name) {
    const wanted = P.mostCommon([name]);
    const norm = (x) => (x || "").trim().toLowerCase();
    const find = () => sideRows().find((r) => norm(r.name) === norm(wanted));
    let hit = find();
    if (!hit) { // arama kutusunu kullan (yalnız arama alanına yazılır)
      const box = document.querySelector(SEL.search);
      guard(box);
      box.focus();
      document.execCommand("selectAll", false);
      document.execCommand("insertText", false, wanted);
      hit = await waitFor(find, 6000, 300);
    }
    if (!hit) throw new Error(`"${wanted}" sohbeti bulunamadı`);
    guard(hit.row).scrollIntoView({ block: "nearest" });
    hit.row.querySelector("span[title]").click();
    const ok = await waitFor(() => norm(document.querySelector(SEL.title)?.textContent) === norm(wanted), 8000, 250);
    const box = document.querySelector(SEL.search);
    if (box && (box.innerText || box.value || "").trim()) { // aramayı temizle
      box.focus(); document.execCommand("selectAll", false); document.execCommand("delete", false);
    }
    if (!ok) throw new Error(`"${wanted}" açılamadı`);
    await sleep(rnd(600, 1200));
    return { note: "açıldı" };
  }


  // ---- AI modu primitifleri (yalnız beyaz listeli eylemler; model çıktısına güvenilmez)
  const aiSeen = new Map();
  let aiChat = ""; // aiSeen'in ait olduğu sohbet: sohbet değişince karışmasın
  const curTitle = () => (document.querySelector(SEL.title)?.textContent || "").trim();
  let aiRefs = new Map();

  function aiSnapshot(withText) {
    const panel = document.querySelector(SEL.panel);
    aiRefs = new Map();
    const side = sideRows().slice(0, 25).map((r, i) => { const ref = "c" + i; aiRefs.set(ref, r); return { ref, name: r.name }; });
    return {
      title: (document.querySelector(SEL.title)?.textContent || "").trim(),
      side_rows: side,
      panel_top: panel ? Math.round(panel.scrollTop) : 0,
      panel_height: panel ? panel.scrollHeight : 0,
      panel_rows: panel ? panel.querySelectorAll("[data-testid^='conv-msg-']").length : 0,
      at_top: panel ? panel.scrollTop <= 2 : false,
      has_search: !!document.querySelector(SEL.search),
      text_tail: withText && panel ? (panel.innerText || "").slice(-6000) : undefined,
      read_total: aiSeen.size,
    };
  }

  async function aiAct(a) {
    const panel = document.querySelector(SEL.panel);
    const side = document.querySelector(SEL.side);
    switch (a.action) {
      case "scroll_up": { if (!panel) throw new Error("panel yok"); const h = panel.scrollHeight; panel.scrollTop = 0; await sleep(rnd(900, 1800)); await waitFor(() => panel.scrollHeight !== h, 4000, 250); return "yukarı kaydırıldı"; }
      case "scroll_down": { if (!panel) throw new Error("panel yok"); panel.scrollTop += panel.clientHeight * 0.9; await sleep(rnd(500, 900)); return "aşağı kaydırıldı"; }
      case "scroll_side_down": { if (!side) throw new Error("liste yok"); side.scrollTop += side.clientHeight * 0.8; await sleep(rnd(500, 900)); return "liste kaydırıldı"; }
      case "click_chat": {
        const r = aiRefs.get(a.ref);
        if (!r) throw new Error("ref geçersiz");
        guard(r.row).scrollIntoView({ block: "nearest" });
        r.row.querySelector("span[title]").click();
        await sleep(rnd(800, 1400));
        return "sohbet seçildi: " + r.name;
      }
      case "type_search": {
        const t = String(a.text || "").trim().slice(0, 60);
        if (!t) throw new Error("metin boş");
        const box = guard(document.querySelector(SEL.search));
        box.focus(); document.execCommand("selectAll", false); document.execCommand("insertText", false, t);
        await sleep(rnd(800, 1400));
        return "arama yazıldı";
      }
      case "clear_search": {
        const box = guard(document.querySelector(SEL.search));
        box.focus(); document.execCommand("selectAll", false); document.execCommand("delete", false);
        return "arama temizlendi";
      }
      case "read": {
        if (aiChat !== curTitle()) { aiSeen.clear(); aiChat = curTitle(); }
        const before = aiSeen.size;
        for (const r of snapshotRows()) if (r.id && !aiSeen.has(r.id)) aiSeen.set(r.id, r);
        return `okundu: toplam ${aiSeen.size} (+${aiSeen.size - before})`;
      }
    }
    throw new Error("izin verilmeyen eylem: " + a.action);
  }

  async function runCommand(msg) {
    const { job_id: id, cmd, args: a = {} } = msg;
    if (cmd === "stop") { imp.abort = true; return; }
    if (cmd === "resync") { // yeşil "Senk": yalnızca şu an ekranda yüklü mesajlar; kaydırma yok, meşgulken de çalışır
      const chat = readChat();
      if (!chat) { setStatus("Senk: açık sohbet yok"); return; }
      const rows = snapshotRows().filter((r) => r.id);
      const messages = rows.map((r) => ({ from: r.from, text: r.text, ts: r.ts, kind: r.kind }));
      const owner = P.mostCommon(rows.filter((r) => r.from === "me").map((r) => r.sender));
      send({ type: "sync_live", chat, messages, partner: chat.partner, owner });
      setStatus("Senk: " + messages.length + " mesaj okundu");
      return;
    }
    if (imp.busy) { emitCmd({ type: "command_error", job_id: id, error: "uzantı başka bir komutla meşgul" }); return; }
    imp.busy = true; imp.abort = false;
    setStatus("Okuma: " + cmd + "…");
    try {
      let r;
      if (cmd === "scrape_current") r = await scrapeHistory(id, a);
      else if (cmd === "list_chats") r = await listChats(id);
      else if (cmd === "open_chat") r = await openChat(a.name);
      else if (cmd === "ai_snapshot") r = { data: aiSnapshot(!!a.with_text) };
      else if (cmd === "ai_act") { const note = await aiAct(a); r = { note }; }
      else if (cmd === "ai_collect") {
        const partner = curTitle();
        if (aiChat !== partner) aiSeen.clear(); // başka sohbetin mesajları bu sohbete yazılmaz
        const f = finishRows(id, aiSeen, a, partner);
        aiSeen.clear(); aiChat = "";
        r = { loaded: f.loaded, note: f.loaded + " mesaj" };
      }
      else throw new Error("bilinmeyen komut: " + cmd);
      emitCmd({ type: "command_result", job_id: id, ok: true, loaded: r.loaded || 0, note: r.note || "", fallback: !!r.fallback, data: r.data });
      setStatus("Okuma bitti: " + (r.note || ""));
    } catch (e) {
      emitCmd({ type: "command_error", job_id: id, error: String(e.message || e), fallback: !!e.fallback });
      setStatus("Okuma hatası: " + (e.message || e));
    } finally { imp.busy = false; schedule(); }
  }

  // ---------------------------------------------------------------- sunucudan eylem

  chrome.runtime.onMessage.addListener((msg) => {
    if (msg.t === "command") { runCommand(msg); return; }
    if (msg.t !== "action") return;
    const a = msg.action;
    if (!st.enabled && a.kind !== "draft") return;
    if (a.kind === "send") { setStatus("Cevap gönderiliyor…"); runSteps(a, { userInitiated: false }); }
    else if (a.kind === "draft") writeDraft(a);
    else if (a.kind === "suggest") renderSuggestions(a);
    else if (a.kind === "skip") setStatus("Atlandı: " + (a.reason || ""));
    else if (a.kind === "error") setStatus("Hata: " + (a.reason || ""));
  });

  // ---------------------------------------------------------------- panel (gölge DOM)

  const host = document.createElement("div");
  host.style.cssText = "position:fixed;right:16px;bottom:16px;z-index:2147483647;";
  const root = host.attachShadow({ mode: "open" });
  root.innerHTML = `
  <style>
    .p{width:300px;background:#0B1026;color:#E6EAF7;border:1px solid #1F2A5C;border-radius:12px;font:13px system-ui,sans-serif;box-shadow:0 8px 30px #0008;overflow:hidden}
    .h{display:flex;align-items:center;gap:8px;padding:8px 10px;background:#121A3A;border-bottom:1px solid #1F2A5C;cursor:pointer}
    .h b{color:#E8C547}.h span{flex:1;color:#8D97C4;font-size:12px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    .b{padding:8px 10px;max-height:340px;overflow:auto}.b.c{display:none}
    button{background:#5B7CFF;color:#fff;border:0;border-radius:6px;padding:5px 8px;cursor:pointer;font:inherit}
    button.g{background:#121A3A;border:1px solid #1F2A5C}button.r{background:#FF6B7A}
    .cand{border:1px solid #1F2A5C;border-radius:8px;padding:8px;margin-bottom:8px;background:#121A3A}
    .cand h4{margin:0 0 4px;font-size:12px;color:#E8C547;text-transform:uppercase}.cand .m{margin:2px 0}
    .cand .a{display:flex;gap:6px;margin-top:6px}.warn{color:#FF6B7A;font-size:12px}.star{color:#4ADE80}
  </style>
  <div class="p"><div class="h" id="h"><b>Barney</b><span id="s">Hazır</span><button class="r" id="stop" title="Durdur / Sürdür">■</button><button class="g" id="close" title="Paneli gizle (uzantı simgesinden geri getirilir)">✕</button></div>
  <div class="b c" id="b"><button class="g" id="now">Şimdi öner</button><div id="list"></div></div></div>`;
  document.documentElement.appendChild(host);
  const $ = (id) => root.getElementById(id);

  function setStatus(t) { $("s").textContent = t; }
  $("h").onclick = (e) => { if (e.target.id !== "stop" && e.target.id !== "close") $("b").classList.toggle("c"); };
  $("stop").onclick = async () => {
    if (!alive()) return;
    if (!st.enabled) { // durdurulmuşsa bu düğme sürdürür
      await chrome.storage.local.set({ enabled: true }).catch(() => {});
      chrome.runtime.sendMessage({ t: "resume" }).catch(() => {});
      $("stop").textContent = "■"; $("stop").title = "Durdur";
      setStatus("Sürdürüldü");
      return;
    }
    st.abort = true;
    await chrome.storage.local.set({ enabled: false }).catch(() => {});
    chrome.runtime.sendMessage({ t: "pause" }).catch(() => {});
    $("stop").textContent = "▶"; $("stop").title = "Sürdür";
    setStatus("DURDURULDU — ▶ ile sürdür");
  };
  $("close").onclick = async () => { host.style.display = "none"; if (!alive()) return; await chrome.storage.local.set({ panelHidden: true }).catch(() => {}); };
  $("now").onclick = () => {
    const chat = readChat();
    if (!chat) return setStatus("Açık sohbet okunamadı");
    send({ type: "manual", chat });
    setStatus("İstendi, düşünüyor…");
  };

  function renderSuggestions(a) {
    const list = $("list");
    list.textContent = "";
    $("b").classList.remove("c");
    (a.cands || []).forEach((c, i) => {
      const d = document.createElement("div");
      d.className = "cand";
      const h = document.createElement("h4");
      h.textContent = c.tur + (i === a.recommended ? " ★" : "") + (c.blocked ? " (Vicdan engelledi)" : "");
      d.appendChild(h);
      (c.steps || []).forEach((s) => { const m = document.createElement("div"); m.className = "m"; m.textContent = s.text; d.appendChild(m); });
      (c.violations || []).forEach((v) => { const w = document.createElement("div"); w.className = "warn"; w.textContent = v; d.appendChild(w); });
      if (!c.blocked) {
        const act = document.createElement("div"); act.className = "a";
        const w = document.createElement("button"); w.className = "g"; w.textContent = "Kutuya yaz";
        w.onclick = async () => { try { clearComposer(); await typeText(c.steps[0].text); } catch (e) { setStatus(String(e.message || e)); } };
        const g = document.createElement("button"); g.textContent = "Sırayla gönder";
        g.onclick = () => runSteps({ chat_id: a.chat_id, incoming_key: a.incoming_key, steps: c.steps }, { userInitiated: true });
        act.append(w, g); d.appendChild(act);
      }
      list.appendChild(d);
    });
    setStatus(a.reason ? a.reason : "Öneriler hazır");
  }

  // ---------------------------------------------------------------- başlat

  chrome.storage.local.get(["enabled", "panelHidden"]).then((s) => {
    st.enabled = s.enabled !== false;
    if (!st.enabled) { $("stop").textContent = "▶"; $("stop").title = "Sürdür"; setStatus("DURDURULDU — ▶ ile sürdür"); }
    if (s.panelHidden) host.style.display = "none";
  }).catch(() => {});
  // Uzantı yenilenince eski betiğin bekleyen chrome.* çağrıları "Extension context invalidated" ile reddedilir;
  // bunu hata olarak kaydettirme, betiği kapat.
  window.addEventListener("unhandledrejection", (e) => {
    if (/context invalidated/i.test(String((e.reason && e.reason.message) || e.reason))) { e.preventDefault(); alive(); }
  });
  chrome.storage.onChanged.addListener((ch) => {
    if (ch.panelHidden) host.style.display = ch.panelHidden.newValue ? "none" : ""; if (ch.enabled) { st.enabled = ch.enabled.newValue !== false; if (!st.enabled) st.abort = true; } });
  const obs = new MutationObserver(() => { if (alive()) schedule(); });
  obs.observe(document.body, { childList: true, subtree: true });
  const tick = setInterval(check, 5000);
  cleanups.push(() => obs.disconnect(), () => clearInterval(tick), () => { if (timer) clearTimeout(timer); }, () => host.remove()); // yedek: gözden kaçan değişiklikler (ör. düzenleme) 5 sn içinde yakalanır
  schedule();
})();
