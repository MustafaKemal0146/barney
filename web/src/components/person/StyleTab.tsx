import { Card, SectionTitle, Stat } from '@/components/kit'
import { C_OWNER, C_PARTNER, CompareRow } from '@/components/charts'
import { num, pct, secs } from '@/lib/fmt'
import type { CardSummary, Person } from '@/lib/types'

function Burst({ label, c, color }: { label: string; c: CardSummary; color: string }) {
  const parts = [
    { k: 'Tek', v: c.burst.single, o: 1 },
    { k: '2–3', v: c.burst.two_three, o: 0.7 },
    { k: '4+', v: c.burst.four_plus, o: 0.45 },
  ]
  return (
    <div>
      <div className="mb-1 text-xs text-muted-foreground">{label}</div>
      <div className="flex h-5 overflow-hidden rounded-md" role="img" aria-label={parts.map((p) => `${p.k}: %${p.v}`).join(', ')}>
        {parts.map((p) => (
          <div key={p.k} style={{ width: `${p.v}%`, background: color, opacity: p.o }} className="flex items-center justify-center text-[10px] font-medium text-background" title={`${p.k} mesaj: %${p.v}`}>
            {p.v >= 12 ? `${p.k} %${Math.round(p.v)}` : ''}
          </div>
        ))}
      </div>
      <div className="mt-1 text-[11px] text-muted-foreground">medyan {c.burst.median} arka arkaya · en çok {c.burst.max}</div>
    </div>
  )
}

function Chips({ items }: { items?: string[] | null }) {
  if (!items?.length) return <span className="text-xs text-muted-foreground">—</span>
  return <div className="flex flex-wrap gap-1.5">{(items ?? []).map((s) => <span key={s} className="rounded-full border px-2 py-0.5 text-sm">{s}</span>)}</div>
}

export function StyleTab({ person }: { person: Person }) {
  const o = person.card.owner
  const p = person.card.partner
  const legend = (
    <div className="flex gap-3 text-xs">
      <span className="flex items-center gap-1.5"><i className="size-2 rounded-full" style={{ background: C_OWNER }} />{person.owner || 'Sen'}</span>
      <span className="flex items-center gap-1.5"><i className="size-2 rounded-full" style={{ background: C_PARTNER }} />{person.name}</span>
    </div>
  )
  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-5xl space-y-4 p-3 sm:p-4">
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <Stat label="Toplam mesaj" value={num(person.messages_total)} />
          <Stat label="Senin mesajın" value={num(o.messages)} tone="primary" />
          <Stat label={`${person.name} mesajı`} value={num(p.messages)} tone="gold" />
          <Stat label="Güven" value={person.confidence} sub={person.messages_total < 300 ? 'az veri' : undefined} />
        </div>
        <Card>
          <SectionTitle hint="Barney senin tarafını taklit eder; karşı tarafı bağlam olarak okur." right={legend}>Yazış tarzı karşılaştırması</SectionTitle>
          <div className="grid gap-x-8 gap-y-4 md:grid-cols-2">
            <CompareRow label="Mesaj başına kelime (medyan)" owner={o.words_median} partner={p.words_median} />
            <CompareRow label="Uzun mesaj (90. yüzdelik, kelime)" owner={o.words_p90} partner={p.words_p90} />
            <CompareRow label="Küçük harfle başlama" owner={o.lower_start_pct} partner={p.lower_start_pct} max={100} fmt={pct} />
            <CompareRow label="Hiç büyük harf kullanmama" owner={o.no_caps_pct} partner={p.no_caps_pct} max={100} fmt={pct} />
            <CompareRow label="Noktalamasız mesaj" owner={o.punct_none_pct} partner={p.punct_none_pct} max={100} fmt={pct} />
            <CompareRow label="Emojili mesaj" owner={o.emoji_pct} partner={p.emoji_pct} max={100} fmt={pct} />
            <CompareRow label="Cevap süresi (medyan)" owner={o.latency_median_sec} partner={p.latency_median_sec} fmt={secs} />
          </div>
        </Card>
        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <SectionTitle hint="Arka arkaya kaç mesaj atıyor?">Mesaj serileri</SectionTitle>
            <div className="space-y-3">
              <Burst label={person.owner || 'Sen'} c={o} color={C_OWNER} />
              <Burst label={person.name} c={p} color={C_PARTNER} />
            </div>
          </Card>
          <Card>
            <SectionTitle>İmza ifadeler ve emojiler</SectionTitle>
            <div className="space-y-3">
              <div><div className="mb-1 text-xs text-muted-foreground">Senin kelimelerin</div><Chips items={o.signature} /></div>
              <div><div className="mb-1 text-xs text-muted-foreground">Senin emojilerin</div><Chips items={o.top_emoji} /></div>
              <div><div className="mb-1 text-xs text-muted-foreground">{person.name} kelimeleri</div><Chips items={p.signature} /></div>
              <div><div className="mb-1 text-xs text-muted-foreground">{person.name} emojileri</div><Chips items={p.top_emoji} /></div>
            </div>
          </Card>
        </div>
      </div>
    </div>
  )
}
