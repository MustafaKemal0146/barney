# Barney yerel API (web arayüzü ↔ Go sunucu)

Yalnız 127.0.0.1 (ileride LAN modu). Kimlik: her istekte `Authorization: Bearer <token>`.
Arayüz token'ı `/#token=<token>` hash'inden alır, sessionStorage'a koyar, adres çubuğundan siler.
Hatalar: HTTP 4xx/5xx, gövde düz metin Türkçe mesaj. Tüm gövdeler JSON (yükleme hariç).
Zaman damgaları unix saniye. `wait_sec` bir mesajdan önce beklenecek saniye.

## Tipler
```
Tone    {preset:"benim"|"barney"|"sade"|"samimi"|"flort", flort,cesaret,mizah,samimiyet: 0..10}
Guards  {first_n_approvals:int(3), quiet_start:"01:00", quiet_end:"08:00", quiet_on:bool,
         max_per_hour:int(6), max_per_day:int(30), stop_sensitive:bool(true)}
Person  {id, name, owner, messages_total, messages_owner, messages_partner,
         confidence:"düşük"|"orta"|"yüksek", last_message_ts, goal:"flort"|"arkadaslik"|"is"|"aile"|"barisma"|"",
         notes, stage, tone:Tone, watch:bool, auto_send:bool, council:bool, guards:Guards,
         silence_days:int, approvals_done:int,
         card:{owner:CardSummary, partner:CardSummary}}
CardSummary {messages, burst:{single,two_three,four_plus,median,max}, words_median, words_p90,
         lower_start_pct, no_caps_pct, punct_none_pct, emoji_pct, top_emoji:[string],
         latency_median_sec, signature:[string]}
Step    {text, wait_sec}
Cand    {tur:"tek"|"guvenli"|"komik"|"cesur", steps:[Step], confident:bool, blocked:bool, sensitive:bool, violations:[string]}
ChatMsg {id, role:"user"|"assistant", text, ts, steps?:[Step]}   // rol provasında assistant.steps dolu
LiveChat{partner, history:[{from:"me"|"them", text, ts, kind?}], incoming_key, person_id?:string}
State   {connected:bool, key_set:bool, provider, providers:[string], model, has_people:bool,
         paused:bool, stats:{calls_today,sent_last_hour,auto_send,paused}, ext_dir, version,
         notifications:bool, learn_feedback:bool, lan:bool}
```

## Uçlar
- `GET  /api/state` → State
- `POST /api/settings` {provider?, key?, model?, notifications?, learn_feedback?} → State  (key "" = sil; key asla geri dönmez)
- `POST /api/pause` | `POST /api/resume` → State
- `GET  /api/people` → [Person]
- `POST /api/people` multipart (`file` zip/txt, opsiyonel `owner`) → Person | `{needs_owner:true, id, owner_candidates:[string]}` (202)
- `POST /api/people/{id}/owner` {owner} → Person
- `GET  /api/people/{id}` → Person
- `PATCH /api/people/{id}` {goal?, notes?, stage?, tone?, watch?, auto_send?, council?, guards?, silence_days?} → Person
- `DELETE /api/people/{id}` → 204
- `GET  /api/people/{id}/chat?mode=advisor|roleplay` → {messages:[ChatMsg]}
- `POST /api/people/{id}/chat` {mode, text} → {messages:[ChatMsg,ChatMsg]}  (kullanıcı + asistan)
- `DELETE /api/people/{id}/chat?mode=` → 204
- `POST /api/people/{id}/suggest` {goal?} → {cands:[Cand], recommended:int, note}
- `GET  /api/people/{id}/memory` → {facts:[{id,text}], stage}
- `DELETE /api/people/{id}/memory/{fid}` → 204 ; `POST /api/people/{id}/memory/refresh` → {facts, stage}
- `GET  /api/people/{id}/insights` → {total, owner_share_pct, partner_share_pct, initiator_owner_pct,
   latency_owner_median_sec, latency_partner_median_sec, by_hour:[24 sayı], by_weekday:[7 sayı], weekly:[{week:"2026-W44", owner:int, partner:int}], last_ts, silence_days}
- `GET  /api/people/{id}/history` → [{ts, kind:"suggested"|"sent"|"edited"|"rejected"|"approved", text, final?}]
- `POST /api/people/{id}/nudge` → {cands:[Cand], note}   (sessizlikte konu aç; model çağrısı yalnız tıklayınca)
- `GET  /api/live` → {connected:bool, platform:"whatsapp"|"instagram"|"", chat?:LiveChat}
- `POST /api/live/suggest` {} → {cands, recommended, note}       (açık sohbet için şimdi öner)
- `POST /api/live/send` {chat_id, incoming_key, steps:[Step], cand_tur?} → 202   (uzantı bayatlık/sohbet/taslak denetimlerini yapar)
- `POST /api/live/approve` {chat_id, incoming_key, steps, cand_tur} → 202       (oto modda onay bekleyen cevabı gönder)
- `GET  /api/events` SSE (`text/event-stream`), her olay `data: {json}`:
  `{type:"state", state:State}` · `{type:"live", live:LiveChat, person_id?}` ·
  `{type:"suggestions", chat_id, incoming_key, person_id?, cands, recommended, note, pending_approval?:bool}` ·
  `{type:"sent", chat_id, text, auto:bool}` · `{type:"aborted", chat_id, reason}` · `{type:"notify", title, body}`
  Kopunca tarayıcı yeniden bağlanır (EventSource değil; fetch+ReadableStream, Authorization başlığı için).

---
# Ek (plan 2): tarayıcıdan okuma, sohbet açma, toplu ekleme, AI modu

```
ImportOpts {max_messages:int(3000), max_seconds:int(300), all:bool, mode:"reader"|"ai"}
ImportJob  {id, kind:"current"|"bulk"|"sync", state:"queued"|"running"|"done"|"failed"|"canceled",
            mode:"reader"|"ai", loaded:int, chat:string, i:int, n:int, person_ids:[string],
            note:string, log:[string], started_at:int, error?:string, can_fallback_ai?:bool}
ChatRow    {name, last, unread:int}
State'e eklenenler: history:{max_messages,max_seconds,all}, auto_learn:bool, bulk_speed:"yavas"|"hizli"
POST /api/settings ayrıca: history_max_messages?, history_max_seconds?, history_all?, auto_learn?, bulk_speed?
```
- `POST /api/import/current` ImportOpts → 202 ImportJob (açık WhatsApp sohbetinin geçmişini uzantı okur; zip yok)
- `POST /api/import/chats/list` {} → 202; sonuç SSE `{type:"chats", chats:[ChatRow]}` (uzantı sol listeyi okur)
- `POST /api/import/bulk` {names:[string] (≤10), ...ImportOpts} → 202 ImportJob
- `GET  /api/import/active` → ImportJob | null ; `GET /api/import/{id}` → ImportJob ; `POST /api/import/{id}/cancel` → ImportJob
- `POST /api/people/{id}/sync` ImportOpts → 202 ImportJob (kişinin sohbetini WhatsApp'ta açıp yeniden okur, yeni mesajları birleştirir)
- `POST /api/people/{id}/open` → 202 (WhatsApp'ta sohbeti açar; yazmaz/göndermez)
- SSE `{type:"import", job:ImportJob}` (her ilerlemede), `{type:"chats", chats:[...]}`
- Hata gövdesi düz metin: "WhatsApp sekmesi bağlı değil", "başka bir iş çalışıyor" (409) vb.
- Zip yükleme (`POST /api/people`) arayüzde KULLANILMAZ (uç nokta dev/test için durur).

---
# Ek (plan 3): tam otonom, anlık senk, sınırsız sınır

```
Person'a eklenen: full_auto:bool   (açılınca auto_send ve watch da açılır; auto_send kapanınca kapanır)
Guards: max_per_hour / max_per_day = 0 → sınırsız
```
- `PATCH /api/people/{id}` ayrıca `full_auto?` kabul eder.
- `POST /api/live/resync` → 202 (uzantı yalnızca ekranda yüklü mesajları okur; kaydırma yok)
  Sonuç SSE `{type:"resynced", partner, added, known, seen}`
- `POST /api/import/{id}/cancel` okunan mesajları kaydederek durdurur.
- Uzantı → sunucu `live_update` karesi: açık sohbetin görünümünü tazeler (düzenlenen/silinen/giden mesaj);
  Gözcü ve model çağrılmaz.
