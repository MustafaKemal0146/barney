// Sahte API: `?mock=1` ile arka uç olmadan arayüz geliştirmek için. Bellekte durur.
import { ApiError } from './api'
import type {
  AppState, Cand, CardSummary, ChatMode, ChatMsg, ChatRow, Fact, HistoryItem, ImportJob, ImportOpts, Insights,
  LiveChat, LiveInfo, Person, PersonPatch, ServerEvent, SuggestResult,
} from './types'

const now = () => Math.floor(Date.now() / 1000)
const DAY = 86400
const delay = (ms = 250) => new Promise((r) => setTimeout(r, ms))

const defGuards = { first_n_approvals: 3, quiet_start: '01:00', quiet_end: '08:00', quiet_on: true, max_per_hour: 6, max_per_day: 30, stop_sensitive: true }
const defTone = { preset: 'benim' as const, flort: 3, cesaret: 4, mizah: 5, samimiyet: 6 }

function card(over: Partial<CardSummary>): CardSummary {
  return {
    messages: 2000, burst: { single: 46, two_three: 38, four_plus: 16, median: 2, max: 9 },
    words_median: 5, words_p90: 14, lower_start_pct: 72, no_caps_pct: 64, punct_none_pct: 58,
    emoji_pct: 18, top_emoji: ['😂', '🥹', '🙈'], latency_median_sec: 140, signature: ['ya', 'hee', 'aynen'], ...over,
  }
}

function mkPerson(p: Partial<Person> & { id: string; name: string }): Person {
  return {
    owner: 'Ara', messages_total: 4200, messages_owner: 2100, messages_partner: 2100, confidence: 'yüksek',
    last_message_ts: now() - 3600, goal: '', notes: '', stage: '', tone: { ...defTone }, watch: true,
    auto_send: false, full_auto: false, council: false, guards: { ...defGuards }, silence_days: 5, approvals_done: 1,
    card: { owner: card({}), partner: card({ lower_start_pct: 40, no_caps_pct: 30, emoji_pct: 34, top_emoji: ['😊', '❤️', '🥰'], latency_median_sec: 320, signature: ['canım', 'ayy', 'valla'], words_median: 7, words_p90: 19 }) },
    ...p,
  }
}

interface DB {
  state: AppState
  people: Person[]
  chats: Record<string, ChatMsg[]>
  facts: Record<string, Fact[]>
  history: Record<string, HistoryItem[]>
  live: LiveInfo
  seq: number
  job: ImportJob | null
}

const db: DB = {
  state: {
    connected: true, key_set: true, provider: 'deepseek', providers: ['deepseek', 'openai', 'anthropic', 'gemini'],
    model: 'deepseek-chat', has_people: true, paused: false,
    stats: { calls_today: 14, sent_last_hour: 2, auto_send: false, paused: false },
    ext_dir: '~/.config/barney/extension', version: '0.1.0-mock', notifications: false, learn_feedback: true, lan: false,
    history: { max_messages: 3000, max_seconds: 300, all: false }, auto_learn: false, bulk_speed: 'yavas',
  },
  people: [
    mkPerson({ id: 'p1', name: 'Zeynep', goal: 'flort', stage: 'yakinlasma', last_message_ts: now() - 2 * 3600, notes: 'Mimarlık okuyor. Kahve sever, sabahları geç cevap verir.', tone: { preset: 'barney', flort: 6, cesaret: 6, mizah: 7, samimiyet: 6 } }),
    mkPerson({ id: 'p2', name: 'Kerem', goal: 'arkadaslik', stage: 'yakin', messages_total: 1800, messages_owner: 950, messages_partner: 850, last_message_ts: now() - 9 * DAY, silence_days: 7, tone: { ...defTone, preset: 'samimi', samimiyet: 9 } }),
    mkPerson({ id: 'p3', name: 'Elif', goal: 'is', messages_total: 180, messages_owner: 90, messages_partner: 90, confidence: 'düşük', last_message_ts: now() - 26 * 3600, silence_days: 14, tone: { ...defTone, preset: 'sade' } }),
  ],
  chats: {
    'p1:advisor': [
      { id: 'a1', role: 'user', text: 'Bu akşam görüşmeyi nasıl teklif edeyim?', ts: now() - 5000 },
      { id: 'a2', role: 'assistant', text: 'Doğrudan ama hafif git: "Bu hafta o kahveciye gitmemiz lazım, ben seni sürüklüyorum" tarzı. Cuma akşamı için iki seçenek ver, birini seçmesi kolay olsun.', ts: now() - 4990 },
    ],
    'p1:roleplay': [
      { id: 'r1', role: 'user', text: 'nasılsın bugün', ts: now() - 4000 },
      { id: 'r2', role: 'assistant', text: 'iyiyim ya sen', ts: now() - 3990, steps: [{ text: 'iyiyim ya', wait_sec: 8 }, { text: 'sen nasılsın bugün 🙈', wait_sec: 12 }] },
    ],
  },
  facts: {
    p1: [
      { id: 'f1', text: 'Mimarlık 4. sınıf, final haftası yaklaşıyor.' },
      { id: 'f2', text: 'Köpeği Pamuk; fotoğraflarını sık paylaşıyor.' },
      { id: 'f3', text: 'Geçen hafta Cumartesi kahve içmeyi konuştular, tarih kesinleşmedi.' },
    ],
    p2: [{ id: 'f4', text: 'Kerem Ankara\'ya taşındı, hafta sonları geliyor.' }],
    p3: [],
  },
  history: {
    p1: [
      { ts: now() - 3 * 3600, kind: 'suggested', text: 'ya o zaman cumartesi kesin, ben ayarlarım' },
      { ts: now() - 3 * 3600 + 40, kind: 'edited', text: 'ya o zaman cumartesi kesin, ben ayarlarım', final: 'o zaman cumartesi kesin, ben ayarlarım 😄' },
      { ts: now() - 3 * 3600 + 60, kind: 'sent', text: 'o zaman cumartesi kesin, ben ayarlarım 😄' },
      { ts: now() - DAY, kind: 'rejected', text: 'bugün çok yoruldum ya, sen?' },
      { ts: now() - DAY - 600, kind: 'approved', text: 'hee ben de geliyorum o zaman' },
    ],
  },
  live: {
    connected: true, platform: 'whatsapp',
    chat: {
      partner: 'Zeynep', incoming_key: 'k-100', person_id: 'p1',
      history: [
        { from: 'them', text: 'yarın final var ya canım sıkkın', ts: now() - 900 },
        { from: 'me', text: 'ayy hangi ders', ts: now() - 840 },
        { from: 'me', text: 'sen halledersin zaten', ts: now() - 830 },
        { from: 'them', text: 'statik 😭 hiç çalışamadım', ts: now() - 700 },
        { from: 'them', text: 'bi kahve içsek keşke', ts: now() - 120 },
      ],
    },
  },
  seq: 100,
  job: null,
}

// ?mock=1&fresh=1 → ilk kurulum: kişi yok, anahtar yok, uzantı bağlı değil.
if (typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('fresh') === '1') {
  db.people = []
  db.live = { connected: false, platform: '' }
  Object.assign(db.state, { connected: false, key_set: false, has_people: false })
}
// &ext=1 → uzantı bağlı, WhatsApp'ta kayıtlı olmayan bir sohbet ("Deniz") açık (kurulum akışını denemek için).
if (typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('ext') === '1') {
  db.state.connected = true
  db.live = {
    connected: true, platform: 'whatsapp',
    chat: { partner: 'Deniz', incoming_key: 'k-1', history: [{ from: 'them', text: 'selam, nasılsın', ts: now() - 300 }] },
  }
}
// &fail=1 → "WhatsApp'tan oku" işi başarısız olur ve can_fallback_ai döner (AI'a geçiş düğmesini denemek için).
const FAIL = typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('fail') === '1'

const listeners = new Set<(e: ServerEvent) => void>()
function emit(e: ServerEvent) {
  listeners.forEach((l) => l(e))
}
const setStats = () => {
  db.state.stats = { ...db.state.stats, auto_send: db.people.some((p) => p.auto_send), paused: db.state.paused }
}
const stateEvent = () => emit({ type: 'state', state: structuredClone(db.state) })

function cands(name: string, seed = 0): SuggestResult {
  const sets: Cand[][] = [
    [
      { tur: 'guvenli', steps: [{ text: 'ayy geçmiş olsun, halledersin sen', wait_sec: 6 }], confident: true, blocked: false, sensitive: false, violations: [] },
      { tur: 'komik', steps: [{ text: 'statik mi 😭 binalar yıkılmasın diye çalışıyoruz ciddi bir iş', wait_sec: 9 }, { text: 'kahve benden', wait_sec: 14 }], confident: true, blocked: false, sensitive: false, violations: [] },
      { tur: 'cesur', steps: [{ text: 'final bitince kahveyi ben ısmarlıyorum, kaçış yok', wait_sec: 12 }], confident: false, blocked: false, sensitive: false, violations: ['Büyük harfle başladı'] },
      { tur: 'tek', steps: [{ text: 'çalışırken yanında olabilirim istersen', wait_sec: 7 }], confident: false, blocked: true, sensitive: true, violations: ['Hassas konu: yakın zamanda yalnızlık geçti', 'Noktalama kalıbı uyuşmuyor'] },
    ],
    [
      { tur: 'tek', steps: [{ text: 'hee ben de aynısını düşünüyordum', wait_sec: 5 }], confident: true, blocked: false, sensitive: false, violations: [] },
      { tur: 'komik', steps: [{ text: 'telepati bu 🙈', wait_sec: 8 }], confident: true, blocked: false, sensitive: false, violations: [] },
    ],
  ]
  const c = structuredClone(sets[seed % sets.length])
  return { cands: c, recommended: 0, note: `${name} ile son konuşma sakin; öneriler sahibin kısa ve küçük harfli stiline göre.` }
}

function advisorReply(text: string): string {
  if (/teklif|görüş|buluş/i.test(text)) return 'Net ama baskısız git: iki gün/saat seçeneği ver, "olur mu" yerine "hangisi" diye sor. Örnek: "Cuma mı cumartesi mi, ben seni kahveye sürükleyeceğim."'
  return 'Son konuşmaya göre sıcak ama acele etmeyen bir ton iyi olur. İstersen "Cevap öner" ile hazır adaylar çıkarabilirim.'
}
function roleplayReply(text: string): ChatMsg['steps'] {
  const t = text.toLowerCase()
  if (t.includes('kahve')) return [{ text: 'olurr', wait_sec: 7 }, { text: 'ama final bitsin önce 😭', wait_sec: 11 }, { text: 'sonra ben ısmarlarım', wait_sec: 15 }]
  return [{ text: 'hmm', wait_sec: 6 }, { text: 'nasıl yani anlat bakayım', wait_sec: 10 }]
}

function insightsFor(p: Person): Insights {
  const byHour = Array.from({ length: 24 }, (_, h) => Math.round(30 + 160 * Math.exp(-((h - 22) ** 2) / 18) + 70 * Math.exp(-((h - 13) ** 2) / 10)))
  const byWeekday = [310, 280, 295, 340, 410, 520, 450]
  const weekly = Array.from({ length: 12 }, (_, i) => ({ week: `2026-W${33 + i}`, owner: 60 + ((i * 37) % 80), partner: 50 + ((i * 53) % 90) }))
  return {
    total: p.messages_total, owner_share_pct: Math.round((p.messages_owner / p.messages_total) * 100),
    partner_share_pct: Math.round((p.messages_partner / p.messages_total) * 100), initiator_owner_pct: 41,
    latency_owner_median_sec: 140, latency_partner_median_sec: 320, by_hour: byHour, by_weekday: byWeekday, weekly,
    last_ts: p.last_message_ts, silence_days: Math.floor((now() - p.last_message_ts) / DAY),
  }
}

const find = (id: string) => {
  const p = db.people.find((x) => x.id === id)
  if (!p) throw new ApiError(404, 'Kişi bulunamadı')
  return p
}

const CHAT_LIST: ChatRow[] = [
  { name: 'Zeynep', last: 'bi kahve içsek keşke', unread: 1 },
  { name: 'Kerem', last: 'abi hafta sonu geliyorum', unread: 0 },
  { name: 'Elif', last: 'toplantı notlarını attım', unread: 0 },
  { name: 'Deniz', last: 'selam, nasılsın', unread: 2 },
  { name: 'Merve', last: 'tamam görüşürüz 🙂', unread: 0 },
  { name: 'Can', last: 'maçı izledin mi', unread: 0 },
  { name: 'Aylin', last: 'fotoğraflar çok güzel olmuş', unread: 3 },
  { name: 'Burak', last: 'yarın uğrarım', unread: 0 },
]

let jobTimer: ReturnType<typeof setInterval> | null = null
const stopJob = () => {
  if (jobTimer) clearInterval(jobTimer)
  jobTimer = null
}
const emitJob = () => {
  if (db.job) emit({ type: 'import', job: structuredClone(db.job) })
}
const active = (j: ImportJob | null) => !!j && (j.state === 'queued' || j.state === 'running')
const requireExt = () => {
  if (!db.state.connected || !db.live.connected) throw new ApiError(409, 'WhatsApp sekmesi bağlı değil')
}
const requireFree = () => {
  if (active(db.job)) throw new ApiError(409, 'başka bir iş çalışıyor')
}

/** Kişiyi ada göre bulur ya da oluşturur; okunan mesaj sayısına göre güven düşer. */
function upsertImported(name: string, loaded: number): Person {
  const ex = db.people.find((p) => p.name.toLowerCase() === name.toLowerCase())
  if (ex) {
    ex.messages_total += loaded
    ex.messages_partner += Math.floor(loaded / 2)
    ex.messages_owner += Math.ceil(loaded / 2)
    ex.last_message_ts = now()
    return ex
  }
  const p = mkPerson({
    id: 'p' + ++db.seq, name, messages_total: loaded, messages_owner: Math.ceil(loaded / 2), messages_partner: Math.floor(loaded / 2),
    confidence: loaded < 300 ? 'düşük' : loaded < 2500 ? 'orta' : 'yüksek', last_message_ts: now() - 600,
  })
  db.people.push(p)
  db.state.has_people = true
  if (db.live.chat && db.live.chat.partner === name) {
    db.live.chat.person_id = p.id
    emit({ type: 'live', live: structuredClone(db.live.chat), person_id: p.id })
  }
  return p
}

/** Sahte iş: ~6 sn'de ilerler (toplu: sohbet başına ~2 sn), günlük satırları üretir. */
function runJob(kind: ImportJob['kind'], names: string[], o: ImportOpts, syncId?: string): ImportJob {
  const target = o.all ? 8200 : Math.min(o.max_messages, 2400)
  const job: ImportJob = {
    id: 'j' + ++db.seq, kind, state: 'queued', mode: o.mode, loaded: 0, chat: names[0] ?? '', i: kind === 'bulk' ? 1 : 0,
    n: kind === 'bulk' ? names.length : 1, person_ids: [], note: '', log: [], started_at: now(),
  }
  db.job = job
  const perChat = kind === 'bulk' ? 4 : 12 // 500 ms'lik adım sayısı
  let tick = 0
  const say = (t: string) => {
    job.log.push(t)
    if (job.log.length > 40) job.log.shift()
  }
  const failNow = kind === 'current' && o.mode === 'reader' && FAIL
  setTimeout(() => {
    if (db.job !== job || job.state !== 'queued') return
    job.state = 'running'
    say(o.mode === 'ai' ? 'AI WhatsApp sekmesini inceliyor…' : `Sohbet açılıyor: ${job.chat || 'açık sohbet'}`)
    emitJob()
    jobTimer = setInterval(() => {
      if (db.job !== job || job.state !== 'running') return stopJob()
      tick++
      const total = perChat * (kind === 'bulk' ? names.length : 1)
      if (failNow && tick === 4) {
        job.state = 'failed'
        job.error = 'Sohbet geçmişi okunamadı: kaydırma alanı bulunamadı'
        job.can_fallback_ai = true
        say('Hata: kaydırma alanı bulunamadı')
        stopJob()
        return emitJob()
      }
      const idx = Math.min((kind === 'bulk' ? names.length : 1) - 1, Math.floor((tick - 1) / perChat))
      const within = tick - idx * perChat
      job.loaded = Math.round((target * within) / perChat)
      if (kind === 'bulk') {
        job.i = idx + 1
        job.chat = names[idx]
        if (within === 1 && idx > 0) say(`Sonraki sohbet: ${job.chat} (5–15 sn bekleme)`)
      }
      say(o.mode === 'ai' ? (['Sohbet listesine bakıyor', 'Kaydırıyor', 'Mesajları okuyor'][tick % 3]) : `Kaydırıldı: ${job.loaded.toLocaleString('tr-TR')} mesaj`)
      if (tick >= total) {
        stopJob()
        if (kind === 'sync' && syncId) {
          const p = find(syncId)
          p.messages_total += 38
          p.last_message_ts = now()
          job.person_ids = [syncId]
          job.loaded = 38
        } else {
          job.person_ids = names.map((n, k) => upsertImported(n, Math.max(1, Math.round(target * (k % 2 ? 0.6 : 1)))).id)
        }
        job.state = 'done'
        say('Bitti')
        setStats()
        stateEvent()
      }
      emitJob()
    }, 500)
  }, 300)
  return structuredClone(job)
}

/** Gerçek api.ts `request()` ile aynı imza: yol + gövde → JSON. */
export async function mockCall(method: string, rawPath: string, body?: unknown): Promise<unknown> {
  await delay()
  const url = new URL(rawPath, 'http://x')
  const path = url.pathname
  const mode = (url.searchParams.get('mode') ?? 'advisor') as ChatMode
  const b = (body ?? {}) as Record<string, unknown>
  let m: RegExpMatchArray | null

  if (path === '/api/state') return structuredClone(db.state)
  if (path === '/api/settings') {
    const s = db.state
    if (typeof b.provider === 'string') s.provider = b.provider
    if (typeof b.model === 'string') s.model = b.model
    if (typeof b.key === 'string') s.key_set = b.key !== ''
    if (typeof b.notifications === 'boolean') s.notifications = b.notifications
    if (typeof b.learn_feedback === 'boolean') s.learn_feedback = b.learn_feedback
    if (typeof b.history_max_messages === 'number') s.history.max_messages = b.history_max_messages
    if (typeof b.history_max_seconds === 'number') s.history.max_seconds = b.history_max_seconds
    if (typeof b.history_all === 'boolean') s.history.all = b.history_all
    if (typeof b.auto_learn === 'boolean') s.auto_learn = b.auto_learn
    if (b.bulk_speed === 'yavas' || b.bulk_speed === 'hizli') s.bulk_speed = b.bulk_speed
    return structuredClone(s)
  }
  if (path === '/api/pause' || path === '/api/resume') {
    db.state.paused = path === '/api/pause'
    setStats()
    return structuredClone(db.state)
  }
  if (path === '/api/people') return structuredClone(db.people)
  if (path === '/api/live') return structuredClone(db.live)
  if (path === '/api/import/current' && method === 'POST') {
    const o = b as unknown as ImportOpts
    requireFree()
    requireExt()
    if (o.mode !== 'ai' && !db.live.chat) throw new ApiError(409, 'Açık bir WhatsApp sohbeti yok')
    return runJob('current', [db.live.chat?.partner ?? 'Yeni Kişi'], o)
  }
  if (path === '/api/import/chats/list' && method === 'POST') {
    requireExt()
    setTimeout(() => emit({ type: 'chats', chats: structuredClone(CHAT_LIST) }), 1200)
    return undefined
  }
  if (path === '/api/import/bulk' && method === 'POST') {
    const names = Array.isArray(b.names) ? (b.names as string[]) : []
    if (names.length === 0) throw new ApiError(400, 'En az bir sohbet seç')
    if (names.length > 10) throw new ApiError(400, 'En fazla 10 sohbet seçilebilir')
    requireFree()
    requireExt()
    return runJob('bulk', names, b as unknown as ImportOpts)
  }
  if (path === '/api/import/active') return db.job && active(db.job) ? structuredClone(db.job) : null
  if ((m = path.match(/^\/api\/import\/([^/]+)(\/cancel)?$/))) {
    const j = db.job
    if (!j || j.id !== decodeURIComponent(m[1])) throw new ApiError(404, 'İş bulunamadı')
    if (m[2]) {
      if (active(j)) {
        stopJob()
        j.state = 'canceled'
        j.log.push('Kullanıcı durdurdu')
        emitJob()
      }
    }
    return structuredClone(j)
  }
  if (path === '/api/live/resync') {
    setTimeout(() => listeners.forEach((l) => l({ type: 'resynced', partner: db.live.chat?.partner ?? '', added: 0, known: true, seen: db.live.chat?.history.length ?? 0 })), 300)
    return { ok: true }
  }
  if (path === '/api/live/suggest') {
    const name = db.live.chat?.partner ?? 'Kişi'
    return cands(name, db.seq++)
  }
  if (path === '/api/live/send' || path === '/api/live/approve') {
    const steps = (b.steps as { text: string }[]) ?? []
    const text = steps.map((s) => s.text).join(' / ')
    setTimeout(() => {
      if (db.live.chat) db.live.chat.history.push(...steps.map((s) => ({ from: 'me' as const, text: s.text, ts: now() })))
      db.state.stats.sent_last_hour++
      emit({ type: 'sent', chat_id: String(b.chat_id ?? ''), text, auto: path.endsWith('approve') })
      stateEvent()
      if (db.live.chat) emit({ type: 'live', live: structuredClone(db.live.chat), person_id: db.live.chat.person_id })
    }, 500)
    return undefined
  }

  if ((m = path.match(/^\/api\/people\/([^/]+)(?:\/(.*))?$/))) {
    const id = decodeURIComponent(m[1])
    const sub = m[2] ?? ''
    const p = find(id)
    if (sub === '') {
      if (method === 'GET') return structuredClone(p)
      if (method === 'DELETE') {
        db.people = db.people.filter((x) => x.id !== id)
        db.state.has_people = db.people.length > 0
        setStats()
        stateEvent()
        return undefined
      }
      if (method === 'PATCH') {
        Object.assign(p, b as PersonPatch)
        setStats()
        stateEvent()
        return structuredClone(p)
      }
    }
    if (sub === 'open' && method === 'POST') {
      requireExt()
      const chat: LiveChat = { partner: p.name, incoming_key: 'k-' + ++db.seq, person_id: p.id, history: [{ from: 'them', text: 'selam', ts: now() - 200 }] }
      setTimeout(() => {
        db.live = { connected: true, platform: 'whatsapp', chat }
        emit({ type: 'live', live: structuredClone(chat), person_id: p.id })
      }, 400)
      return undefined
    }
    if (sub === 'sync' && method === 'POST') {
      requireFree()
      requireExt()
      return runJob('sync', [p.name], b as unknown as ImportOpts, id)
    }
    if (sub === 'chat') {
      const key = `${id}:${mode}`
      if (method === 'GET') return { messages: structuredClone(db.chats[key] ?? []) }
      if (method === 'DELETE') {
        delete db.chats[key]
        return undefined
      }
      const text = String(b.text ?? '')
      const md = (b.mode as ChatMode) ?? 'advisor'
      const k2 = `${id}:${md}`
      const u: ChatMsg = { id: 'm' + ++db.seq, role: 'user', text, ts: now() }
      const a: ChatMsg =
        md === 'roleplay'
          ? { id: 'm' + ++db.seq, role: 'assistant', text: '', ts: now(), steps: roleplayReply(text) }
          : { id: 'm' + ++db.seq, role: 'assistant', text: advisorReply(text), ts: now() }
      if (md === 'roleplay') a.text = a.steps!.map((s) => s.text).join('\n')
      db.chats[k2] = [...(db.chats[k2] ?? []), u, a]
      db.state.stats.calls_today++
      return { messages: [u, a] }
    }
    if (sub === 'suggest') {
      db.state.stats.calls_today++
      return cands(p.name, db.seq++)
    }
    if (sub === 'nudge') {
      const r = cands(p.name, 1)
      return { cands: r.cands, note: `${p.name} ile sessizlik sürüyor; konu açan hafif mesajlar.` }
    }
    if (sub === 'memory') return { facts: structuredClone(db.facts[id] ?? []), stage: p.stage }
    if (sub === 'memory/refresh') {
      db.facts[id] = [...(db.facts[id] ?? []), { id: 'f' + ++db.seq, text: 'Yeni: hafta sonu planı için seçenek bekliyor.' }]
      return { facts: structuredClone(db.facts[id]), stage: p.stage }
    }
    if ((m = sub.match(/^memory\/(.+)$/)) && method === 'DELETE') {
      db.facts[id] = (db.facts[id] ?? []).filter((f) => f.id !== m![1])
      return undefined
    }
    if (sub === 'insights') return insightsFor(p)
    if (sub === 'history') return structuredClone(db.history[id] ?? [])
  }
  throw new ApiError(404, `Sahte API: ${method} ${path} tanımlı değil`)
}

/** Zamanlayıcıyla SSE benzeri olaylar üretir; `signal` iptal olunca durur. */
export function mockStream(onEvent: (e: ServerEvent) => void, signal: AbortSignal): Promise<void> {
  return new Promise<void>((resolve) => {
    listeners.add(onEvent)
    onEvent({ type: 'state', state: structuredClone(db.state) })
    const incoming = ['bu arada yarın akşam boşum', 'yaa çok sıkıldım', 'bi şey soracağım', 'sen ne yapıyorsun']
    let i = 0
    const timer = setInterval(() => {
      const chat = db.live.chat as LiveChat | undefined
      if (!chat || db.state.paused) return
      const text = incoming[i++ % incoming.length]
      const key = 'k-' + ++db.seq
      chat.history.push({ from: 'them', text, ts: now() })
      chat.incoming_key = key
      onEvent({ type: 'live', live: structuredClone(chat), person_id: chat.person_id })
      setTimeout(() => {
        const p = db.people.find((x) => x.id === chat.person_id)
        const r = cands(chat.partner, i)
        db.state.stats.calls_today++
        onEvent({ type: 'suggestions', chat_id: chat.partner, incoming_key: key, person_id: chat.person_id, pending_approval: !!p?.auto_send, ...r })
        stateEvent()
      }, 1500)
      if (i % 3 === 0) onEvent({ type: 'notify', title: 'Barney', body: `${chat.partner}: ${text}` })
    }, 25000)
    const stop = () => {
      clearInterval(timer)
      listeners.delete(onEvent)
      resolve()
    }
    if (signal.aborted) stop()
    else signal.addEventListener('abort', stop, { once: true })
  })
}

// Elle deneme için: konsoldan `__barneyMock.setLive('Deniz')` ile açık WhatsApp sohbetini değiştir.
if (typeof window !== 'undefined') {
  ;(window as unknown as { __barneyMock: unknown }).__barneyMock = {
    setLive(partner: string | null) {
      if (!partner) {
        db.live = { connected: false, platform: '' }
        return
      }
      const p = db.people.find((x) => x.name === partner)
      const chat: LiveChat = { partner, incoming_key: 'k-' + ++db.seq, person_id: p?.id, history: [{ from: 'them', text: 'selam', ts: now() - 60 }] }
      db.live = { connected: true, platform: 'whatsapp', chat }
      emit({ type: 'live', live: structuredClone(chat), person_id: p?.id })
    },
  }
}
