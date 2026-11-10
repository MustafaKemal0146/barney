import { DEFAULTS } from "./config.js";
const $ = (id) => document.getElementById(id);
const ask = (m) => chrome.runtime.sendMessage(m);

async function refresh() {
  const s = await ask({ t: "status" });
  $("dot").classList.toggle("on", s.connected);
  $("conn").textContent = s.connected ? "Bağlı" : "Bağlı değil";
  $("mode").textContent = s.paused ? "DURDURULDU" : s.autoSend ? "Otomatik gönderim" : "Yalnız öneri";
  $("calls").textContent = s.callsToday;
  $("sent").textContent = s.sentHour;
  $("err").textContent = s.lastError || "";
  const t = $("toggle");
  t.textContent = s.paused ? "Sürdür" : "Durdur";
  t.className = s.paused ? "" : "stop";
  t.dataset.paused = s.paused ? "1" : "";
}

$("toggle").onclick = async () => {
  const pausing = !$("toggle").dataset.paused;
  await chrome.storage.local.set({ enabled: !pausing }); // içerik betiği çalışan gönderimi de keser
  await ask({ t: pausing ? "pause" : "resume" });
  setTimeout(refresh, 200);
};
$("openpanel").onclick = async () => {
  const w = await chrome.windows.getCurrent();
  await chrome.sidePanel.open({ windowId: w.id });
  window.close();
};
$("opentab").onclick = async () => {
  const c = await chrome.storage.local.get(["baseUrl", "key"]);
  const base = (c.baseUrl || DEFAULTS.baseUrl).replace(/\/+$/, "");
  await chrome.tabs.create({ url: `${base}/#token=${encodeURIComponent(c.key || DEFAULTS.key)}` });
};
$("showpanel").onclick = () => chrome.storage.local.set({ panelHidden: false });
$("reconnect").onclick = async () => { await ask({ t: "reconnect" }); setTimeout(refresh, 400); };
$("save").onclick = async () => {
  await chrome.storage.local.set({ baseUrl: $("baseUrl").value.trim() || DEFAULTS.baseUrl, key: $("key").value.trim() });
  await ask({ t: "reconnect" });
  setTimeout(refresh, 400);
};
(async () => {
  const s = await chrome.storage.local.get(["baseUrl", "key"]);
  $("baseUrl").value = s.baseUrl || "";
  $("key").value = s.key || "";
  refresh();
})();
