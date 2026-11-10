import { useEffect, useRef, useState } from 'react'
import { Lightbulb, Send, SlidersHorizontal, Sparkles, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Segmented, Spinner } from '@/components/kit'
import { ToneBar } from './ToneBar'
import { CandidateList, StepBubbles, type ActionSpec } from './Candidates'
import { api } from '@/lib/api'
import { clock, silenceDays, silenceExceeded } from '@/lib/fmt'
import { sendToLive, useLiveFor } from '@/lib/liveActions'
import { cn } from '@/lib/utils'
import { useApp } from '@/store/app'
import type { ChatMode, ChatMsg, Person, SuggestResult } from '@/lib/types'

const EMPTY: ChatMsg[] = []

function Message({ m, mode, partner }: { m: ChatMsg; mode: ChatMode; partner: string }) {
  if (m.role === 'user') {
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%] rounded-2xl rounded-br-sm bg-primary px-3 py-2 text-sm break-words whitespace-pre-wrap text-primary-foreground">
          {m.text}
          <div className="mt-0.5 text-right text-[10px] opacity-70">{clock(m.ts)}</div>
        </div>
      </div>
    )
  }
  if (mode === 'roleplay') {
    return (
      <div className="max-w-[85%]">
        <div className="mb-1 text-[11px] text-muted-foreground">{partner} (rol)</div>
        <StepBubbles steps={m.steps?.length ? m.steps : [{ text: m.text, wait_sec: 0 }]} />
      </div>
    )
  }
  return (
    <div className="max-w-[88%]">
      <div className="mb-1 text-[11px] text-muted-foreground">Barney · danışman</div>
      <div className="rounded-2xl rounded-tl-sm border bg-card px-3 py-2 text-sm break-words whitespace-pre-wrap">{m.text}</div>
    </div>
  )
}

export function ChatTab({ person }: { person: Person }) {
  const [mode, setMode] = useState<ChatMode>('advisor')
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const [suggesting, setSuggesting] = useState(false)
  const [result, setResult] = useState<SuggestResult | null>(null)
  const [toneOpen, setToneOpen] = useState(false)
  const key = `${person.id}:${mode}`
  const msgs = useApp((s) => s.chats[key] ?? EMPTY)
  const loadChat = useApp((s) => s.loadChat)
  const sendChat = useApp((s) => s.sendChat)
  const clearChat = useApp((s) => s.clearChat)
  const toast = useApp((s) => s.showToast)
  const { ok: liveOk } = useLiveFor(person)
  const scroller = useRef<HTMLDivElement>(null)
  const loaded = useApp((s) => key in s.chats)

  useEffect(() => { setResult(null); void loadChat(person.id, mode) }, [person.id, mode, loadChat])
  useEffect(() => { scroller.current?.scrollTo({ top: scroller.current.scrollHeight, behavior: 'smooth' }) }, [msgs.length, result, sending])

  const submit = async () => {
    const t = text.trim()
    if (!t || sending) return
    setText('')
    setSending(true)
    try { await sendChat(person.id, mode, t) } catch { setText(t) } finally { setSending(false) }
  }
  const suggest = async (kind: 'suggest' | 'nudge') => {
    setSuggesting(true)
    try {
      const r = kind === 'suggest' ? await api.suggest(person.id, person.goal || undefined) : await api.nudge(person.id)
      setResult({ cands: r.cands, recommended: 'recommended' in r ? (r as SuggestResult).recommended : 0, note: r.note })
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), true)
    } finally { setSuggesting(false) }
  }

  const why = 'Bu kişiyle açık bir canlı sohbet yok. WhatsApp Web / Instagram’da bu kişinin sohbetini aç.'
  const actions: ActionSpec[] = [
    { label: 'Kutuya yaz', disabled: !liveOk, disabledReason: why, onClick: (s, c) => void sendToLive(s, c, 'draft', 'current') },
    { label: 'Gönder', primary: true, disabled: !liveOk, disabledReason: why, onClick: (s, c) => void sendToLive(s, c, 'send', 'current') },
  ]
  const quiet = silenceExceeded(person)

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-3 py-2">
        <Segmented<ChatMode> value={mode} onChange={setMode} options={[{ value: 'advisor', label: 'Danışman' }, { value: 'roleplay', label: 'Rol provası' }]} />
        <Button variant="ghost" size="sm" onClick={() => setToneOpen((v) => !v)} aria-expanded={toneOpen} className="ml-auto">
          <SlidersHorizontal /> Ton · {person.tone.preset}
        </Button>
        <Button variant="ghost" size="icon-sm" aria-label="Sohbeti temizle" title="Bu sohbeti temizle" disabled={!msgs.length} onClick={() => void clearChat(person.id, mode)}>
          <Trash2 />
        </Button>
      </div>
      {toneOpen && <div className="shrink-0 border-b p-3"><ToneBar person={person} /></div>}

      <div ref={scroller} className="min-h-0 flex-1 space-y-3 overflow-y-auto p-3 sm:p-4">
        {!loaded && <div className="flex justify-center py-6"><Spinner /></div>}
        {loaded && msgs.length === 0 && (
          <div className="mx-auto max-w-sm py-10 text-center text-sm text-muted-foreground">
            {mode === 'advisor'
              ? `${person.name} hakkında ne yapmak istediğini sor: nasıl cevap vereyim, görüşme nasıl teklif edilir…`
              : `${person.name} rolündeki Barney’e yaz. Cevabı onun gibi, birkaç mesaj halinde ve bekleme süreleriyle verir.`}
          </div>
        )}
        {msgs.map((m) => <Message key={m.id} m={m} mode={mode} partner={person.name} />)}
        {sending && <div className="flex items-center gap-2 text-xs text-muted-foreground"><span className="dot-pulse size-1.5 rounded-full bg-primary" /> yazıyor…</div>}
        {result && (
          <div className="border-t pt-3">
            <div className="mb-2 flex items-center justify-between">
              <h4 className="text-sm font-semibold">Cevap adayları</h4>
              <Button variant="ghost" size="xs" onClick={() => setResult(null)}>Gizle</Button>
            </div>
            <CandidateList result={result} actions={actions} />
          </div>
        )}
      </div>

      <div className="shrink-0 space-y-2 border-t p-3">
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" disabled={suggesting} onClick={() => void suggest('suggest')}>
            {suggesting ? <Spinner className="size-3.5" /> : <Sparkles />} Cevap öner
          </Button>
          {quiet && (
            <Button variant="outline" size="sm" disabled={suggesting} onClick={() => void suggest('nudge')} title="Sessizliği bozacak konu aç">
              <Lightbulb /> Konu aç ({silenceDays(person)} gün)
            </Button>
          )}
        </div>
        <div className="flex items-end gap-2">
          <Textarea
            value={text}
            rows={1}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); void submit() } }}
            placeholder={mode === 'advisor' ? 'Barney’e danış…' : `${person.name}’e yaz (rol provası)…`}
            aria-label="Mesaj"
            className={cn('max-h-32 min-h-9 flex-1')}
          />
          <Button onClick={() => void submit()} disabled={!text.trim() || sending} aria-label="Gönder" size="icon"><Send /></Button>
        </div>
      </div>
    </div>
  )
}
