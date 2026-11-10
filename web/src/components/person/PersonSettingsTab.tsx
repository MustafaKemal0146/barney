import { useEffect, useState } from 'react'
import { Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Card, ConfirmDialog, Row, SectionTitle } from '@/components/kit'
import { useDebouncedPatch } from '@/lib/useDebouncedPatch'
import { navigate } from '@/lib/route'
import { useApp } from '@/store/app'
import type { Guards, Person } from '@/lib/types'

function NumInput({ value, onChange, min = 0, max = 999, label }: { value: number; onChange: (n: number) => void; min?: number; max?: number; label: string }) {
  return (
    <Input type="number" inputMode="numeric" aria-label={label} className="w-24 text-right" min={min} max={max} value={value}
      onChange={(e) => { const n = Number(e.target.value); if (Number.isFinite(n)) onChange(Math.min(max, Math.max(min, Math.round(n)))) }} />
  )
}

// LimitInput sayı ya da "Sınırsız" (0) seçtirir.
function LimitInput({ value, onChange, max, label, fallback }: { value: number; onChange: (n: number) => void; max: number; label: string; fallback: number }) {
  const unlimited = value === 0
  return (
    <div className="flex items-center gap-3">
      {!unlimited && <NumInput label={label} value={value} min={1} max={max} onChange={onChange} />}
      <label className="flex items-center gap-2 text-sm text-muted-foreground">
        Sınırsız
        <Switch checked={unlimited} onCheckedChange={(v) => onChange(v ? 0 : fallback)} aria-label={`${label}: sınırsız`} />
      </label>
    </div>
  )
}

export function PersonSettingsTab({ person }: { person: Person }) {
  const patch = useApp((s) => s.patchPerson)
  const remove = useApp((s) => s.removePerson)
  const toast = useApp((s) => s.showToast)
  const [guards, setGuards] = useState<Guards>(person.guards)
  const [silence, setSilence] = useState(person.silence_days)
  const [del, setDel] = useState(false)
  const { schedule } = useDebouncedPatch(person.id)
  useEffect(() => { setGuards(person.guards); setSilence(person.silence_days) }, [person.id]) // eslint-disable-line react-hooks/exhaustive-deps

  const setG = (g: Partial<Guards>) => {
    const next = { ...guards, ...g }
    setGuards(next)
    schedule({ guards: next })
  }
  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-3xl space-y-4 p-3 sm:p-4">
        <Card className="divide-y py-2">
          <Row title="İzle" hint="Bu kişinin sohbeti açıkken gelen mesajlar için öneri hazırla."><Switch checked={person.watch} onCheckedChange={(v) => void patch(person.id, { watch: v })} aria-label="İzle" /></Row>
          <Row title="Konsey" hint="Birden fazla model görüşü birleştirilir; daha yavaş ve pahalı, daha tutarlı."><Switch checked={person.council} onCheckedChange={(v) => void patch(person.id, { council: v })} aria-label="Konsey" /></Row>
          <Row title="Sessizlik eşiği (gün)" hint="Bu kadar gün yazışılmazsa kenar çubuğunda hatırlatma çıkar. 0 = kapalı.">
            <NumInput label="Sessizlik eşiği" value={silence} max={365} onChange={(n) => { setSilence(n); schedule({ silence_days: n }) }} />
          </Row>
        </Card>

        <Card>
          <SectionTitle hint="Otomatik gönderim açıkken geçerli olan sınırlar.">Korumalar</SectionTitle>
          <div className="divide-y">
            <Row title="İlk N cevap onaylı" hint="Otomatik mod bu kadar cevabı senin onayına bırakır."><NumInput label="İlk N onay" value={guards.first_n_approvals} max={50} onChange={(n) => setG({ first_n_approvals: n })} /></Row>
            <Row title="Sessiz saatler" hint="Bu aralıkta hiçbir şey gönderilmez.">
              <Switch checked={guards.quiet_on} onCheckedChange={(v) => setG({ quiet_on: v })} aria-label="Sessiz saatler" />
            </Row>
            {guards.quiet_on && (
              <div className="flex items-center gap-2 py-2.5 text-sm">
                <Input type="time" aria-label="Sessiz saat başlangıç" className="w-28" value={guards.quiet_start} onChange={(e) => setG({ quiet_start: e.target.value })} />
                <span className="text-muted-foreground">–</span>
                <Input type="time" aria-label="Sessiz saat bitiş" className="w-28" value={guards.quiet_end} onChange={(e) => setG({ quiet_end: e.target.value })} />
              </div>
            )}
            <Row title="Saatte en fazla" hint="Otomatik cevap sayısı (art arda giden kısa mesajlar tek cevap sayılır)."><LimitInput label="Saatte en fazla" value={guards.max_per_hour} max={1000} fallback={50} onChange={(n) => setG({ max_per_hour: n })} /></Row>
            <Row title="Günde en fazla"><LimitInput label="Günde en fazla" value={guards.max_per_day} max={10000} fallback={300} onChange={(n) => setG({ max_per_day: n })} /></Row>
            <Row title="Hassas konuda dur" hint="Sağlık, para, kavga, ayrılık gibi konularda otomatik gönderim durur."><Switch checked={guards.stop_sensitive} onCheckedChange={(v) => setG({ stop_sensitive: v })} aria-label="Hassas konuda dur" /></Row>
          </div>
        </Card>

        <Card className="border-destructive/30">
          <SectionTitle hint="Kişi, stil kartı, hafıza ve geçmişi bu bilgisayardan silinir. Geri alınamaz.">Tehlikeli bölge</SectionTitle>
          <Button variant="destructive" onClick={() => setDel(true)}><Trash2 /> {person.name} kişisini sil</Button>
        </Card>
      </div>
      <ConfirmDialog
        open={del}
        onOpenChange={setDel}
        destructive
        title={`${person.name} silinsin mi?`}
        description="Bu kişiye ait tüm veriler kalıcı olarak silinir. Bu işlem geri alınamaz."
        confirmLabel="Evet, sil"
        onConfirm={async () => {
          try { await remove(person.id); toast('Kişi silindi'); navigate('#/') } catch (e) { toast(e instanceof Error ? e.message : String(e), true) }
        }}
      />
    </div>
  )
}
