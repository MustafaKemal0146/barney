import { useEffect, useRef, useState } from 'react'
import { BellRing, Bot, ChevronDown, SlidersHorizontal, Zap, CheckCheck, ExternalLink, MessageCircle, Radio, ShieldCheck, Sparkles } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Card, ConfirmDialog, Empty, SectionTitle, Spinner, Stat } from '@/components/kit'
import { PRESETS, ToneBar } from './ToneBar'
import { CandidateList, StepBubbles, type ActionSpec } from './Candidates'
import { api } from '@/lib/api'
import { clock } from '@/lib/fmt'
import { sendToLive } from '@/lib/liveActions'
import { personHref } from '@/lib/route'
import { useApp } from '@/store/app'
import type { Person } from '@/lib/types'

function GuardList({ p }: { p: Person }) {
  const g = p.guards
  const items = [
    `İlk ${g.first_n_approvals} cevap her zaman senin onayını bekler (şu an ${Math.min(p.approvals_done, g.first_n_approvals)}/${g.first_n_approvals}).`,
    g.quiet_on ? `Sessiz saatlerde (${g.quiet_start}–${g.quiet_end}) gönderilmez.` : 'Sessiz saat koruması kapalı.',
    `Saatte en fazla ${g.max_per_hour || 'sınırsız'}, günde en fazla ${g.max_per_day || 'sınırsız'} cevap.`,
    g.stop_sensitive ? 'Hassas konu algılanırsa durur ve sana sorar.' : 'Hassas konuda durma koruması KAPALI.',
    'Stil denetimini geçemeyen ya da “engellendi” işaretli adaylar gönderilmez.',
    'Üstteki “Durdur” düğmesi her şeyi anında keser.',
  ]
  return (
    <ul className="space-y-1.5 text-sm">
      {items.map((t, i) => <li key={i} className="flex gap-2"><ShieldCheck className="mt-0.5 size-4 shrink-0 text-success" />{t}</li>)}
    </ul>
  )
}

export function LiveTab({ person }: { person: Person }) {
  const live = useApp((s) => s.live)
  const state = useApp((s) => s.state)
  const sugg = useApp((s) => s.suggestions)
  const setSugg = useApp((s) => s.setSuggestions)
  const sentLog = useApp((s) => s.sentLog)
  const loadLive = useApp((s) => s.loadLive)
  const patchPerson = useApp((s) => s.patchPerson)
  const toast = useApp((s) => s.showToast)
  const [busy, setBusy] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const [understood, setUnderstood] = useState(false)
  const [confirmFull, setConfirmFull] = useState(false)
  const [toneOpen, setToneOpen] = useState(false)
  const [understoodFull, setUnderstoodFull] = useState(false)
  const scroller = useRef<HTMLDivElement>(null)

  useEffect(() => { void loadLive() }, [loadLive])
  useEffect(() => { scroller.current?.scrollTo({ top: scroller.current.scrollHeight }) }, [live.chat?.history.length])

  const chat = live.chat
  const mine = !!chat && (!chat.person_id || chat.person_id === person.id)
  const other = chat?.person_id && chat.person_id !== person.id ? chat.person_id : ''
  const otherName = useApp((s) => s.people.find((p) => p.id === other)?.name)
  const platform = live.platform === 'whatsapp' ? 'WhatsApp' : live.platform === 'instagram' ? 'Instagram' : ''
  const mySugg = sugg && (!sugg.person_id || sugg.person_id === person.id) ? sugg : null
  const pending = !!mySugg?.pending_approval

  const suggestNow = async () => {
    setBusy(true)
    try {
      const r = await api.liveSuggest()
      setSugg({ ...r, chat_id: live.chatId || chat?.partner || '', incoming_key: chat?.incoming_key ?? '', person_id: chat?.person_id, pending_approval: false })
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), true)
    } finally { setBusy(false) }
  }

  const sendActions: ActionSpec[] = [
    { label: 'Gönder', primary: true, disabled: state?.paused, disabledReason: 'Barney durdurulmuş. Sürdür düğmesine bas.', onClick: (s, c) => void sendToLive(s, c, 'send') },
    { label: 'Kutuya yaz', onClick: (s, c) => void sendToLive(s, c, 'draft') },
  ]
  const recCand = mySugg?.cands[mySugg.recommended]

  // Öneriyi listeden siler (hiçbir şey gönderilmez); sonuncuysa kutu boşalır.
  const dismissCand = (i: number) => {
    if (!mySugg) return
    const cands = mySugg.cands.filter((_, j) => j !== i)
    if (cands.length === 0) return setSugg(null)
    const rec = mySugg.recommended === i ? 0 : mySugg.recommended > i ? mySugg.recommended - 1 : mySugg.recommended
    setSugg({ ...mySugg, cands, recommended: rec, pending_approval: cands.length ? mySugg.pending_approval : false })
  }

  const toggleAuto = (v: boolean) => {
    if (v) { setUnderstood(false); setConfirm(true) }
    else void patchPerson(person.id, { auto_send: false })
  }

  const toggleFull = (v: boolean) => {
    if (v) { setUnderstoodFull(false); setConfirmFull(true) }
    else void patchPerson(person.id, { full_auto: false })
  }

  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto">
      <div className="mx-auto w-full max-w-5xl space-y-4 p-3 sm:p-4">
        {/* Durum + Otomatik */}
        <Card>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-3">
              <span className={cn('flex size-9 items-center justify-center rounded-full', live.connected ? 'bg-success/15 text-success' : 'bg-muted text-muted-foreground')}>
                <Radio className="size-4" />
              </span>
              <div>
                <div className="text-sm font-medium">
                  {live.connected ? (chat ? <>Açık sohbet: <b>{chat.partner}</b></> : 'Uzantı bağlı, açık sohbet yok') : 'Uzantı bağlı değil'}
                </div>
                <div className="text-xs text-muted-foreground">{platform ? `${platform} · ` : ''}{person.watch ? 'izleme açık' : 'izleme kapalı (Ayarlar sekmesi)'}</div>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
            <label className={cn('flex items-center gap-2.5 rounded-lg border px-3 py-2', person.full_auto ? 'border-gold/60 bg-gold/10' : 'bg-raised/50')}>
              <Zap className={cn('size-4', person.full_auto ? 'text-gold' : 'text-muted-foreground')} />
              <span className="text-sm font-medium">Tam otonom</span>
              <Switch checked={!!person.full_auto} onCheckedChange={toggleFull} aria-label="Tam otonom mod" />
            </label>
            <label className="flex items-center gap-2.5 rounded-lg border bg-raised/50 px-3 py-2">
              <Bot className={cn('size-4', person.auto_send ? 'text-gold' : 'text-muted-foreground')} />
              <span className="text-sm font-medium">Otomatik</span>
              <Switch checked={person.auto_send} onCheckedChange={toggleAuto} aria-label="Otomatik gönderim" />
            </label>
            </div>
          </div>
          <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Stat label="Bugün çağrı" value={state?.stats.calls_today ?? 0} />
            <Stat label="Son 1 saat gönderim" value={state?.stats.sent_last_hour ?? 0} sub={person.guards.max_per_hour ? `sınır ${person.guards.max_per_hour}/sa` : 'sınırsız'} />
            <Stat label="Onaylanan" value={`${person.approvals_done}/${person.guards.first_n_approvals}`} sub="ilk onaylar" tone="gold" />
            <Stat label="Otomatik" value={person.full_auto ? 'Tam otonom' : person.auto_send ? 'Açık' : 'Kapalı'} tone={person.auto_send ? 'gold' : undefined} />
          </div>
        </Card>

        {/* Ton: Sohbet sekmesindekiyle aynı ayar; değişince sonraki öneriler (otomatik dahil) buna göre yazılır */}
        <Card className="p-0">
          <button type="button" onClick={() => setToneOpen((v) => !v)} aria-expanded={toneOpen}
            className="flex w-full items-center gap-2 px-4 py-2.5 text-left text-sm">
            <SlidersHorizontal className="size-4 text-muted-foreground" />
            <span className="font-medium">Ton</span>
            <span className="text-muted-foreground">· {PRESETS.find((p) => p.id === person.tone.preset)?.label ?? person.tone.preset}</span>
            <span className="ml-2 hidden text-xs text-muted-foreground sm:inline">
              flört {person.tone.flort} · cesaret {person.tone.cesaret} · mizah {person.tone.mizah} · samimiyet {person.tone.samimiyet}
            </span>
            <ChevronDown className={cn('ml-auto size-4 text-muted-foreground transition-transform', toneOpen && 'rotate-180')} />
          </button>
          {toneOpen && <div className="border-t p-3"><ToneBar person={person} /></div>}
        </Card>

        {!live.connected && (
          <Empty icon={<Radio className="size-6" />}>Chrome uzantısı bağlı değil. WhatsApp Web ya da Instagram sekmesini aç; bağlantı kurulunca sohbet burada belirir.</Empty>
        )}
        {live.connected && !chat && <Empty icon={<MessageCircle className="size-6" />}>Henüz açık bir sohbet yok. Tarayıcıda bir sohbeti aç.</Empty>}

        {other && (
          <div className="flex flex-wrap items-center gap-2 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">
            Açık sohbet başka bir kişiye ait ({otherName ?? other}).
            <a className="inline-flex items-center gap-1 underline" href={personHref(other, 'live')}>Oraya git <ExternalLink className="size-3" /></a>
          </div>
        )}

        {mine && chat && (
          <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
            <Card className="flex min-h-0 flex-col p-0">
              <div className="border-b px-4 py-2.5 text-sm font-semibold">{chat.partner} · son mesajlar</div>
              <div ref={scroller} className="max-h-96 min-h-48 space-y-1.5 overflow-y-auto p-3">
                {chat.history.map((m, i) => {
                  const last = i === chat.history.length - 1 && m.from === 'them'
                  return (
                    <div key={i} className={cn('flex', m.from === 'me' ? 'justify-end' : 'justify-start')}>
                      <div className={cn('max-w-[85%] rounded-2xl px-3 py-1.5 text-sm break-words whitespace-pre-wrap', m.from === 'me' ? 'rounded-br-sm bg-primary/90 text-primary-foreground' : 'rounded-bl-sm bg-raised', last && 'ring-1 ring-gold/60')}>
                        {m.text}
                        <div className="mt-0.5 text-right text-[10px] opacity-60">{clock(m.ts)}{m.kind ? ` · ${m.kind}` : ''}</div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </Card>

            <div className="space-y-3">
              {pending && recCand && (
                <div className="rounded-xl border border-gold/50 bg-gold/10 p-3">
                  <div className="mb-2 flex items-center gap-2 text-sm font-semibold text-gold"><BellRing className="size-4" /> Onay bekliyor</div>
                  <p className="mb-2 text-xs text-muted-foreground">Otomatik mod bu cevabı hazırladı ama senin onayını bekliyor.</p>
                  <StepBubbles steps={recCand.steps} />
                  <div className="mt-3 flex gap-2">
                    <Button size="sm" disabled={recCand.blocked || state?.paused} onClick={() => void sendToLive(recCand.steps, recCand, 'approve')}><CheckCheck /> Onayla</Button>
                    <Button size="sm" variant="ghost" onClick={() => setSugg(mySugg ? { ...mySugg, pending_approval: false } : null)}>Kendim seçeyim</Button>
                  </div>
                </div>
              )}
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-sm font-semibold">Öneriler</h3>
                <Button size="sm" variant="secondary" disabled={busy} onClick={() => void suggestNow()}>
                  {busy ? <Spinner className="size-3.5" /> : <Sparkles />} Şimdi öner
                </Button>
              </div>
              {mySugg ? (
                <CandidateList result={mySugg} actions={sendActions} onDismiss={dismissCand} />
              ) : (
                <Empty>Yeni mesaj gelince öneriler burada belirir. İstersen “Şimdi öner”e bas.</Empty>
              )}
            </div>
          </div>
        )}

        {sentLog.length > 0 && (
          <Card>
            <SectionTitle>Bu oturumda gönderilenler</SectionTitle>
            <ul className="space-y-1.5 text-sm">
              {sentLog.map((s, i) => (
                <li key={i} className="flex items-start gap-2">
                  <span className="mt-0.5 w-12 shrink-0 text-xs text-muted-foreground">{clock(s.ts)}</span>
                  <span className="min-w-0 flex-1 break-words">{s.text}</span>
                  {s.auto && <span className="rounded-full bg-gold/15 px-1.5 text-[10px] text-gold">oto</span>}
                </li>
              ))}
            </ul>
          </Card>
        )}
      </div>

      <ConfirmDialog
        open={confirmFull}
        onOpenChange={setConfirmFull}
        title="Tam otonom modu aç"
        description={`${person.name} yazar yazmaz Barney, senin onayını beklemeden ~1-2 saniye içinde cevap gönderir. Gönderilen mesaj geri alınamaz.`}
        confirmLabel="Tam otonomu aç"
        confirmDisabled={!understoodFull}
        onConfirm={async () => { await patchPerson(person.id, { full_auto: true }) }}
      >
        <div className="space-y-3">
          <ul className="space-y-1.5 text-sm">
            <li className="flex gap-2"><ShieldCheck className="mt-0.5 size-4 shrink-0 text-warning" />İlk onaylar ve sessiz saat atlanır; cevap hemen gider.</li>
            <li className="flex gap-2"><ShieldCheck className="mt-0.5 size-4 shrink-0 text-success" />Hassas konu algılanırsa yine durur ve sana sorar.</li>
            <li className="flex gap-2"><ShieldCheck className="mt-0.5 size-4 shrink-0 text-success" />Saatlik/günlük sınır ve “Durdur” düğmesi geçerli kalır.</li>
          </ul>
          <label className="flex items-start gap-2 text-sm">
            <input type="checkbox" className="mt-1" checked={understoodFull} onChange={(e) => setUnderstoodFull(e.target.checked)} />
            <span>Riskleri anladım; tam otonom modu onaylıyorum.</span>
          </label>
        </div>
      </ConfirmDialog>

      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title="Otomatik gönderimi aç"
        description="Barney, sen onaylamadan senin adına mesaj gönderebilir. Yanlış anlaşılma, hatalı ton veya kaçırılmış bir bağlam gönderildiği anda geri alınamaz."
        confirmLabel="Otomatiği aç"
        confirmDisabled={!understood}
        onConfirm={async () => { await patchPerson(person.id, { auto_send: true }) }}
      >
        <div className="space-y-3">
          <div className="text-xs font-medium text-muted-foreground">Korumalar (Ayarlar sekmesinden değişir)</div>
          <GuardList p={person} />
          <label className="flex items-start gap-2 text-sm">
            <input type="checkbox" className="mt-1" checked={understood} onChange={(e) => setUnderstood(e.target.checked)} />
            <span>Riskleri anladım; otomatik gönderimin açılmasını onaylıyorum.</span>
          </label>
          {!understood && <p className="text-xs text-warning">Devam etmek için kutuyu işaretle.</p>}
        </div>
      </ConfirmDialog>
    </div>
  )
}
