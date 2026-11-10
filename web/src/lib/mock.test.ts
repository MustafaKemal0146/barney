import { describe, expect, it, vi } from 'vitest'
import { mockCall, mockStream } from './mock'
import type { AppState, ImportJob, Person, ServerEvent, SuggestResult } from './types'

describe('mock API', () => {
  it('state ve kişi listesi döner', async () => {
    const st = (await mockCall('GET', '/api/state')) as { key_set: boolean; has_people: boolean }
    expect(st.key_set).toBe(true)
    const ppl = (await mockCall('GET', '/api/people')) as Person[]
    expect(ppl.length).toBeGreaterThanOrEqual(2)
    expect(ppl[0].tone.preset).toBeTruthy()
  })
  it('PATCH tone kalıcıdır', async () => {
    const p = (await mockCall('PATCH', '/api/people/p2', { tone: { preset: 'flort', flort: 9, cesaret: 5, mizah: 5, samimiyet: 5 } })) as Person
    expect(p.tone.flort).toBe(9)
    const again = (await mockCall('GET', '/api/people/p2')) as Person
    expect(again.tone.preset).toBe('flort')
  })
  it('rol provasında assistant steps döner', async () => {
    const r = (await mockCall('POST', '/api/people/p1/chat', { mode: 'roleplay', text: 'kahve içelim mi' })) as { messages: { role: string; steps?: unknown[] }[] }
    expect(r.messages.map((m) => m.role)).toEqual(['user', 'assistant'])
    expect(r.messages[1].steps!.length).toBeGreaterThan(1)
  })
  it('suggest aday ve wait_sec içerir', async () => {
    const r = (await mockCall('POST', '/api/people/p1/suggest', {})) as SuggestResult
    expect(r.cands.length).toBeGreaterThan(1)
    expect(r.cands[0].steps[0].wait_sec).toBeGreaterThan(0)
  })
  it('olmayan kişi 404', async () => {
    await expect(mockCall('GET', '/api/people/yok')).rejects.toMatchObject({ status: 404 })
  })
  it('SSE akışı ilk olarak state yollar ve iptalde biter', async () => {
    const got: ServerEvent[] = []
    const ctrl = new AbortController()
    const done = mockStream((e) => got.push(e), ctrl.signal)
    ctrl.abort()
    await done
    expect(got[0].type).toBe('state')
  })
})

describe('mock: içe aktarma uçları', () => {
  const opts = { max_messages: 500, max_seconds: 60, all: false, mode: 'reader' as const }
  it('state yeni alanları içerir ve ayarlar kaydedilir', async () => {
    const st = (await mockCall('POST', '/api/settings', { history_max_messages: 1234, history_all: true, auto_learn: true, bulk_speed: 'hizli' })) as AppState
    expect(st.history).toMatchObject({ max_messages: 1234, all: true })
    expect(st.auto_learn).toBe(true)
    expect(st.bulk_speed).toBe('hizli')
    await mockCall('POST', '/api/settings', { history_max_messages: 3000, history_all: false, auto_learn: false, bulk_speed: 'yavas' })
  })
  it('sohbet listesi SSE ile ~8 satır gelir', async () => {
    vi.useFakeTimers()
    const got: ServerEvent[] = []
    const ctrl = new AbortController()
    const done = mockStream((e) => got.push(e), ctrl.signal)
    const p = mockCall('POST', '/api/import/chats/list', {})
    await vi.advanceTimersByTimeAsync(300)
    await p
    await vi.advanceTimersByTimeAsync(1500)
    ctrl.abort()
    await done
    vi.useRealTimers()
    const ev = got.find((e) => e.type === 'chats') as Extract<ServerEvent, { type: 'chats' }>
    expect(ev.chats).toHaveLength(8)
  })
  it('tek iş ilerler, günlük üretir, kişiyi oluşturur; ikinci iş 409', async () => {
    vi.useFakeTimers()
    const got: ImportJob[] = []
    const ctrl = new AbortController()
    const done = mockStream((e) => { if (e.type === 'import') got.push(e.job) }, ctrl.signal)
    const p = mockCall('POST', '/api/import/current', opts)
    await vi.advanceTimersByTimeAsync(300)
    const j = (await p) as ImportJob
    expect(j.state).toBe('queued')
    const p2 = mockCall('POST', '/api/import/current', opts)
    const p2r = expect(p2).rejects.toMatchObject({ status: 409 })
    await vi.advanceTimersByTimeAsync(300)
    await p2r
    const a = mockCall('GET', '/api/import/active')
    await vi.advanceTimersByTimeAsync(300)
    expect(((await a) as ImportJob).id).toBe(j.id)
    await vi.advanceTimersByTimeAsync(8000)
    ctrl.abort()
    await done
    vi.useRealTimers()
    const last = got[got.length - 1]
    expect(last.state).toBe('done')
    expect(last.log.length).toBeGreaterThan(3)
    expect(last.person_ids).toHaveLength(1)
  })
  it('iptal işi canceled yapar', async () => {
    vi.useFakeTimers()
    const p = mockCall('POST', '/api/import/bulk', { names: ['A', 'B', 'C'], ...opts })
    await vi.advanceTimersByTimeAsync(300)
    const j = (await p) as ImportJob
    expect(j.n).toBe(3)
    await vi.advanceTimersByTimeAsync(1500)
    const c = mockCall('POST', `/api/import/${j.id}/cancel`, {})
    await vi.advanceTimersByTimeAsync(300)
    expect(((await c) as ImportJob).state).toBe('canceled')
    vi.useRealTimers()
  })
  it('toplu ekleme en fazla 10 sohbet kabul eder', async () => {
    vi.useFakeTimers()
    const p = expect(mockCall('POST', '/api/import/bulk', { names: Array.from({ length: 11 }, (_, i) => 'k' + i), ...opts })).rejects.toMatchObject({ status: 400 })
    await vi.advanceTimersByTimeAsync(300)
    await p
    vi.useRealTimers()
  })
})
