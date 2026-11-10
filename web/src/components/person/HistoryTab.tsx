import { useEffect, useState } from 'react'
import { ArrowRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Empty, Loading } from '@/components/kit'
import { api } from '@/lib/api'
import { dateTime } from '@/lib/fmt'
import type { HistoryItem, HistoryKind, Person } from '@/lib/types'

const KINDS: Record<HistoryKind, { label: string; cls: string }> = {
  suggested: { label: 'önerildi', cls: 'bg-primary/15 text-primary' },
  sent: { label: 'gönderildi', cls: 'bg-success/15 text-success' },
  edited: { label: 'düzenlendi', cls: 'bg-gold/15 text-gold' },
  rejected: { label: 'reddedildi', cls: 'bg-destructive/15 text-destructive' },
  approved: { label: 'onaylandı', cls: 'bg-success/15 text-success' },
}

export function HistoryTab({ person }: { person: Person }) {
  const [items, setItems] = useState<HistoryItem[] | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    let live = true
    setItems(null); setErr('')
    api.history(person.id).then((h) => live && setItems([...h].sort((a, b) => b.ts - a.ts))).catch((e) => live && setErr(e instanceof Error ? e.message : String(e)))
    return () => { live = false }
  }, [person.id])
  if (err) return <div className="p-6 text-sm text-destructive">{err}</div>
  if (!items) return <Loading />
  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-3xl p-3 sm:p-4">
        {items.length === 0 ? <Empty>Henüz öneri geçmişi yok.</Empty> : (
          <ol className="relative space-y-4 border-l pl-5">
            {items.map((h, i) => {
              const k = KINDS[h.kind] ?? { label: h.kind, cls: 'bg-muted' }
              return (
                <li key={i} className="relative">
                  <span className="absolute -left-[25px] top-1.5 size-2 rounded-full bg-primary ring-4 ring-background" />
                  <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    <span className={cn('rounded-full px-2 py-0.5 font-medium', k.cls)}>{k.label}</span>
                    {dateTime(h.ts)}
                  </div>
                  <div className="mt-1 rounded-lg border bg-card px-3 py-2 text-sm break-words whitespace-pre-wrap">
                    {h.text}
                    {h.final && h.final !== h.text && (
                      <div className="mt-1.5 flex items-start gap-2 border-t pt-1.5 text-gold"><ArrowRight className="mt-0.5 size-3.5 shrink-0" />{h.final}</div>
                    )}
                  </div>
                </li>
              )
            })}
          </ol>
        )}
      </div>
    </div>
  )
}
