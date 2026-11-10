import { cn } from '@/lib/utils'

export const C_OWNER = 'var(--primary)'
export const C_PARTNER = 'var(--gold)'

/** İki taraflı yüzde çubuğu. */
export function ShareBar({ a, b, labelA, labelB }: { a: number; b: number; labelA: string; labelB: string }) {
  const total = a + b || 1
  const pa = (a / total) * 100
  return (
    <div>
      <div className="flex h-3 overflow-hidden rounded-full bg-border" role="img" aria-label={`${labelA} %${Math.round(pa)}, ${labelB} %${Math.round(100 - pa)}`}>
        <div style={{ width: `${pa}%`, background: C_OWNER }} />
        <div style={{ width: `${100 - pa}%`, background: C_PARTNER }} />
      </div>
      <div className="mt-1.5 flex justify-between text-xs">
        <span className="flex items-center gap-1.5"><i className="size-2 rounded-full" style={{ background: C_OWNER }} />{labelA} <b className="tabular-nums">%{Math.round(pa)}</b></span>
        <span className="flex items-center gap-1.5">{labelB} <b className="tabular-nums">%{Math.round(100 - pa)}</b><i className="size-2 rounded-full" style={{ background: C_PARTNER }} /></span>
      </div>
    </div>
  )
}

/** Sahip / karşı taraf yan yana yatay çubuk satırları (Stil sekmesi). */
export function CompareRow({ label, owner, partner, max, fmt = (n: number) => String(Math.round(n)), hint }: {
  label: string; owner: number; partner: number; max?: number; fmt?: (n: number) => string; hint?: string
}) {
  const m = max ?? Math.max(owner, partner, 1)
  return (
    <div className="space-y-1" title={hint}>
      <div className="text-xs text-muted-foreground">{label}</div>
      {[{ v: owner, c: C_OWNER, k: 'Sen' }, { v: partner, c: C_PARTNER, k: 'O' }].map((r) => (
        <div key={r.k} className="flex items-center gap-2">
          <span className="w-6 shrink-0 text-[10px] text-muted-foreground">{r.k}</span>
          <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-border">
            <div className="h-full rounded-full" style={{ width: `${Math.min(100, (r.v / m) * 100)}%`, background: r.c }} />
          </div>
          <span className="w-14 shrink-0 text-right text-xs tabular-nums">{fmt(r.v)}</span>
        </div>
      ))}
    </div>
  )
}

/** Dikey sütun grafiği (24 saat / hafta günü). */
export function Columns({ values, labels, color = C_OWNER, labelEvery = 1, height = 120 }: {
  values: number[]; labels: string[]; color?: string; labelEvery?: number; height?: number
}) {
  const W = 480, padB = 18, padT = 6
  const n = values.length
  const bw = W / n
  const max = Math.max(...values, 1)
  const peak = values.indexOf(Math.max(...values))
  return (
    <svg viewBox={`0 0 ${W} ${height}`} className="w-full" role="img" aria-label="Sütun grafiği">
      {values.map((v, i) => {
        const h = ((height - padB - padT) * v) / max
        return (
          <g key={i}>
            <rect x={i * bw + bw * 0.14} y={height - padB - h} width={bw * 0.72} height={Math.max(h, 1)} rx={3} fill={color} opacity={i === peak ? 1 : 0.55}>
              <title>{`${labels[i]}: ${v}`}</title>
            </rect>
            {i % labelEvery === 0 && (
              <text x={i * bw + bw / 2} y={height - 4} textAnchor="middle" fontSize="10" fill="var(--muted-foreground)">{labels[i]}</text>
            )}
          </g>
        )
      })}
    </svg>
  )
}

/** Çok çizgili haftalık grafik. */
export function Lines({ series, labels, height = 150 }: {
  series: { name: string; color: string; values: number[] }[]; labels: string[]; height?: number
}) {
  const W = 480, padL = 28, padB = 18, padT = 8, padR = 6
  const n = labels.length
  const max = Math.max(...series.flatMap((s) => s.values), 1)
  const x = (i: number) => padL + (n <= 1 ? 0 : (i * (W - padL - padR)) / (n - 1))
  const y = (v: number) => padT + (height - padT - padB) * (1 - v / max)
  const step = Math.max(1, Math.ceil(n / 6))
  return (
    <div>
      <svg viewBox={`0 0 ${W} ${height}`} className="w-full" role="img" aria-label="Haftalık mesaj sayısı">
        {[0, 0.5, 1].map((f) => (
          <g key={f}>
            <line x1={padL} x2={W - padR} y1={y(max * f)} y2={y(max * f)} stroke="var(--border)" strokeDasharray="3 4" />
            <text x={padL - 4} y={y(max * f) + 3} textAnchor="end" fontSize="9" fill="var(--muted-foreground)">{Math.round(max * f)}</text>
          </g>
        ))}
        {series.map((s) => (
          <g key={s.name}>
            <polyline fill="none" stroke={s.color} strokeWidth={2} strokeLinejoin="round" points={s.values.map((v, i) => `${x(i)},${y(v)}`).join(' ')} />
            {s.values.map((v, i) => <circle key={i} cx={x(i)} cy={y(v)} r={2.5} fill={s.color}><title>{`${labels[i]} · ${s.name}: ${v}`}</title></circle>)}
          </g>
        ))}
        {labels.map((l, i) => i % step === 0 && (
          <text key={i} x={x(i)} y={height - 4} textAnchor="middle" fontSize="9" fill="var(--muted-foreground)">{l.replace(/^\d{4}-/, '')}</text>
        ))}
      </svg>
      <div className={cn('mt-1 flex gap-4 text-xs')}>
        {series.map((s) => <span key={s.name} className="flex items-center gap-1.5"><i className="size-2 rounded-full" style={{ background: s.color }} />{s.name}</span>)}
      </div>
    </div>
  )
}
