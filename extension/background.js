// Barney Chrome uzantısı — service worker.
// Yerel Barney sunucusuna WebSocket ile bağlanır; WhatsApp sekmelerinden gelen olayları iletir,
// sunucunun eylemlerini (öner/gönder) ilgili sekmeye yollar. Burada yapay zekâ yoktur.
import { DEFAULTS } from "./config.js";

const SUBPROTO = "barney.v1";
let ws = null;
let connecting = false;
let backoff = 1000;
let nextAllowed = 0;
let status = { connected: false, lastError: "", paused: false, autoSend: false, callsToday: 0, sentHour: 0 };
const chatTab = new Map(); // sohbet kimliği → son bildiren sekme

async function getConfig() {
  const s = await chrome.storage.local.get(["baseUrl", "key"]);
  return { baseUrl: (s.baseUrl || DEFAULTS.baseUrl).replace(/\/+$/, ""), key: s.key || DEFAULTS.key };
}

function applyStats(st) {
  if (!st) return;
  status.paused = !!st.paused;
  status.autoSend = !!st.auto_send;
  status.callsToday = st.calls_today || 0;
  status.sentHour = st.sent_last_hour || 0;
}

// Anahtarla /api/state'e bakar: yalnızca gerçek Barney sunucusu 200 döner.
async function probe(baseUrl, key) {
  try {
    const ctl = new AbortController();
    const t = setTimeout(() => ctl.abort(), 2500);
    const r = await fetch(baseUrl + "/api/state", { headers: { Authorization: "Bearer " + key }, signal: ctl.signal, cache: "no-store" });
    clearTimeout(t);
    return r.ok;
  } catch { return false; }
}

async function connect(force = false) {
  if (connecting || (ws && ws.readyState <= 1)) return;
  if (!force && Date.now() < nextAllowed) return;
  connecting = true;
  const { baseUrl, key } = await getConfig();
  if (!key) {
    connecting = false;
    status = { ...status, connected: false, lastError: "Eşleşme anahtarı yok (açılır pencereden gir ya da `barney extension install` çalıştır)." };
    return;
  }
  // Önce sunucuyu HTTP ile yokla: kapalıysa ya da 8777'de başka bir uygulama varsa WebSocket hiç açılmaz;
    // böylece Chrome "WebSocket connection failed" hatası kaydetmez (uzantı Errors sayfası temiz kalır).
  if (!(await probe(baseUrl, key))) {
    connecting = false;
    status = { ...status, connected: false, lastError: "Barney sunucusu bulunamadı (barney serve çalışıyor mu? port/anahtar doğru mu?)" };
    nextAllowed = Date.now() + backoff;
    backoff = Math.min(backoff * 2, 15000);
    return;
  }
  try {
    const sock = new WebSocket(baseUrl.replace(/^http/, "ws") + "/ws", [SUBPROTO, key]);
    ws = sock;
    sock.onopen = () => { connecting = false; backoff = 1000; status.connected = true; status.lastError = ""; };
    sock.onmessage = (ev) => onServerFrame(ev.data);
    sock.onclose = () => {
      connecting = false;
      if (ws === sock) { ws = null; status.connected = false; }
      nextAllowed = Date.now() + backoff;
      backoff = Math.min(backoff * 2, 30000);
    };
    sock.onerror = () => { status.lastError = "Sunucuya bağlanılamadı (barney serve çalışıyor mu?)"; };
  } catch (e) {
    connecting = false;
    status.lastError = String(e);
  }
}

function sendFrame(frame) {
  if (!ws || ws.readyState !== 1) return false;
  ws.send(JSON.stringify(frame));
  return true;
}

async function onServerFrame(raw) {
  let f;
  try { f = JSON.parse(raw); } catch { return; }
  if (f.stats) {
    applyStats(f.stats);
    // Sunucudaki durdur/sürdür durumu (arayüzden de değişebilir) içerik betiğiyle eşitlenir.
    chrome.storage.local.get(["enabled"]).then((s) => {
      if ((s.enabled !== false) === !!f.stats.paused) chrome.storage.local.set({ enabled: !f.stats.paused });
    });
  }
  if (f.type === "notify") {
    chrome.notifications.create({ type: "basic", iconUrl: "icons/icon128.png", title: f.title || "Barney", message: f.body || "" });
    return;
  }
  if (f.type === "command") {
    // Sunucudan okuma/sohbet açma komutu: WhatsApp sekmesine ilet (yoksa hata dön).
    const tabs = await chrome.tabs.query({ url: "https://web.whatsapp.com/*" });
    const t = tabs.find((x) => x.active) || tabs[0];
    if (!t) { sendFrame({ type: "command_error", job_id: f.job_id, error: "WhatsApp Web sekmesi açık değil" }); return; }
    chrome.tabs.sendMessage(t.id, { t: "command", job_id: f.job_id, cmd: f.cmd, args: f.args || {} }).catch(() => {
      sendFrame({ type: "command_error", job_id: f.job_id, error: "WhatsApp sekmesi yanıt vermedi: sayfayı yenile" });
    });
    return;
  }
  if (f.type !== "action" || !f.action) return;
  const tabId = chatTab.get(f.action.chat_id);
  const tabs = tabId != null ? [{ id: tabId }] : await chrome.tabs.query({ url: "https://web.whatsapp.com/*" });
  for (const t of tabs) chrome.tabs.sendMessage(t.id, { t: "action", action: f.action }).catch(() => {});
}

chrome.runtime.onMessage.addListener((msg, sender, respond) => {
  if (msg.t === "frame") {
    if (msg.frame && msg.frame.chat && sender.tab) chatTab.set(msg.frame.chat.id, sender.tab.id);
    connect();
    respond({ ok: sendFrame(msg.frame) });
  } else if (msg.t === "status") {
    respond(status);
  } else if (msg.t === "reconnect") {
    connect(true).then(() => respond({ ok: true }));
    return true;
  } else if (msg.t === "pause" || msg.t === "resume") {
    sendFrame({ type: msg.t });
    respond({ ok: true });
  }
  return false;
});

chrome.alarms.create("keepalive", { periodInMinutes: 0.5 });
chrome.alarms.onAlarm.addListener((a) => {
  if (a.name !== "keepalive") return;
  if (!sendFrame({ type: "state" })) connect();
});
chrome.runtime.onInstalled.addListener(() => connect(true));
chrome.runtime.onStartup.addListener(() => connect(true));
connect();
