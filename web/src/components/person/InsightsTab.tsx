import { useEffect, useState } from 'react'
import { Card, Loading, SectionTitle, Stat } from '@/components/kit'
import { C_OWNER, C_PARTNER, Columns, Lines, ShareBar } from '@/components/charts'
import { api } from '@/lib/api'
import { dateTime, num, secs } from '@/lib/fmt'
import type { Insights, Person } from '@/lib/types'

/** Varsayım: by_weekday[0] = Pazartesi (API.md belirtmiyor). */
export const WEEKDAYS = ['Pzt', 'Sal', 'Çar', 'Per', 'Cum', 'Cmt', 'Paz']

export function InsightsTab({ person }: { person: Person }) {
  const [data, setData] = useState<Insights | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    let live = true
    setData(null); setErr('')
    api.insights(person.id).then((d) => live && setData(d)).catch((e) => live && setErr(e instanceof Error ? e.message : String(e)))
    return () => { live = false }
  }, [person.id])
  if (err) return <div className="p-6 text-sm text-destructive">{err}</div>
  if (!data) return <Loading />
  const hours = Array.from({ length: 24 }, (_, i) => String(i))
  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-5xl space-y-4 p-3 sm:p-4">
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <Stat label="Toplam mesaj" value={num(data.total)} />
          <Stat label="Konuşmayı sen başlatırsın" value={`%${Math.round(data.initiator_owner_pct)}`} sub={`${person.name} %${Math.round(100 - data.initiator_owner_pct)}`} tone="primary" />
          <Stat label="Sessizlik" value={`${data.silence_days} gün`} sub={data.last_ts ? `son: ${dateTime(data.last_ts)}` : undefined} tone={data.silence_days >= (person.silence_days || 99) ? 'gold' : undefined} />
          <Stat label="Medyan cevap süresi" value={`${secs(data.latency_owner_median_sec)} · ${secs(data.latency_partner_median_sec)}`} sub="sen · o" />
        </div>
        <Card>
          <SectionTitle>Mesaj payı</SectionTitle>
          <ShareBar a={data.owner_share_pct} b={data.partner_share_pct} labelA={person.owner || 'Sen'} labelB={person.name} />
        </Card>
        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <SectionTitle hint="Günün hangi saatinde yazışıyorsunuz?">Saate göre</SectionTitle>
            <Columns values={data.by_hour} labels={hours} labelEvery={3} />
          </Card>
          <Card>
            <SectionTitle hint="Haftanın hangi günü?">Güne göre</SectionTitle>
            <Columns values={data.by_weekday} labels={WEEKDAYS} color={C_PARTNER} />
          </Card>
        </div>
        <Card>
          <SectionTitle hint="Haftalık mesaj sayısı">Zaman içinde</SectionTitle>
          <Lines
            labels={data.weekly.map((w) => w.week)}
            series={[
              { name: person.owner || 'Sen', color: C_OWNER, values: data.weekly.map((w) => w.owner) },
              { name: person.name, color: C_PARTNER, values: data.weekly.map((w) => w.partner) },
            ]}
          />
        </Card>
      </div>
    </div>
  )
}
