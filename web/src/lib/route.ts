import { useEffect, useState } from 'react'

export type TabId = 'chat' | 'live' | 'style' | 'insights' | 'memory' | 'history' | 'settings'
export const TABS: { id: TabId; label: string }[] = [
  { id: 'chat', label: 'Sohbet' },
  { id: 'live', label: 'Canlı' },
  { id: 'style', label: 'Stil' },
  { id: 'insights', label: 'Analiz' },
  { id: 'memory', label: 'Hafıza' },
  { id: 'history', label: 'Geçmiş' },
  { id: 'settings', label: 'Ayarlar' },
]

export type Route =
  | { page: 'home' }
  | { page: 'settings' }
  | { page: 'person'; id: string; tab: TabId }

/** `#/p/<id>/<tab>`, `#/settings`; anlaşılmayan her şey home. */
export function parseRoute(hash: string): Route {
  const h = hash.replace(/^#/, '')
  const parts = h.split('&')[0].split('/').filter(Boolean)
  if (parts[0] === 'settings') return { page: 'settings' }
  if (parts[0] === 'p' && parts[1]) {
    const tab = TABS.some((t) => t.id === parts[2]) ? (parts[2] as TabId) : 'chat'
    return { page: 'person', id: decodeURIComponent(parts[1]), tab }
  }
  return { page: 'home' }
}

export const personHref = (id: string, tab: TabId = 'chat') => `#/p/${encodeURIComponent(id)}/${tab}`
export const SETTINGS_HREF = '#/settings'

export function useRoute(): Route {
  const [hash, setHash] = useState(() => window.location.hash)
  useEffect(() => {
    const on = () => setHash(window.location.hash)
    window.addEventListener('hashchange', on)
    return () => window.removeEventListener('hashchange', on)
  }, [])
  return parseRoute(hash)
}

export function navigate(href: string) {
  window.location.hash = href
}

export const isEmbed = () => new URLSearchParams(window.location.search).get('embed') === '1'
