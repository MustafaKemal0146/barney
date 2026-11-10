import { useState, type ReactNode } from 'react'
import { Check, Copy, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'

export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn('rounded-xl border bg-card p-4', className)}>{children}</div>
}
export function SectionTitle({ children, hint, right }: { children: ReactNode; hint?: ReactNode; right?: ReactNode }) {
  return (
    <div className="mb-3 flex items-start justify-between gap-3">
      <div className="min-w-0">
        <h3 className="text-sm font-semibold">{children}</h3>
        {hint && <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>}
      </div>
      {right}
    </div>
  )
}
export function Stat({ label, value, sub, tone }: { label: string; value: ReactNode; sub?: ReactNode; tone?: 'gold' | 'primary' }) {
  return (
    <div className="rounded-lg border bg-raised/60 p-3">
      <div className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className={cn('mt-1 text-xl font-semibold tabular-nums', tone === 'gold' && 'text-gold', tone === 'primary' && 'text-primary')}>{value}</div>
      {sub && <div className="mt-0.5 text-xs text-muted-foreground">{sub}</div>}
    </div>
  )
}
export function Pill({ ok, warn, children, title, onClick }: { ok?: boolean; warn?: boolean; children: ReactNode; title?: string; onClick?: () => void }) {
  const cls = cn(
    'inline-flex h-6 items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium whitespace-nowrap',
    ok ? 'border-success/30 bg-success/10 text-success' : warn ? 'border-warning/30 bg-warning/10 text-warning' : 'border-destructive/30 bg-destructive/10 text-destructive',
    onClick && 'transition-opacity hover:opacity-80',
  )
  const dot = <span className={cn('size-1.5 rounded-full', ok ? 'bg-success' : warn ? 'bg-warning' : 'bg-destructive')} />
  return onClick ? (
    <button type="button" className={cls} title={title} onClick={onClick}>{dot}{children}</button>
  ) : (
    <span className={cls} title={title}>{dot}{children}</span>
  )
}
export function Empty({ children, icon }: { children: ReactNode; icon?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed px-4 py-10 text-center text-sm text-muted-foreground">
      {icon}
      <div>{children}</div>
    </div>
  )
}
export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cn('size-4 animate-spin text-muted-foreground', className)} />
}
export function Loading({ children = 'Yükleniyor…' }: { children?: ReactNode }) {
  return <div className="flex items-center gap-2 p-6 text-sm text-muted-foreground"><Spinner />{children}</div>
}
export function Field({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <div className="text-xs font-medium">{label}</div>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}
export function Row({ title, hint, children }: { title: ReactNode; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-2.5">
      <div className="min-w-0">
        <div className="text-sm font-medium">{title}</div>
        {hint && <div className="mt-0.5 text-xs text-muted-foreground">{hint}</div>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}

export function Segmented<T extends string>({ value, onChange, options, className }: {
  value: T; onChange: (v: T) => void; options: { value: T; label: string }[]; className?: string
}) {
  return (
    <div role="tablist" className={cn('inline-flex rounded-lg border bg-background p-0.5', className)}>
      {options.map((o) => (
        <button
          key={o.value}
          role="tab"
          aria-selected={value === o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={cn(
            'rounded-md px-3 py-1 text-sm font-medium transition-colors',
            value === o.value ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

export function CopyButton({ text, label = 'Kopyala' }: { text: string; label?: string }) {
  const [done, setDone] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      const ta = document.createElement('textarea')
      ta.value = text
      document.body.appendChild(ta)
      ta.select()
      try { document.execCommand('copy') } catch { /* yoksay */ }
      ta.remove()
    }
    setDone(true)
    setTimeout(() => setDone(false), 1800)
  }
  return (
    <Button variant="outline" size="sm" onClick={copy} type="button">
      {done ? <Check className="text-success" /> : <Copy />}
      {done ? 'Kopyalandı' : label}
    </Button>
  )
}

export function ConfirmDialog({ open, onOpenChange, title, description, confirmLabel, destructive, confirmDisabled, onConfirm, children }: {
  open: boolean; onOpenChange: (v: boolean) => void; title: string; description?: ReactNode
  confirmLabel: string; destructive?: boolean; confirmDisabled?: boolean; onConfirm: () => void | Promise<void>; children?: ReactNode
}) {
  const [busy, setBusy] = useState(false)
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        {children}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Vazgeç</Button>
          <Button
            variant={destructive ? 'destructive' : 'default'}
            disabled={busy || confirmDisabled}
            onClick={async () => {
              setBusy(true)
              try { await onConfirm() } finally { setBusy(false) }
              onOpenChange(false)
            }}
          >
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function NativeSelect({ value, onChange, options, className, label, disabled }: {
  value: string; onChange: (v: string) => void; options: { value: string; label: string }[]; className?: string; label: string; disabled?: boolean
}) {
  return (
    <select
      aria-label={label}
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      className={cn('h-8 rounded-lg border border-input bg-background px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50', className)}
    >
      {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
    </select>
  )
}
