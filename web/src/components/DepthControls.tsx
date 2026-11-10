import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Field } from '@/components/kit'
import { DEFAULT_HISTORY } from '@/lib/importJob'
import type { HistoryPrefs } from '@/lib/types'

const PRESETS: { label: string; v: HistoryPrefs | ((c: HistoryPrefs) => HistoryPrefs); on: (c: HistoryPrefs) => boolean }[] = [
  { label: '500', v: (c) => ({ ...c, max_messages: 500, all: false }), on: (c) => !c.all && c.max_messages === 500 },
  { label: '3000', v: (c) => ({ ...c, max_messages: 3000, all: false }), on: (c) => !c.all && c.max_messages === 3000 },
  { label: 'Tüm geçmiş', v: (c) => ({ ...c, all: true }), on: (c) => c.all },
]

/** Geçmiş derinliği: hazır seçenekler + özel en çok mesaj / en çok süre. */
export function DepthControls({ value, onChange }: { value: HistoryPrefs; onChange: (v: HistoryPrefs) => void }) {
  const num = (s: string, min: number, fb: number) => {
    const n = parseInt(s, 10)
    return Number.isFinite(n) ? Math.max(min, n) : fb
  }
  return (
    <div className="space-y-3" data-testid="depth-controls">
      <Field label="Geçmiş derinliği">
        <div className="flex flex-wrap gap-1.5" role="group" aria-label="Geçmiş derinliği">
          {PRESETS.map((p) => {
            const on = p.on(value)
            return (
              <button
                key={p.label}
                type="button"
                aria-pressed={on}
                onClick={() => onChange((p.v as (c: HistoryPrefs) => HistoryPrefs)(value))}
                className={cn('rounded-full border px-3 py-1 text-sm transition-colors', on ? 'border-primary bg-primary/15 font-medium text-primary' : 'text-muted-foreground hover:border-primary/60 hover:text-foreground')}
              >
                {p.label}
              </button>
            )
          })}
        </div>
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="En çok mesaj">
          <Input
            type="number" min={50} step={100} inputMode="numeric" aria-label="En çok mesaj" disabled={value.all}
            value={value.all ? '' : value.max_messages} placeholder={value.all ? 'sınırsız' : ''}
            onChange={(e) => onChange({ ...value, max_messages: num(e.target.value, 1, DEFAULT_HISTORY.max_messages) })}
          />
        </Field>
        <Field label="En çok süre (sn)">
          <Input
            type="number" min={10} step={30} inputMode="numeric" aria-label="En çok süre (saniye)"
            value={value.max_seconds}
            onChange={(e) => onChange({ ...value, max_seconds: num(e.target.value, 1, DEFAULT_HISTORY.max_seconds) })}
          />
        </Field>
      </div>
    </div>
  )
}
