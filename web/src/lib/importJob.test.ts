import { describe, expect, it } from 'vitest'
import { importTransition, isActive, optsFrom } from './importJob'
import type { ImportJob } from './types'

const j = (o: Partial<ImportJob>): ImportJob => ({ id: 'a', kind: 'current', state: 'running', mode: 'reader', loaded: 0, chat: '', i: 0, n: 1, person_ids: [], note: '', log: [], started_at: 0, ...o })

describe('importTransition', () => {
  it('bitişi yalnız ilk görüşte bildirir', () => {
    const run = j({})
    expect(importTransition(null, run).finished).toBeNull()
    const done = j({ state: 'done' })
    const r = importTransition(run, done)
    expect(r.finished).toBe(done)
    expect(importTransition(r.job, done).finished).toBeNull()
  })
  it('hiç görülmemiş işin bitişi de bildirilir (hızlı iş yarışı)', () => {
    expect(importTransition(null, j({ state: 'failed' })).finished).toBeTruthy()
  })
  it('yeni iş kimliği önceki bitmiş işten bağımsızdır', () => {
    expect(importTransition(j({ state: 'done' }), j({ id: 'b', state: 'canceled' })).finished).toBeTruthy()
  })
})
describe('yardımcılar', () => {
  it('isActive', () => {
    expect(isActive(j({ state: 'queued' }))).toBe(true)
    expect(isActive(j({ state: 'done' }))).toBe(false)
    expect(isActive(null)).toBe(false)
  })
  it('optsFrom State.history varsayılanını kullanır', () => {
    expect(optsFrom(null, 'ai')).toEqual({ max_messages: 3000, max_seconds: 300, all: false, mode: 'ai' })
    expect(optsFrom({ history: { max_messages: 9, max_seconds: 8, all: true } })).toEqual({ max_messages: 9, max_seconds: 8, all: true, mode: 'reader' })
  })
})
