import { Brand } from '@/components/brand/Brand'
import { KillSwitch, StatusPills } from './Topbar'
import { useApp } from '@/store/app'
import { personHref, useRoute } from '@/lib/route'

/** ?embed=1 (Chrome yan paneli): kenar çubuğu yok; küçük bir kişi seçici ve durum. */
export function EmbedBar() {
  const people = useApp((s) => s.people)
  const route = useRoute()
  const cur = route.page === 'person' ? route.id : ''
  return (
    <header className="shrink-0 space-y-2 border-b bg-background px-3 py-2">
      <div className="flex items-center justify-between gap-2">
        <Brand tagline={false} />
        <KillSwitch />
      </div>
      <div className="flex gap-2 overflow-x-auto no-scrollbar">
        <StatusPills />
      </div>
      {people.length > 0 && (
        <select
          aria-label="Kişi seç"
          value={cur}
          onChange={(e) => { window.location.hash = personHref(e.target.value) }}
          className="h-8 w-full rounded-lg border bg-card px-2 text-sm"
        >
          {!cur && <option value="">Kişi seç…</option>}
          {people.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      )}
    </header>
  )
}
