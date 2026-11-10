import { useEffect, useState } from 'react'
import { RefreshCw, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Card, Empty, Field, Loading, NativeSelect, SectionTitle, Spinner } from '@/components/kit'
import { api } from '@/lib/api'
import { GOALS, STAGES } from '@/lib/fmt'
import { useApp } from '@/store/app'
import type { Goal, Memory, Person } from '@/lib/types'

export function MemoryTab({ person }: { person: Person }) {
  const [mem, setMem] = useState<Memory | null>(null)
  const [busy, setBusy] = useState(false)
  const [notes, setNotes] = useState(person.notes)
  const patch = useApp((s) => s.patchPerson)
  const toast = useApp((s) => s.showToast)
  const fail = (e: unknown) => toast(e instanceof Error ? e.message : String(e), true)

  useEffect(() => {
    let live = true
    setMem(null)
    setNotes(person.notes)
    api.memory(person.id).then((m) => live && setMem(m)).catch(fail)
    return () => { live = false }
  }, [person.id]) // eslint-disable-line react-hooks/exhaustive-deps

  const refresh = async () => {
    setBusy(true)
    try { setMem(await api.refreshMemory(person.id)) } catch (e) { fail(e) } finally { setBusy(false) }
  }
  const del = async (fid: string) => {
    try {
      await api.deleteFact(person.id, fid)
      setMem((m) => m && { ...m, facts: m.facts.filter((f) => f.id !== fid) })
    } catch (e) { fail(e) }
  }
  const stageOpts = STAGES.some((s) => s.value === person.stage) ? STAGES : [...STAGES, { value: person.stage, label: person.stage }]

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto grid max-w-5xl gap-4 p-3 sm:p-4 lg:grid-cols-[3fr_2fr]">
        <Card>
          <SectionTitle
            hint="Barney sohbetten çıkardığı kalıcı gerçekleri hatırlar. Yanlış olanı sil."
            right={<Button variant="outline" size="sm" disabled={busy} onClick={() => void refresh()}>{busy ? <Spinner className="size-3.5" /> : <RefreshCw />} Yenile</Button>}
          >
            Hafıza
          </SectionTitle>
          {!mem ? <Loading /> : mem.facts.length === 0 ? (
            <Empty>Henüz kayıtlı bir gerçek yok. “Yenile” ile son konuşmalardan çıkar.</Empty>
          ) : (
            <ul className="divide-y">
              {mem.facts.map((f) => (
                <li key={f.id} className="flex items-start gap-3 py-2.5">
                  <span className="min-w-0 flex-1 text-sm break-words">{f.text}</span>
                  <Button variant="ghost" size="icon-sm" aria-label="Gerçeği sil" title="Sil" onClick={() => void del(f.id)}><Trash2 /></Button>
                </li>
              ))}
            </ul>
          )}
        </Card>
        <div className="space-y-4">
          <Card className="space-y-4">
            <Field label="Hedef" hint="Önerilerin yönünü belirler.">
              <NativeSelect label="Hedef" className="w-full" value={person.goal} options={GOALS} onChange={(v) => void patch(person.id, { goal: v as Goal })} />
            </Field>
            <Field label="Aşama" hint="Barney hafızadan tahmin eder; sen de değiştirebilirsin.">
              <NativeSelect label="Aşama" className="w-full" value={person.stage} options={stageOpts} onChange={(v) => void patch(person.id, { stage: v })} />
            </Field>
            <Field label="Notlar" hint="Her öneride modele verilir. Hassas bilgi yazma.">
              <Textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={5} aria-label="Notlar" className="min-h-28" />
              <div className="flex justify-end">
                <Button size="sm" disabled={notes === person.notes} onClick={() => { void patch(person.id, { notes }); toast('Not kaydedildi') }}>Kaydet</Button>
              </div>
            </Field>
          </Card>
        </div>
      </div>
    </div>
  )
}
