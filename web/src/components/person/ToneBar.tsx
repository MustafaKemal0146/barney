import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'
import { Slider } from '@/components/ui/slider'
import { useDebouncedPatch } from '@/lib/useDebouncedPatch'
import type { Person, Tone, TonePreset } from '@/lib/types'

export const PRESETS: { id: TonePreset; label: string; values: Omit<Tone, 'preset'> }[] = [
  { id: 'benim', label: 'Benim gibi', values: { flort: 3, cesaret: 4, mizah: 5, samimiyet: 6 } },
  { id: 'barney', label: 'Özgüvenli', values: { flort: 7, cesaret: 8, mizah: 7, samimiyet: 6 } },
  { id: 'sade', label: 'Sade', values: { flort: 1, cesaret: 2, mizah: 2, samimiyet: 4 } },
  { id: 'samimi', label: 'Samimi', values: { flort: 3, cesaret: 3, mizah: 6, samimiyet: 9 } },
  { id: 'flort', label: 'Flört', values: { flort: 8, cesaret: 6, mizah: 6, samimiyet: 7 } },
]
const SLIDERS: { key: 'flort' | 'cesaret' | 'mizah' | 'samimiyet'; label: string }[] = [
  { key: 'flort', label: 'Flört seviyesi' },
  { key: 'cesaret', label: 'Cesaret' },
  { key: 'mizah', label: 'Mizah' },
  { key: 'samimiyet', label: 'Samimiyet' },
]

export function ToneBar({ person }: { person: Person }) {
  const [tone, setTone] = useState<Tone>(person.tone)
  const { schedule } = useDebouncedPatch(person.id)
  // başka kişiye geçince (ya da sunucudan yeni değer gelince kişi değişince) senkronla
  useEffect(() => setTone(person.tone), [person.id]) // eslint-disable-line react-hooks/exhaustive-deps

  const apply = (t: Tone) => {
    setTone(t)
    schedule({ tone: t })
  }
  return (
    <div className="space-y-3 rounded-xl border bg-card p-3" aria-label="Ton ayarı">
      <div className="flex flex-wrap gap-1.5">
        {PRESETS.map((p) => (
          <button
            key={p.id}
            type="button"
            aria-pressed={tone.preset === p.id}
            onClick={() => apply({ preset: p.id, ...p.values })}
            className={cn(
              'rounded-full border px-3 py-1 text-xs font-medium transition-colors',
              tone.preset === p.id ? 'border-primary bg-primary text-primary-foreground' : 'text-muted-foreground hover:border-primary/60 hover:text-foreground',
            )}
          >
            {p.label}
          </button>
        ))}
      </div>
      <div className="grid gap-x-6 gap-y-2.5 sm:grid-cols-2">
        {SLIDERS.map((s) => (
          <label key={s.key} className="block">
            <span className="mb-1 flex justify-between text-xs">
              <span className="text-muted-foreground">{s.label}</span>
              <b className="tabular-nums text-gold">{tone[s.key]}</b>
            </span>
            <Slider label={s.label} value={tone[s.key]} onChange={(v) => apply({ ...tone, [s.key]: v })} />
          </label>
        ))}
      </div>
    </div>
  )
}
