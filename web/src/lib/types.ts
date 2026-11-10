// docs/API.md ile birebir tipler.
export type TonePreset = 'benim' | 'barney' | 'sade' | 'samimi' | 'flort'
export interface Tone {
  preset: TonePreset
  flort: number
  cesaret: number
  mizah: number
  samimiyet: number
}
export interface Guards {
  first_n_approvals: number
  quiet_start: string
  quiet_end: string
  quiet_on: boolean
  max_per_hour: number
  max_per_day: number
  stop_sensitive: boolean
}
export type Goal = 'flort' | 'arkadaslik' | 'is' | 'aile' | 'barisma' | ''
export type Confidence = 'düşük' | 'orta' | 'yüksek'

export interface CardSummary {
  messages: number
  burst: { single: number; two_three: number; four_plus: number; median: number; max: number }
  words_median: number
  words_p90: number
  lower_start_pct: number
  no_caps_pct: number
  punct_none_pct: number
  emoji_pct: number
  top_emoji: string[]
  latency_median_sec: number
  signature: string[]
}
export interface Person {
  id: string
  name: string
  owner: string
  messages_total: number
  messages_owner: number
  messages_partner: number
  confidence: Confidence
  last_message_ts: number
  goal: Goal
  notes: string
  stage: string
  tone: Tone
  watch: boolean
  auto_send: boolean
  full_auto: boolean
  council: boolean
  guards: Guards
  silence_days: number
  approvals_done: number
  card: { owner: CardSummary; partner: CardSummary }
}
export interface Step {
  text: string
  wait_sec: number
}
export type CandTur = 'tek' | 'guvenli' | 'komik' | 'cesur'
export interface Cand {
  tur: CandTur
  steps: Step[]
  confident: boolean
  blocked: boolean
  sensitive: boolean
  violations: string[]
}
export type ChatMode = 'advisor' | 'roleplay'
export interface ChatMsg {
  id: string
  role: 'user' | 'assistant'
  text: string
  ts: number
  steps?: Step[]
}
export interface LiveMsg {
  from: 'me' | 'them'
  text: string
  ts: number
  kind?: string
}
export interface LiveChat {
  partner: string
  history: LiveMsg[]
  incoming_key: string
  person_id?: string
}
export interface Stats {
  calls_today: number
  sent_last_hour: number
  auto_send: boolean
  paused: boolean
}
export interface AppState {
  connected: boolean
  key_set: boolean
  provider: string
  providers: string[]
  model: string
  has_people: boolean
  paused: boolean
  stats: Stats
  ext_dir: string
  version: string
  notifications: boolean
  learn_feedback: boolean
  lan: boolean
  history: HistoryPrefs
  auto_learn: boolean
  bulk_speed: BulkSpeed
}
export interface SuggestResult {
  cands: Cand[]
  recommended: number
  note: string
}
export interface Fact {
  id: string
  text: string
}
export interface Memory {
  facts: Fact[]
  stage: string
}
export interface Insights {
  total: number
  owner_share_pct: number
  partner_share_pct: number
  initiator_owner_pct: number
  latency_owner_median_sec: number
  latency_partner_median_sec: number
  by_hour: number[]
  by_weekday: number[]
  weekly: { week: string; owner: number; partner: number }[]
  last_ts: number
  silence_days: number
}
export type HistoryKind = 'suggested' | 'sent' | 'edited' | 'rejected' | 'approved'
export interface HistoryItem {
  ts: number
  kind: HistoryKind
  text: string
  final?: string
}
export interface LiveInfo {
  connected: boolean
  platform: 'whatsapp' | 'instagram' | ''
  chat?: LiveChat
}
export type BulkSpeed = 'yavas' | 'hizli'
export interface HistoryPrefs {
  max_messages: number
  max_seconds: number
  all: boolean
}
export type ImportMode = 'reader' | 'ai'
export interface ImportOpts extends HistoryPrefs {
  mode: ImportMode
}
export type ImportState = 'queued' | 'running' | 'done' | 'failed' | 'canceled'
export interface ImportJob {
  id: string
  kind: 'current' | 'bulk' | 'sync'
  state: ImportState
  mode: ImportMode
  loaded: number
  chat: string
  i: number
  n: number
  person_ids: string[]
  note: string
  log: string[]
  started_at: number
  error?: string
  can_fallback_ai?: boolean
}
export interface ChatRow {
  name: string
  last: string
  unread: number
}

export interface SettingsPatch {
  provider?: string
  key?: string
  model?: string
  notifications?: boolean
  learn_feedback?: boolean
  history_max_messages?: number
  history_max_seconds?: number
  history_all?: boolean
  auto_learn?: boolean
  bulk_speed?: BulkSpeed
}
export interface PersonPatch {
  goal?: Goal
  notes?: string
  stage?: string
  tone?: Tone
  watch?: boolean
  auto_send?: boolean
  full_auto?: boolean
  council?: boolean
  guards?: Guards
  silence_days?: number
}
export interface SendBody {
  chat_id: string
  incoming_key: string
  steps: Step[]
  cand_tur?: string
  /** ÖNERİLEN GENİŞLETME (API.md'de yok): true ise yalnız kutuya yazar, göndermez. */
  draft?: boolean
}

export type ServerEvent =
  | { type: 'state'; state: AppState }
  | { type: 'live'; live: LiveChat; person_id?: string }
  | ({ type: 'suggestions'; chat_id: string; incoming_key: string; person_id?: string; pending_approval?: boolean } & SuggestResult)
  | { type: 'sent'; chat_id: string; text: string; auto: boolean }
  | { type: 'aborted'; chat_id: string; reason: string }
  | { type: 'notify'; title: string; body: string }
  | { type: 'resynced'; partner: string; added: number; known: boolean; seen: number }
  | { type: 'import'; job: ImportJob }
  | { type: 'chats'; chats: ChatRow[] }
