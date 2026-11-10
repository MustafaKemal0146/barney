import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImportJob, Person } from '@/lib/types'

const people: Person[] = [{ id: 'pn', name: 'Yeni' } as Person]
vi.mock('@/lib/api', async (orig) => {
  const m = await orig<typeof import('@/lib/api')>()
  return {
    ...m,
    api: {
      ...m.api,
      people: vi.fn(async () => people),
      state: vi.fn(async () => ({ history: { max_messages: 3000, max_seconds: 300, all: false } })),
      importActive: vi.fn(),
      importCancel: vi.fn(),
      importChatsList: vi.fn(async () => undefined),
      importCurrent: vi.fn(),
      syncPerson: vi.fn(),
      openPerson: vi.fn(),
    },
  }
})
import { api } from '@/lib/api'
import { useApp } from './app'

const job = (o: Partial<ImportJob> = {}): ImportJob => ({
  id: 'j1', kind: 'current', state: 'running', mode: 'reader', loaded: 10, chat: 'Yeni', i: 0, n: 1,
  person_ids: [], note: '', log: [], started_at: 1, ...o,
})
const flush = () => new Promise((r) => setTimeout(r, 0))

beforeEach(() => {
  window.location.hash = ''
  useApp.setState({ importJob: null, chatList: null, chatListLoading: false, toast: null, people: [], aiConsent: false })
  vi.clearAllMocks()
})

describe('import SSE olayları', () => {
  it('ilerleme olayı importJob günceller, bitişte toast + kişiye yönlendirme (bir kez)', async () => {
    const { handleEvent } = useApp.getState()
    handleEvent({ type: 'import', job: job() })
    expect(useApp.getState().importJob?.loaded).toBe(10)
    handleEvent({ type: 'import', job: job({ loaded: 500 }) })
    expect(useApp.getState().importJob?.loaded).toBe(500)
    expect(useApp.getState().toast).toBeNull()

    const done = job({ state: 'done', loaded: 900, person_ids: ['pn'] })
    handleEvent({ type: 'import', job: done })
    await flush()
    expect(useApp.getState().importJob?.state).toBe('done')
    expect(useApp.getState().toast?.text).toContain('Kişi eklendi')
    expect(window.location.hash).toBe('#/p/pn/chat')

    // aynı bitiş olayı tekrarlanırsa ikinci kez tetiklenmez
    window.location.hash = ''
    handleEvent({ type: 'import', job: done })
    await flush()
    expect(window.location.hash).toBe('')
  })

  it('başarısız iş hata toast’ı gösterir, yönlendirmez', async () => {
    useApp.getState().handleEvent({ type: 'import', job: job({ state: 'failed', error: 'Sekme yok', can_fallback_ai: true }) })
    await flush()
    expect(useApp.getState().toast).toMatchObject({ text: 'Sekme yok', error: true })
    expect(useApp.getState().importJob?.can_fallback_ai).toBe(true)
    expect(window.location.hash).toBe('')
  })

  it('senkronizasyon bitince yönlendirmez', async () => {
    useApp.getState().handleEvent({ type: 'import', job: job({ kind: 'sync', state: 'done', person_ids: ['pn'] }) })
    await flush()
    expect(window.location.hash).toBe('')
    expect(useApp.getState().toast?.text).toContain('Senkronizasyon')
  })

  it('chats olayı listeyi doldurur ve yüklemeyi kapatır', async () => {
    await useApp.getState().requestChatList()
    expect(useApp.getState().chatListLoading).toBe(true)
    useApp.getState().handleEvent({ type: 'chats', chats: [{ name: 'A', last: 'x', unread: 0 }] })
    expect(useApp.getState().chatList).toHaveLength(1)
    expect(useApp.getState().chatListLoading).toBe(false)
  })
})

describe('aktif iş yükleme ve durdurma', () => {
  it('loadActiveImport aktif işi alır; bitmişi almaz', async () => {
    vi.mocked(api.importActive).mockResolvedValueOnce(job())
    await useApp.getState().loadActiveImport()
    expect(useApp.getState().importJob?.id).toBe('j1')
    vi.mocked(api.importActive).mockResolvedValueOnce(null)
    await useApp.getState().loadActiveImport()
    expect(useApp.getState().importJob).toBeNull()
    useApp.setState({ importJob: null })
    vi.mocked(api.importActive).mockResolvedValueOnce(job({ state: 'done' }))
    await useApp.getState().loadActiveImport()
    expect(useApp.getState().importJob).toBeNull()
  })
  it('cancelImport iş kimliğiyle iptal uç noktasını çağırır', async () => {
    useApp.setState({ importJob: job() })
    vi.mocked(api.importCancel).mockResolvedValueOnce(job({ state: 'canceled' }))
    await useApp.getState().cancelImport()
    expect(api.importCancel).toHaveBeenCalledWith('j1')
    expect(useApp.getState().importJob?.state).toBe('canceled')
  })
  it('syncPerson varsayılan derinlikle çağrılır; hatada toast', async () => {
    useApp.setState({ state: { history: { max_messages: 800, max_seconds: 60, all: false } } as never })
    vi.mocked(api.syncPerson).mockResolvedValueOnce(job({ kind: 'sync' }))
    await useApp.getState().syncPerson('p1')
    expect(api.syncPerson).toHaveBeenCalledWith('p1', { max_messages: 800, max_seconds: 60, all: false, mode: 'reader' })
    vi.mocked(api.syncPerson).mockRejectedValueOnce(new Error('başka bir iş çalışıyor'))
    expect(await useApp.getState().syncPerson('p1')).toBeNull()
    expect(useApp.getState().toast).toMatchObject({ text: 'başka bir iş çalışıyor', error: true })
  })
  it('openInWhatsApp hata mesajını toast eder', async () => {
    vi.mocked(api.openPerson).mockRejectedValueOnce(new Error('WhatsApp sekmesi bağlı değil'))
    await useApp.getState().openInWhatsApp('p1')
    expect(useApp.getState().toast?.error).toBe(true)
  })
})

describe('canlı şerit kuralı', () => {
  it('live olayı rotayı değiştirmez', () => {
    window.location.hash = '#/p/p1/chat'
    useApp.getState().handleEvent({ type: 'live', live: { partner: 'Başkası', history: [], incoming_key: 'k', person_id: 'p2' } })
    expect(window.location.hash).toBe('#/p/p1/chat')
    expect(useApp.getState().live.chat?.person_id).toBe('p2')
  })
})
