// Yan panel: Barney arayüzünü (embed modu) iframe içinde gösterir. Token adres çubuğunda değil, URL
// hash'inde taşınır; sunucu yalnızca yerel adrestir.
import { DEFAULTS } from "./config.js";
const $ = (id) => document.getElementById(id);
(async () => {
  const s = await chrome.storage.local.get(["baseUrl", "key"]);
  const base = (s.baseUrl || DEFAULTS.baseUrl).replace(/\/+$/, "");
  const key = s.key || DEFAULTS.key;
  if (!key) { $("msg").textContent = "Eşleşme anahtarı yok: `barney extension install` çalıştır."; return; }
  const f = $("app");
  f.src = `${base}/?embed=1#token=${encodeURIComponent(key)}`;
  f.onload = () => { $("msg").hidden = true; f.hidden = false; };
  setTimeout(() => { if (f.hidden) $("msg").textContent = "Sunucuya ulaşılamıyor: `make dev` (ya da `barney serve`) çalışıyor mu?"; }, 4000);
})();
