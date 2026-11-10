import { MessageCircle, RefreshCw, RotateCw, Square } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { LiveStrip } from '@/components/LiveStrip'
import { isActive } from '@/lib/importJob'
import { Avatar } from '@/components/brand/Brand'
import { Empty } from '@/components/kit'
import { ChatTab } from '@/components/person/ChatTab'
import { LiveTab } from '@/components/person/LiveTab'
import { StyleTab } from '@/components/person/StyleTab'
import { InsightsTab } from '@/components/person/InsightsTab'
import { MemoryTab } from '@/components/person/MemoryTab'
import { HistoryTab } from '@/components/person/HistoryTab'
import { PersonSettingsTab } from '@/components/person/PersonSettingsTab'
import { GOALS, STAGES } from '@/lib/fmt'
import { personHref, TABS, type TabId } from '@/lib/route'
import { useApp } from '@/store/app'
import { api } from '@/lib/api'
import { useState } from 'react'

export function PersonPage({ id, tab }: { id: string; tab: TabId }) {
  const person = useApp((s) => s.people.find((p) => p.id === id))
  const loaded = useApp((s) => s.peopleLoaded)
  const openWa = useApp((s) => s.openInWhatsApp)
  const sync = useApp((s) => s.syncPerson)
  const cancel = useApp((s) => s.cancelImport)
  const toast = useApp((s) => s.showToast)
  const [resyncing, setResyncing] = useState(false)
  const resync = async () => {
    setResyncing(true)
    try { await api.liveResync() } catch (e) { toast(e instanceof Error ? e.message : String(e), true) }
    finally { setTimeout(() => setResyncing(false), 800) }
  }
  const busy = useApp((s) => isActive(s.importJob))
  const liveHere = useApp((s) => s.live.connected && s.live.chat?.person_id === id)
  if (!person) return loaded ? <div className="p-6"><Empty>Bu kişi bulunamadı.</Empty></div> : null
  const goal = GOALS.find((g) => g.value === person.goal)?.label
  const stage = STAGES.find((g) => g.value === person.stage)?.label
  return (
    <div className="flex h-full min-h-0 flex-col">
      <LiveStrip selectedId={id} />
      <div className="shrink-0 border-b">
        <div className="flex items-center gap-3 px-4 pt-3">
          <Avatar name={person.name} size={32} />
          <div className="min-w-0">
            <h1 className="truncate text-base font-semibold leading-tight">{person.name}</h1>
            <p className="truncate text-xs text-muted-foreground">
              {[goal && person.goal ? `Hedef: ${goal}` : '', stage && person.stage ? `Aşama: ${stage}` : ''].filter(Boolean).join(' · ') || `${person.messages_total.toLocaleString('tr-TR')} mesaj`}
            </p>
          </div>
          <div className="ml-auto flex shrink-0 gap-1.5">
            <Button size="sm" variant="outline" aria-label="WhatsApp'ta aç" title="WhatsApp'ta aç" onClick={() => void openWa(person.id)}>
              <MessageCircle /> <span className="hidden sm:inline">WhatsApp'ta aç</span>
            </Button>
            {busy ? (
              <Button size="sm" variant="destructive" aria-label="Durdur" title="Durdur: o ana kadar okunan mesajlar kaydedilir" onClick={() => void cancel()}>
                <Square /> <span className="hidden sm:inline">Durdur</span>
              </Button>
            ) : (
              <Button size="sm" variant="outline" aria-label="Senkronize et" title="Senkronize et: WhatsApp'taki yeni mesajları geçmişe ekler" onClick={() => void sync(person.id)}>
                <RefreshCw /> <span className="hidden sm:inline">Senkronize et</span>
              </Button>
            )}
            <Button size="sm" aria-label="Ekranı senkronize et" title="Senk: yalnızca şu an WhatsApp ekranında yüklü mesajları okur ve ekler (kaydırma yok)" disabled={resyncing} onClick={() => void resync()} className="bg-success text-white hover:bg-success/85">
              <RotateCw className={resyncing ? 'animate-spin' : ''} /> <span className="hidden sm:inline">Senk</span>
            </Button>
          </div>
        </div>
        <nav aria-label="Kişi sekmeleri" className="mt-2 flex gap-1 overflow-x-auto px-3 no-scrollbar">
          {TABS.map((t) => (
            <a
              key={t.id}
              href={personHref(id, t.id)}
              aria-current={tab === t.id ? 'page' : undefined}
              className={cn(
                'relative shrink-0 rounded-t-md px-3 py-2 text-sm font-medium whitespace-nowrap transition-colors',
                tab === t.id ? 'text-foreground after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:rounded-full after:bg-primary' : 'text-muted-foreground hover:text-foreground',
              )}
            >
              {t.label}
              {t.id === 'live' && liveHere && <span className="ml-1.5 inline-block size-1.5 rounded-full bg-success align-middle" />}
            </a>
          ))}
        </nav>
      </div>
      <div className="min-h-0 flex-1" key={person.id + tab}>
        {tab === 'chat' && <ChatTab person={person} />}
        {tab === 'live' && <LiveTab person={person} />}
        {tab === 'style' && <StyleTab person={person} />}
        {tab === 'insights' && <InsightsTab person={person} />}
        {tab === 'memory' && <MemoryTab person={person} />}
        {tab === 'history' && <HistoryTab person={person} />}
        {tab === 'settings' && <PersonSettingsTab person={person} />}
      </div>
    </div>
  )
}
