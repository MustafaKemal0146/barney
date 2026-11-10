import { MessageCircle, Moon, Plus, Settings, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Brand, Avatar } from '@/components/brand/Brand'
import { Button } from '@/components/ui/button'
import { useApp } from '@/store/app'
import { SETTINGS_HREF, personHref, useRoute } from '@/lib/route'
import { relTime, silenceDays, silenceExceeded } from '@/lib/fmt'

function Content({ onNavigate }: { onNavigate?: () => void }) {
  const people = useApp((s) => s.people)
  const loaded = useApp((s) => s.peopleLoaded)
  const openAdd = useApp((s) => s.openAdd)
  const openWa = useApp((s) => s.openInWhatsApp)
  const route = useRoute()
  const sorted = [...people].sort((a, b) => b.last_message_ts - a.last_message_ts)
  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 shrink-0 items-center justify-between px-4">
        <Brand />
      </div>
      <div className="px-3 pb-2">
        <Button className="w-full" variant="outline" onClick={() => { openAdd(); onNavigate?.() }}>
          <Plus /> Kişi ekle
        </Button>
      </div>
      <nav aria-label="Kişiler" className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2 py-1">
        {loaded && sorted.length === 0 && (
          <p className="px-3 py-6 text-center text-xs text-muted-foreground">Henüz kimse yok. Açık bir WhatsApp sohbetini okuyarak başla.</p>
        )}
        {sorted.map((p) => {
          const active = route.page === 'person' && route.id === p.id
          const exceeded = silenceExceeded(p)
          return (
            <div key={p.id} className="group relative">
            <a
              href={personHref(p.id)}
              onClick={onNavigate}
              aria-current={active ? 'page' : undefined}
              className={cn(
                'flex items-center gap-3 rounded-lg px-2.5 py-2 transition-colors',
                active ? 'bg-accent' : 'hover:bg-accent/50',
              )}
            >
              <Avatar name={p.name} />
              <span className="min-w-0 flex-1">
                <span className="flex items-baseline justify-between gap-2">
                  <span className="flex min-w-0 items-center gap-1.5 text-sm font-medium">
                    <span className="truncate">{p.name}</span>
                    {p.auto_send && <span title="Otomatik gönderim açık" className="size-1.5 shrink-0 rounded-full bg-gold" />}
                  </span>
                  <span className="shrink-0 text-[11px] text-muted-foreground">{relTime(p.last_message_ts)}</span>
                </span>
                {exceeded ? (
                  <span className="mt-0.5 inline-flex items-center gap-1 rounded-full bg-warning/10 px-1.5 py-px text-[11px] text-warning">
                    <Moon className="size-3" /> {silenceDays(p)} gündür yazışmadınız
                  </span>
                ) : (
                  <span className="block truncate text-xs text-muted-foreground">
                    {p.messages_total.toLocaleString('tr-TR')} mesaj{p.confidence === 'düşük' ? ' · az veri' : ''}
                  </span>
                )}
              </span>
            </a>
            <button
              type="button"
              aria-label={`${p.name} sohbetini WhatsApp'ta aç`}
              title="WhatsApp'ta aç"
              onClick={() => void openWa(p.id)}
              className="absolute right-1.5 bottom-1.5 flex size-6 items-center justify-center rounded-md bg-background/80 text-muted-foreground opacity-100 transition-opacity hover:text-foreground focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring md:opacity-0 md:group-focus-within:opacity-100 md:group-hover:opacity-100"
            >
              <MessageCircle className="size-3.5" />
            </button>
            </div>
          )
        })}
      </nav>
      <div className="shrink-0 border-t p-2">
        <a
          href={SETTINGS_HREF}
          onClick={onNavigate}
          className={cn('flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors', route.page === 'settings' ? 'bg-accent' : 'hover:bg-accent/50')}
        >
          <Settings className="size-4 text-muted-foreground" /> Ayarlar
        </a>
      </div>
    </div>
  )
}

export function Sidebar() {
  const open = useApp((s) => s.drawerOpen)
  const setDrawer = useApp((s) => s.setDrawer)
  return (
    <>
      <aside className="hidden w-72 shrink-0 border-r bg-background md:block">
        <Content />
      </aside>
      {/* Telefon: çekmece */}
      <div className={cn('fixed inset-0 z-40 md:hidden', open ? 'pointer-events-auto' : 'pointer-events-none')} aria-hidden={!open}>
        <div className={cn('absolute inset-0 bg-black/60 transition-opacity', open ? 'opacity-100' : 'opacity-0')} onClick={() => setDrawer(false)} />
        <aside className={cn('absolute inset-y-0 left-0 w-[82%] max-w-80 border-r bg-background shadow-2xl transition-transform', open ? 'translate-x-0' : '-translate-x-full')}>
          <button type="button" aria-label="Kapat" onClick={() => setDrawer(false)} className="absolute top-3.5 right-3 rounded p-1 text-muted-foreground hover:text-foreground">
            <X className="size-4" />
          </button>
          <Content onNavigate={() => setDrawer(false)} />
        </aside>
      </div>
    </>
  )
}
