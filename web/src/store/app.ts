import { create } from 'zustand'
import { api, backoffMs, streamEvents } from '@/lib/api'
import { fireNotification, pageHidden } from '@/lib/notify'
import { clock } from '@/lib/fmt'
import { importTransition, isActive, optsFrom } from '@/lib/importJob'
import { navigate, personHref } from '@/lib/route'
import type {
  AppState, ChatMode, ChatMsg, ChatRow, ImportJob, ImportOpts, LiveChat, Person, PersonPatch, ServerEvent,
  SettingsPatch, SuggestResult,
} from '@/lib/types'

export interface LiveSuggestions extends SuggestResult {
  chat_id: string
  incoming_key: string
  person_id?: string
  pending_approval: boolean
}
export interface SentLog {
  chat_id: string
  text: string
  auto: boolean
  ts: number
}
export interface Toast {
  text: string
  error?: boolean
  id: number
}
export type AddTab = 'current' | 'bulk' | 'ai'
export type SseStatus = 'idle' | 'connecting' | 'open' | 'retrying'

interface AppStore {
  state: AppState | null
  stateError: string
  people: Person[]
  peopleLoaded: boolean
  chats: Record<string, ChatMsg[]> // `${id}:${mode}`
  live: { connected: boolean; platform: '' | 'whatsapp' | 'instagram'; chat: LiveChat | null; chatId: string }
  suggestions: LiveSuggestions | null
  sentLog: SentLog[]
  aborted: string
  sse: SseStatus
  toast: Toast | null
  drawerOpen: boolean
  addOpen: boolean
  addTab: AddTab
  setupDismissed: boolean
  importJob: ImportJob | null
  /** SSE `chats` ile gelen sol sohbet listesi (null: henüz istenmedi). */
  chatList: ChatRow[] | null
  chatListLoading: boolean
  /** AI modu için 'Anladım' onayı (oturumluk, kalıcı değil). */
  aiConsent: boolean

  init: () => Promise<void>
  loadState: () => Promise<void>
  loadPeople: () => Promise<void>
  upsertPerson: (p: Person) => void
  removePerson: (id: string) => Promise<void>
  patchPerson: (id: string, patch: PersonPatch) => Promise<void>
  patchSettings: (s: SettingsPatch) => Promise<void>
  setPaused: (paused: boolean) => Promise<void>
  loadChat: (id: string, mode: ChatMode) => Promise<void>
  sendChat: (id: string, mode: ChatMode, text: string) => Promise<void>
  clearChat: (id: string, mode: ChatMode) => Promise<void>
  loadLive: () => Promise<void>
  setSuggestions: (s: LiveSuggestions | null) => void
  showToast: (text: string, error?: boolean) => void
  dismissToast: () => void
  setDrawer: (v: boolean) => void
  setAddOpen: (v: boolean) => void
  openAdd: (tab?: AddTab) => void
  setAiConsent: (v: boolean) => void
  setImportJob: (j: ImportJob | null) => void
  loadActiveImport: () => Promise<void>
  importCurrent: (o: ImportOpts) => Promise<ImportJob>
  importBulk: (names: string[], o: ImportOpts) => Promise<ImportJob>
  syncPerson: (id: string) => Promise<ImportJob | null>
  cancelImport: () => Promise<void>
  requestChatList: () => Promise<void>
  openInWhatsApp: (id: string) => Promise<void>
  dismissSetup: () => void
  onImportFinished: (j: ImportJob) => void
  handleEvent: (e: ServerEvent) => void
  startSSE: () => () => void
}

let toastSeq = 0
const SETUP_KEY = 'barney_setup_dismissed'
const lsGet = (k: string) => {
  try {
    return localStorage.getItem(k)
  } catch {
    return null
  }
}
const msg = (e: unknown) => (e instanceof Error ? e.message : String(e))

export const useApp = create<AppStore>((set, get) => ({
  state: null,
  stateError: '',
  people: [],
  peopleLoaded: false,
  chats: {},
  live: { connected: false, platform: '', chat: null, chatId: '' },
  suggestions: null,
  sentLog: [],
  aborted: '',
  sse: 'idle',
  toast: null,
  drawerOpen: false,
  addOpen: false,
  addTab: 'current',
  setupDismissed: lsGet(SETUP_KEY) === '1',
  importJob: null,
  chatList: null,
  chatListLoading: false,
  aiConsent: false,

  async init() {
    await Promise.all([get().loadState(), get().loadPeople(), get().loadLive(), get().loadActiveImport()])
  },
  async loadState() {
    try {
      set({ state: await api.state(), stateError: '' })
    } catch (e) {
      set({ stateError: msg(e) })
    }
  },
  async loadPeople() {
    try {
      set({ people: await api.people(), peopleLoaded: true })
    } catch (e) {
      set({ peopleLoaded: true })
      get().showToast(msg(e), true)
    }
  },
  upsertPerson(p) {
    set((s) => {
      const i = s.people.findIndex((x) => x.id === p.id)
      const people = i === -1 ? [...s.people, p] : s.people.map((x) => (x.id === p.id ? p : x))
      return { people, state: s.state ? { ...s.state, has_people: true } : s.state }
    })
  },
  async removePerson(id) {
    await api.deletePerson(id)
    set((s) => ({
      people: s.people.filter((p) => p.id !== id),
      state: s.state ? { ...s.state, has_people: s.people.length > 1 } : s.state,
    }))
  },
  async patchPerson(id, patch) {
    // iyimser güncelleme; hata olursa sunucudaki gerçeği yeniden yükle
    set((s) => ({ people: s.people.map((p) => (p.id === id ? { ...p, ...patch } : p)) }))
    try {
      const p = await api.patchPerson(id, patch)
      get().upsertPerson(p)
    } catch (e) {
      get().showToast(msg(e), true)
      void get().loadPeople()
    }
  },
  async patchSettings(s) {
    try {
      set({ state: await api.settings(s) })
    } catch (e) {
      get().showToast(msg(e), true)
      throw e
    }
  },
  async setPaused(paused) {
    try {
      set({ state: await (paused ? api.pause() : api.resume()) })
    } catch (e) {
      get().showToast(msg(e), true)
    }
  },
  async loadChat(id, mode) {
    try {
      const r = await api.chat(id, mode)
      set((s) => ({ chats: { ...s.chats, [`${id}:${mode}`]: r.messages ?? [] } }))
    } catch (e) {
      get().showToast(msg(e), true)
    }
  },
  async sendChat(id, mode, text) {
    const key = `${id}:${mode}`
    const tmp: ChatMsg = { id: 'tmp-' + Date.now(), role: 'user', text, ts: Math.floor(Date.now() / 1000) }
    set((s) => ({ chats: { ...s.chats, [key]: [...(s.chats[key] ?? []), tmp] } }))
    try {
      const r = await api.sendChat(id, mode, text)
      set((s) => ({
        chats: { ...s.chats, [key]: [...(s.chats[key] ?? []).filter((m) => m.id !== tmp.id), ...r.messages] },
      }))
    } catch (e) {
      set((s) => ({ chats: { ...s.chats, [key]: (s.chats[key] ?? []).filter((m) => m.id !== tmp.id) } }))
      get().showToast(msg(e), true)
      throw e
    }
  },
  async clearChat(id, mode) {
    try {
      await api.clearChat(id, mode)
      set((s) => ({ chats: { ...s.chats, [`${id}:${mode}`]: [] } }))
    } catch (e) {
      get().showToast(msg(e), true)
    }
  },
  async loadLive() {
    try {
      const l = await api.live()
      set((s) => ({
        live: { connected: l.connected, platform: l.platform, chat: l.chat ?? null, chatId: l.chat?.partner ?? s.live.chatId },
      }))
    } catch {
      /* canlı bilgi opsiyonel */
    }
  },
  setSuggestions: (s) => set({ suggestions: s }),
  showToast(text, error) {
    const id = ++toastSeq
    set({ toast: { text, error, id } })
    setTimeout(() => {
      if (get().toast?.id === id) set({ toast: null })
    }, error ? 6000 : 3500)
  },
  dismissToast: () => set({ toast: null }),
  setDrawer: (v) => set({ drawerOpen: v }),
  setAddOpen: (v) => set({ addOpen: v }),
  openAdd: (tab = 'current') => set({ addOpen: true, addTab: tab }),
  setAiConsent: (v) => set({ aiConsent: v }),
  setImportJob: (j) => set({ importJob: j }),
  async loadActiveImport() {
    try {
      const j = await api.importActive()
      // yalnız aktif işi al; bitmiş bir iş yenilemede yeniden "yeni" gibi görünmesin
      if (j && isActive(j)) set({ importJob: j })
      else if (!j && isActive(get().importJob)) set({ importJob: null })
    } catch {
      /* eski sunucu / uç nokta yok: sessiz */
    }
  },
  async importCurrent(o) {
    const j = await api.importCurrent(o)
    set({ importJob: j })
    return j
  },
  async importBulk(names, o) {
    const j = await api.importBulk(names, o)
    set({ importJob: j })
    return j
  },
  async syncPerson(id) {
    try {
      const j = await api.syncPerson(id, optsFrom(get().state))
      set({ importJob: j })
      get().showToast('Senkronizasyon başladı')
      return j
    } catch (e) {
      get().showToast(msg(e), true)
      return null
    }
  },
  async cancelImport() {
    const j = get().importJob
    if (!j) return
    try {
      const r = await api.importCancel(j.id)
      // SSE de gelecek; yine de hemen yansıt
      if (get().importJob?.id === r.id) set({ importJob: r })
    } catch (e) {
      get().showToast(msg(e), true)
    }
  },
  async requestChatList() {
    set({ chatListLoading: true, chatList: null })
    try {
      await api.importChatsList()
    } catch (e) {
      set({ chatListLoading: false })
      throw e
    }
  },
  async openInWhatsApp(id) {
    try {
      await api.openPerson(id)
      const name = get().people.find((p) => p.id === id)?.name
      get().showToast(name ? `WhatsApp'ta açılıyor: ${name}` : "WhatsApp'ta açılıyor")
    } catch (e) {
      get().showToast(msg(e), true)
    }
  },
  dismissSetup() {
    try {
      localStorage.setItem(SETUP_KEY, '1')
    } catch {
      /* yoksay */
    }
    set({ setupDismissed: true })
  },

  onImportFinished(j) {
    if (j.state === 'failed') {
      get().showToast(j.error || 'Okuma başarısız oldu', true)
      return
    }
    if (j.state === 'canceled') {
      get().showToast('Okuma durduruldu')
      void get().loadPeople()
      return
    }
    // done
    const first = j.person_ids?.[0]
    void get().loadPeople().then(() => {
      void get().loadState()
      if (j.kind === 'sync') {
        get().showToast(`Senkronizasyon bitti · ${j.loaded.toLocaleString('tr-TR')} mesaj okundu`)
        return
      }
      const n = j.person_ids?.length ?? 0
      get().showToast(n > 1 ? `${n} kişi eklendi` : `Kişi eklendi · ${j.loaded.toLocaleString('tr-TR')} mesaj`)
      if (first && get().people.some((p) => p.id === first)) navigate(personHref(first))
    })
  },

  handleEvent(e) {
    const s = get()
    switch (e.type) {
      case 'state':
        set({ state: e.state })
        break
      case 'live':
        set((st) => ({
          live: { ...st.live, connected: true, chat: e.live, chatId: st.live.chatId || e.live.partner },
          // yeni gelen mesaj → eski öneriler bayatlar
          suggestions: st.suggestions && st.suggestions.incoming_key !== e.live.incoming_key ? null : st.suggestions,
        }))
        break
      case 'suggestions': {
        set((st) => ({
          suggestions: {
            cands: e.cands, recommended: e.recommended, note: e.note, chat_id: e.chat_id,
            incoming_key: e.incoming_key, person_id: e.person_id, pending_approval: !!e.pending_approval,
          },
          live: { ...st.live, chatId: e.chat_id },
        }))
        if (s.state?.notifications && pageHidden()) {
          fireNotification('Barney', e.pending_approval ? 'Onay bekleyen bir cevap var' : 'Yeni cevap önerileri hazır')
        }
        break
      }
      case 'sent':
        set((st) => ({
          sentLog: [{ chat_id: e.chat_id, text: e.text, auto: e.auto, ts: Math.floor(Date.now() / 1000) }, ...st.sentLog].slice(0, 20),
          suggestions: null,
        }))
        s.showToast(`${e.auto ? 'Otomatik gönderildi' : 'Gönderildi'} · ${clock(Date.now() / 1000)}`)
        break
      case 'aborted':
        set({ aborted: e.reason })
        s.showToast(`Gönderim durduruldu: ${e.reason}`, true)
        break
      case 'import': {
        const { job, finished } = importTransition(s.importJob, e.job)
        set({ importJob: job })
        if (finished) get().onImportFinished(finished)
        break
      }
      case 'chats':
        set({ chatList: e.chats, chatListLoading: false })
        break
      case 'resynced':
        s.showToast(e.known ? `Senk: ${e.seen} mesaj okundu · ${e.added} yeni eklendi` : `Senk: ${e.seen} mesaj okundu (${e.partner} kayıtlı değil)`)
        if (e.added > 0) void get().loadPeople()
        break
      case 'notify':
        if (s.state?.notifications && pageHidden()) fireNotification(e.title, e.body)
        else s.showToast(`${e.title}: ${e.body}`)
        break
    }
  },

  /** /api/events'e bağlanır; kopunca 1s→30s geri çekilmeyle yeniden dener. Dönen işlev bağlantıyı keser. */
  startSSE() {
    const ctrl = new AbortController()
    let attempt = 0
    ;(async () => {
      while (!ctrl.signal.aborted) {
        set({ sse: attempt === 0 ? 'connecting' : 'retrying' })
        try {
          let first = true
          await streamEvents((e) => {
            if (first) {
              first = false
              attempt = 0
              set({ sse: 'open' })
              // yeniden bağlanınca kaçırılanları tazele
              void get().loadPeople()
              void get().loadLive()
              void get().loadActiveImport()
            }
            get().handleEvent(e)
          }, ctrl.signal)
        } catch {
          /* ağ/HTTP hatası: geri çekil */
        }
        if (ctrl.signal.aborted) break
        set({ sse: 'retrying' })
        await new Promise((r) => setTimeout(r, backoffMs(attempt++)))
      }
    })()
    return () => {
      ctrl.abort()
      set({ sse: 'idle' })
    }
  },
}))
