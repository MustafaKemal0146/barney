import { SSEParser } from './sse'
import type {
  AppState, ChatMode, ChatMsg, HistoryItem, ImportJob, ImportOpts, Insights, LiveInfo, Memory,
  Person, PersonPatch, SendBody, ServerEvent, SettingsPatch, SuggestResult,
} from './types'

const TOKEN_KEY = 'barney_token'

/** Hash'ten `token=...` parçasını ayırır; kalan hash (ör. `#/p/1/chat`) korunur. Saf fonksiyon (test edilir). */
export function extractToken(hash: string): { token: string | null; rest: string } {
  const raw = hash.replace(/^#/, '')
  if (!raw) return { token: null, rest: '' }
  let token: string | null = null
  const kept: string[] = []
  for (const part of raw.split('&')) {
    const m = /^token=([0-9a-zA-Z_.~-]+)$/.exec(part)
    if (m) token = m[1]
    else if (part) kept.push(part)
  }
  return { token, rest: kept.length ? '#' + kept.join('&') : '' }
}

/** `#token=...`ı sessionStorage'a alır ve adres çubuğundan yalnız token kısmını siler. */
export function initToken(): string | null {
  try {
    const { token, rest } = extractToken(window.location.hash)
    if (token) {
      sessionStorage.setItem(TOKEN_KEY, token)
      window.history.replaceState(null, '', window.location.pathname + window.location.search + rest)
    }
    return sessionStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

export function getToken(): string | null {
  try {
    return sessionStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

/** Sahte veri kipi: `?mock=1` ya da VITE_MOCK=1. */
export const MOCK: boolean =
  (typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('mock') === '1') ||
  import.meta.env.VITE_MOCK === '1'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

function authHeaders(): Record<string, string> {
  const h: Record<string, string> = {}
  const token = getToken()
  if (token) h['Authorization'] = 'Bearer ' + token
  return h
}

async function errorOf(res: Response): Promise<ApiError> {
  let msg = res.statusText
  try {
    const t = (await res.text()).trim()
    if (t) {
      try {
        const j = JSON.parse(t)
        msg = typeof j === 'string' ? j : (j.error ?? j.message ?? t)
      } catch {
        msg = t // gövde düz metin Türkçe mesaj
      }
    }
  } catch {
    /* gövde okunamadı */
  }
  if (res.status === 401 && !msg) msg = 'Yetkisiz'
  return new ApiError(res.status, msg || `HTTP ${res.status}`)
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  if (MOCK) {
    const { mockCall } = await import('./mock')
    return (await mockCall(method, path, body)) as T
  }
  const headers = authHeaders()
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) })
  if (!res.ok) throw await errorOf(res)
  if (res.status === 204 || res.status === 202) {
    const t = await res.text()
    return (t ? JSON.parse(t) : undefined) as T
  }
  return (await res.json()) as T
}

const enc = encodeURIComponent
const P = (id: string) => `/api/people/${enc(id)}`

export const api = {
  state: () => request<AppState>('GET', '/api/state'),
  settings: (b: SettingsPatch) => request<AppState>('POST', '/api/settings', b),
  pause: () => request<AppState>('POST', '/api/pause'),
  resume: () => request<AppState>('POST', '/api/resume'),

  people: () => request<Person[]>('GET', '/api/people'),
  person: (id: string) => request<Person>('GET', P(id)),
  patchPerson: (id: string, b: PersonPatch) => request<Person>('PATCH', P(id), b),
  deletePerson: (id: string) => request<void>('DELETE', P(id)),
  /** Kişinin sohbetini WhatsApp'ta açar (yazmaz/göndermez). */
  openPerson: (id: string) => request<void>('POST', `${P(id)}/open`, {}),
  /** Kişinin sohbetini yeniden okuyup yeni mesajları birleştirir. */
  syncPerson: (id: string, o: ImportOpts) => request<ImportJob>('POST', `${P(id)}/sync`, o),

  chat: (id: string, mode: ChatMode) => request<{ messages: ChatMsg[] }>('GET', `${P(id)}/chat?mode=${mode}`),
  sendChat: (id: string, mode: ChatMode, text: string) =>
    request<{ messages: ChatMsg[] }>('POST', `${P(id)}/chat`, { mode, text }),
  clearChat: (id: string, mode: ChatMode) => request<void>('DELETE', `${P(id)}/chat?mode=${mode}`),
  suggest: (id: string, goal?: string) => request<SuggestResult>('POST', `${P(id)}/suggest`, goal ? { goal } : {}),
  nudge: (id: string) => request<{ cands: SuggestResult['cands']; note: string }>('POST', `${P(id)}/nudge`),

  memory: (id: string) => request<Memory>('GET', `${P(id)}/memory`),
  deleteFact: (id: string, fid: string) => request<void>('DELETE', `${P(id)}/memory/${enc(fid)}`),
  refreshMemory: (id: string) => request<Memory>('POST', `${P(id)}/memory/refresh`),
  insights: (id: string) => request<Insights>('GET', `${P(id)}/insights`),
  history: (id: string) => request<HistoryItem[]>('GET', `${P(id)}/history`),

  importCurrent: (o: ImportOpts) => request<ImportJob>('POST', '/api/import/current', o),
  importChatsList: () => request<void>('POST', '/api/import/chats/list', {}),
  importBulk: (names: string[], o: ImportOpts) => request<ImportJob>('POST', '/api/import/bulk', { names, ...o }),
  importActive: () => request<ImportJob | null>('GET', '/api/import/active'),
  importGet: (id: string) => request<ImportJob>('GET', `/api/import/${enc(id)}`),
  importCancel: (id: string) => request<ImportJob>('POST', `/api/import/${enc(id)}/cancel`, {}),

  live: () => request<LiveInfo>('GET', '/api/live'),
  liveResync: () => request<{ ok: boolean }>('POST', '/api/live/resync', {}),
  liveSuggest: () => request<SuggestResult>('POST', '/api/live/suggest', {}),
  liveSend: (b: SendBody) => request<void>('POST', '/api/live/send', b),
  liveApprove: (b: SendBody & { cand_tur: string }) => request<void>('POST', '/api/live/approve', b),
}

/**
 * /api/events SSE akışını fetch ile okur (EventSource Authorization başlığı gönderemez).
 * Akış bitince sessizce döner; ağ/HTTP hatasında fırlatır.
 */
export async function streamEvents(onEvent: (e: ServerEvent) => void, signal: AbortSignal): Promise<void> {
  if (MOCK) {
    const { mockStream } = await import('./mock')
    return mockStream(onEvent, signal)
  }
  const res = await fetch('/api/events', { headers: { Accept: 'text/event-stream', ...authHeaders() }, signal })
  if (!res.ok || !res.body) throw await errorOf(res)
  const reader = res.body.getReader()
  const dec = new TextDecoder()
  const parser = new SSEParser()
  for (;;) {
    const { value, done } = await reader.read()
    if (done) return
    for (const m of parser.feed(dec.decode(value, { stream: true }))) {
      if (!m.data) continue
      try {
        onEvent(JSON.parse(m.data) as ServerEvent)
      } catch {
        /* bozuk çerçeveyi atla */
      }
    }
  }
}

/** Yeniden bağlanma bekleme süresi: 1s → 30s (ikiye katlanır). */
export const backoffMs = (attempt: number) => Math.min(30000, 1000 * 2 ** Math.max(0, attempt))
