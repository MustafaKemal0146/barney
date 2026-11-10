import { useState } from 'react'
import { Check, ClipboardCopy, Clock, Pencil, ShieldAlert, Star, Trash2, TriangleAlert } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { TURS } from '@/lib/fmt'
import type { Cand, Step, SuggestResult } from '@/lib/types'

const TUR_COLOR: Record<string, string> = {
  tek: 'bg-muted text-foreground',
  guvenli: 'bg-success/15 text-success',
  komik: 'bg-gold/15 text-gold',
  cesur: 'bg-destructive/15 text-destructive',
}

export const waitLabel = (s: number) => `⏱ ${Math.round(s)} sn`

export function StepBubbles({ steps, align = 'left' }: { steps: Step[]; align?: 'left' | 'right' }) {
  return (
    <div className={cn('flex flex-col gap-1.5', align === 'right' ? 'items-end' : 'items-start')}>
      {steps.map((s, i) => (
        <div key={i} className={cn('flex max-w-full flex-col gap-0.5', align === 'right' ? 'items-end' : 'items-start')}>
          <span className="flex items-center gap-1 text-[11px] text-muted-foreground" title="Bu mesajdan önce beklenecek süre">{waitLabel(s.wait_sec)}</span>
          <div className="max-w-full rounded-2xl rounded-tl-sm bg-raised px-3 py-1.5 text-sm break-words whitespace-pre-wrap">{s.text}</div>
        </div>
      ))}
    </div>
  )
}

export interface ActionSpec {
  label: string
  primary?: boolean
  disabled?: boolean
  disabledReason?: string
  onClick: (steps: Step[], cand: Cand) => void | Promise<void>
}

function ActionButton({ a, steps, cand }: { a: ActionSpec; steps: Step[]; cand: Cand }) {
  const [busy, setBusy] = useState(false)
  const btn = (
    <Button
      size="sm"
      variant={a.primary ? 'default' : 'outline'}
      disabled={a.disabled || busy}
      onClick={async () => {
        setBusy(true)
        try { await a.onClick(steps, cand) } finally { setBusy(false) }
      }}
    >
      {a.label}
    </Button>
  )
  if (a.disabled && a.disabledReason) {
    return (
      <Tooltip>
        <TooltipTrigger render={<span tabIndex={0} className="inline-flex" />}>{btn}</TooltipTrigger>
        <TooltipContent>{a.disabledReason}</TooltipContent>
      </Tooltip>
    )
  }
  return btn
}

export function CandidateCard({ cand, recommended, actions, onDismiss }: { cand: Cand; recommended?: boolean; actions: ActionSpec[]; onDismiss?: () => void }) {
  const [editing, setEditing] = useState(false)
  const [steps, setSteps] = useState<Step[]>(cand.steps)
  const [copied, setCopied] = useState(false)
  const blocked = cand.blocked
  const copy = async () => {
    try { await navigator.clipboard.writeText(steps.map((s) => s.text).join('\n')) } catch { /* yoksay */ }
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <div className={cn('rise rounded-xl border bg-card p-3', recommended && 'border-primary/60 ring-1 ring-primary/30', blocked && 'border-destructive/40')}>
      <div className="mb-2 flex flex-wrap items-center gap-1.5">
        <Badge className={cn('border-0', TUR_COLOR[cand.tur])}>{TURS[cand.tur] ?? cand.tur}</Badge>
        {recommended && <Badge variant="outline" className="border-primary/50 text-primary"><Star className="fill-current" /> Önerilen</Badge>}
        {cand.confident ? (
          <span className="inline-flex items-center gap-1 text-xs text-success" title="Stil denetimini geçti"><Check className="size-3.5" /> emin</span>
        ) : (
          <span className="inline-flex items-center gap-1 text-xs text-warning" title="Barney bu adaydan emin değil"><TriangleAlert className="size-3.5" /> emin değil</span>
        )}
        {steps.length > 1 && <span className="text-xs text-muted-foreground">{steps.length} mesaj</span>}
      </div>

      {editing ? (
        <div className="space-y-2">
          {steps.map((s, i) => (
            <div key={i} className="flex items-start gap-2">
              <span className="mt-2 w-14 shrink-0 text-[11px] text-muted-foreground">{waitLabel(s.wait_sec)}</span>
              <Textarea
                aria-label={`Mesaj ${i + 1}`}
                className="min-h-9 flex-1"
                rows={1}
                value={s.text}
                onChange={(e) => setSteps(steps.map((x, j) => (j === i ? { ...x, text: e.target.value } : x)))}
              />
            </div>
          ))}
          <div className="flex gap-2">
            <Button size="sm" onClick={() => { setSteps(steps.filter((s) => s.text.trim())); setEditing(false) }}>Bitti</Button>
            <Button size="sm" variant="ghost" onClick={() => { setSteps(cand.steps); setEditing(false) }}>Vazgeç</Button>
          </div>
        </div>
      ) : (
        <StepBubbles steps={steps} />
      )}

      {(cand.sensitive || blocked) && (
        <div className="mt-2 flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 px-2.5 py-1.5 text-xs text-destructive">
          <ShieldAlert className="mt-0.5 size-3.5 shrink-0" />
          <span>
            {blocked ? 'Engellendi: bu aday gönderilmemeli.' : ''} {cand.sensitive ? 'Hassas konu — göndermeden önce kendin oku.' : ''}
          </span>
        </div>
      )}
      {cand.violations.length > 0 && (
        <ul className="mt-2 space-y-0.5 text-xs text-warning">
          {cand.violations.map((v, i) => <li key={i} className="flex gap-1.5"><span aria-hidden>•</span>{v}</li>)}
        </ul>
      )}

      {!editing && !blocked && [...new Set(actions.filter((a) => a.disabled && a.disabledReason).map((a) => a.disabledReason))].map((r) => (
        <p key={r} className="mt-2 flex items-start gap-1.5 text-xs text-warning"><TriangleAlert className="mt-0.5 size-3.5 shrink-0" />{r}</p>
      ))}
      {!editing && (
        <div className="mt-3 flex flex-wrap items-center gap-2">
          {actions.map((a) => (
            <ActionButton
              key={a.label}
              a={blocked ? { ...a, disabled: true, disabledReason: 'Bu aday engellendi; önce düzenle ya da başka aday seç.' } : a}
              steps={steps}
              cand={cand}
            />
          ))}
          <Button size="sm" variant="ghost" onClick={() => setEditing(true)}><Pencil /> Düzenle</Button>
          <Button size="icon-sm" variant="ghost" onClick={copy} aria-label="Metni kopyala" title="Kopyala">
            {copied ? <Check className="text-success" /> : <ClipboardCopy />}
          </Button>
          {onDismiss && (
            <Button size="icon-sm" variant="ghost" onClick={onDismiss} aria-label="Öneriyi sil" title="Öneriyi sil" className="text-muted-foreground hover:text-destructive">
              <Trash2 />
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

export function CandidateList({ result, actions, onDismiss }: { result: SuggestResult; actions: ActionSpec[]; onDismiss?: (index: number) => void }) {
  return (
    <div className="space-y-3">
      {result.note && (
        <div className="flex items-start gap-2 text-xs text-muted-foreground"><Clock className="mt-0.5 size-3.5 shrink-0" /><span>{result.note}</span></div>
      )}
      <div className="grid gap-3 lg:grid-cols-2">
        {result.cands.map((c, i) => (
          <CandidateCard key={i + c.tur} cand={c} recommended={i === result.recommended} actions={actions} onDismiss={onDismiss ? () => onDismiss(i) : undefined} />
        ))}
      </div>
    </div>
  )
}
