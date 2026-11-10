import { api } from './api'
import { useApp } from '@/store/app'
import type { Cand, Person, SendBody, Step } from './types'

/** Bu kişiye ait açık bir canlı sohbet var mı? */
export function useLiveFor(person: Person) {
  const live = useApp((s) => s.live)
  const ok = live.connected && !!live.chat && live.chat.person_id === person.id
  return { ok, live }
}

/** Açık sohbet için chat_id/incoming_key: önce SSE önerisinin anahtarları, yoksa /api/live durumu. */
export function liveKeys(source: 'suggestion' | 'current' = 'suggestion'): { chat_id: string; incoming_key: string } | null {
  const { suggestions, live } = useApp.getState()
  // 'current': kullanıcı bu cevabı şimdi (Sohbet sekmesinden) üretti → açık sohbetin güncel anahtarı.
  // 'suggestion': canlı bir öneri → o önerinin anahtarı; araya yeni mesaj girdiyse uzantı bayat diye reddeder.
  if (source === 'suggestion' && suggestions) return { chat_id: suggestions.chat_id, incoming_key: suggestions.incoming_key }
  if (live.chat) return { chat_id: live.chatId || live.chat.partner, incoming_key: live.chat.incoming_key }
  return null
}

export async function sendToLive(steps: Step[], cand: Cand, mode: 'send' | 'draft' | 'approve', source: 'suggestion' | 'current' = 'suggestion'): Promise<boolean> {
  const { showToast } = useApp.getState()
  const keys = liveKeys(source)
  if (!keys) {
    showToast('Açık bir sohbet yok.', true)
    return false
  }
  const body: SendBody = { ...keys, steps, cand_tur: cand.tur }
  if (mode === 'draft') body.draft = true
  try {
    if (mode === 'approve') await api.liveApprove({ ...body, cand_tur: cand.tur })
    else await api.liveSend(body)
    showToast(mode === 'draft' ? 'Mesaj sohbet kutusuna yazıldı.' : mode === 'approve' ? 'Onaylandı, gönderiliyor…' : 'Gönderiliyor…')
    return true
  } catch (e) {
    showToast(e instanceof Error ? e.message : String(e), true)
    return false
  }
}
