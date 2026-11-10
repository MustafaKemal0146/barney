import { useState } from 'react'
import { AlertTriangle, Bot, CheckCircle2, Loader2, Square, XCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { isActive, optsFrom, STATE_LABEL } from '@/lib/importJob'
import { num } from '@/lib/fmt'
import type { ImportJob } from '@/lib/types'
import { useApp } from '@/store/app'

export const AI_NOTICE =
  'Yapay zekâ WhatsApp sekmesini sizin için gezer (yalnız kaydırır/tıklar/okur, asla mesaj göndermez). Sayfa içeriği (isimler ve mesajlar) seçtiğiniz model sağlayıcısına gider; maskeleme yapılmaz.'

/** AI modu bilgi kutusu + zorunlu 'Anladım' onayı (onay oturumluk, store'da durur). */
export function AiConsent() {
  const consent = useApp((s) => s.aiConsent)
  const set = useApp((s) => s.setAiConsent)
  return (
    <div className="space-y-2.5">
      <div className="flex gap-2 rounded-lg border border-primary/30 bg-primary/10 p-3 text-xs">
        <Bot className="mt-0.5 size-4 shrink-0 text-primary" />
        <p>{AI_NOTICE}</p>
      </div>
      <label className="flex cursor-pointer items-center gap-2 text-sm">
        <input type="checkbox" checked={consent} onChange={(e) => set(e.target.checked)} className="size-4 accent-[var(--primary)]" />
        Anladım
      </label>
    </div>
  )
}

const BADGE: Record<ImportJob['state'], string> = {
  queued: 'bg-muted text-muted-foreground',
  running: 'bg-primary/15 text-primary',
  done: 'bg-success/15 text-success',
  failed: 'bg-destructive/15 text-destructive',
  canceled: 'bg-warning/15 text-warning',
}

function StateBadge({ job }: { job: ImportJob }) {
  const Icon = job.state === 'done' ? CheckCircle2 : job.state === 'failed' ? XCircle : job.state === 'canceled' ? AlertTriangle : Loader2
  return (
    <span data-testid="import-state" className={cn('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium', BADGE[job.state])}>
      <Icon className={cn('size-3.5', isActive(job) && 'animate-spin')} />
      {STATE_LABEL[job.state]}
    </span>
  )
}

/** İlerleme kartı: durum, okunan mesaj, sohbet i/n, günlük (son 8), Durdur ve AI'a geçiş. */
export function ImportProgress({ job, onDismiss }: { job: ImportJob; onDismiss?: () => void }) {
  const cancel = useApp((s) => s.cancelImport)
  const state = useApp((s) => s.state)
  const consent = useApp((s) => s.aiConsent)
  const importCurrent = useApp((s) => s.importCurrent)
  const toast = useApp((s) => s.showToast)
  const [busy, setBusy] = useState(false)
  const active = isActive(job)
  const bulk = job.kind === 'bulk'
  const log = (job.log ?? []).slice(-8)
  const fallback = job.state === 'failed' && job.can_fallback_ai && job.kind === 'current'

  const goAi = async () => {
    setBusy(true)
    try {
      await importCurrent(optsFrom(state, 'ai'))
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), true)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-3" data-testid="import-progress">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <StateBadge job={job} />
        <span className="text-xs text-muted-foreground">
          {job.kind === 'sync' ? 'Senkronizasyon' : bulk ? 'Toplu ekleme' : 'Tek sohbet'}
          {job.mode === 'ai' ? ' · AI' : ''}
        </span>
        <span className="ml-auto text-sm font-semibold tabular-nums">{num(job.loaded)} <span className="text-xs font-normal text-muted-foreground">mesaj</span></span>
      </div>
      {(job.chat || bulk) && (
        <div className="flex items-center justify-between gap-2 text-sm">
          <span className="min-w-0 truncate">{job.chat ? <>Sohbet: <b>{job.chat}</b></> : null}</span>
          {bulk && job.n > 0 && <span className="shrink-0 text-xs tabular-nums text-muted-foreground">chat {job.i}/{job.n}</span>}
        </div>
      )}
      {bulk && job.n > 0 && (
        <div className="h-1.5 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuemin={0} aria-valuemax={job.n} aria-valuenow={Math.min(job.n, job.state === 'done' ? job.n : job.i - 1)}>
          <div className="h-full bg-primary transition-all" style={{ width: `${(Math.min(job.n, job.state === 'done' ? job.n : job.i - 1) / job.n) * 100}%` }} />
        </div>
      )}
      {job.note && <p className="text-xs text-muted-foreground">{job.note}</p>}
      {log.length > 0 && (
        <ul className="space-y-0.5 rounded-lg border bg-raised/60 p-2 font-mono text-[11px] leading-relaxed text-muted-foreground" aria-label="Okuma günlüğü">
          {log.map((l, i) => <li key={i} className={cn('break-words', i === log.length - 1 && 'text-foreground')}>{l}</li>)}
        </ul>
      )}
      {job.state === 'failed' && (
        <p className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{job.error || 'Okuma başarısız oldu.'}</p>
      )}
      {active && (
        <>
          <p className="text-xs text-muted-foreground">WhatsApp sekmesi açık ve görünür kalmalı.</p>
          <Button variant="destructive" onClick={() => void cancel()}><Square /> Durdur</Button>
        </>
      )}
      {fallback && (
        <div className="space-y-2.5 rounded-lg border border-primary/40 bg-primary/5 p-3">
          <p className="text-xs">Normal okuyucu çalışmadı. İstersen yapay zekâ sekmeyi gezerek devam edebilir.</p>
          <AiConsent />
          <Button size="lg" disabled={!consent || busy} onClick={() => void goAi()}>
            <Bot /> AI ile devam et
          </Button>
        </div>
      )}
      {!active && onDismiss && job.state !== 'done' && (
        <div><Button variant="outline" onClick={onDismiss}>Geri</Button></div>
      )}
    </div>
  )
}

/** Ana alanın üstünde, iş sürerken görünen ince şerit (gömülü kipte de). */
export function ImportBar() {
  const job = useApp((s) => s.importJob)
  const cancel = useApp((s) => s.cancelImport)
  const openAdd = useApp((s) => s.openAdd)
  if (!job || !isActive(job)) return null
  return (
    <div role="status" data-testid="import-bar" className="flex shrink-0 items-center gap-2 border-b bg-primary/10 px-3 py-1.5 text-xs">
      <Loader2 className="size-3.5 shrink-0 animate-spin text-primary" />
      <button type="button" onClick={() => openAdd()} className="min-w-0 flex-1 truncate text-left hover:underline" title="Ayrıntıyı göster">
        <b>{job.kind === 'sync' ? 'Senkronize ediliyor' : 'Okunuyor'}</b>
        {job.chat ? `: ${job.chat}` : ''} · {num(job.loaded)} mesaj{job.kind === 'bulk' && job.n > 0 ? ` · ${job.i}/${job.n}` : ''}
        <span className="hidden text-muted-foreground sm:inline"> · WhatsApp sekmesi açık ve görünür kalmalı</span>
      </button>
      <Button size="xs" variant="destructive" onClick={() => void cancel()}>Durdur</Button>
    </div>
  )
}
