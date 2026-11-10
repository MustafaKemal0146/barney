import { beforeEach, describe, expect, it } from 'vitest'
import { backoffMs, extractToken, getToken, initToken } from './api'
import { parseRoute } from './route'

describe('extractToken', () => {
  it('yalnız token kısmını ayırır, rotayı korur', () => {
    expect(extractToken('#token=abc123')).toEqual({ token: 'abc123', rest: '' })
    expect(extractToken('#token=abc&/p/1/live')).toEqual({ token: 'abc', rest: '#/p/1/live' })
    expect(extractToken('#/p/1/chat&token=zz_9-A')).toEqual({ token: 'zz_9-A', rest: '#/p/1/chat' })
    expect(extractToken('#/settings')).toEqual({ token: null, rest: '#/settings' })
    expect(extractToken('')).toEqual({ token: null, rest: '' })
  })
})

describe('initToken', () => {
  beforeEach(() => sessionStorage.clear())
  it('sessionStorage’a koyar ve adres çubuğundan siler', () => {
    window.history.replaceState(null, '', '/?embed=1#token=tok123')
    expect(initToken()).toBe('tok123')
    expect(getToken()).toBe('tok123')
    expect(window.location.hash).toBe('')
    expect(window.location.search).toBe('?embed=1')
  })
})

describe('backoff ve rota', () => {
  it('1s → 30s', () => {
    expect([0, 1, 2, 3, 4, 5, 6, 10].map(backoffMs)).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000, 30000])
  })
  it('parseRoute', () => {
    expect(parseRoute('#/p/p1/live')).toEqual({ page: 'person', id: 'p1', tab: 'live' })
    expect(parseRoute('#/p/p1/bilinmeyen')).toEqual({ page: 'person', id: 'p1', tab: 'chat' })
    expect(parseRoute('#/settings')).toEqual({ page: 'settings' })
    expect(parseRoute('')).toEqual({ page: 'home' })
  })
})
