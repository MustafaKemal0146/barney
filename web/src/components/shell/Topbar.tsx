import { Menu, OctagonX, Play, Square } from 'lucide-react'
import { Brand } from '@/components/brand/Brand'
import { Pill } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { useApp } from '@/store/app'
import { navigate, SETTINGS_HREF } from '@/lib/route'

export function StatusPills() {
  const state = useApp((s) => s.state)
  const live = useApp((s) => s.live)
  if (!state) return null
  const platform = live.platform === 'whatsapp' ? 'WhatsApp' : live.platform === 'instagram' ? 'Instagram' : ''
  return (
    <>
      <Pill ok={state.connected} title="Chrome uzantısı ile bağlantı">
        {state.connected ? 'Uzantı bağlı' : 'Uzantı bağlı değil'}
      </Pill>
      {state.connected && platform && <Pill ok title="Açık sohbetin bulunduğu platform">{platform}</Pill>}
      <Pill ok={state.key_set} title="Model API anahtarı" onClick={state.key_set ? undefined : () => navigate(SETTINGS_HREF)}>
        {state.key_set ? 'Model anahtarı var' : 'Model anahtarı yok'}
      </Pill>
    </>
  )
}

export function KillSwitch() {
  const state = useApp((s) => s.state)
  const setPaused = useApp((s) => s.setPaused)
  if (!state) return null
  return state.paused ? (
    <Button size="sm" onClick={() => setPaused(false)} className="bg-success/15 text-success hover:bg-success/25" title="Barney'i sürdür">
      <Play /> Sürdür
    </Button>
  ) : (
    <Button size="sm" variant="destructive" onClick={() => setPaused(true)} title="Tüm otomatik ve canlı işlemleri durdur">
      <Square className="fill-current" /> Durdur
    </Button>
  )
}

export function Topbar() {
  const setDrawer = useApp((s) => s.setDrawer)
  const paused = useApp((s) => s.state?.paused)
  return (
    <header className="shrink-0 border-b bg-background/80 backdrop-blur">
      {paused && (
        <div className="flex items-center justify-center gap-2 bg-destructive/15 px-3 py-1 text-xs font-medium text-destructive">
          <OctagonX className="size-3.5" /> Barney durduruldu — hiçbir şey gönderilmeyecek
        </div>
      )}
      <div className="flex h-12 items-center gap-2 px-3">
        <Button variant="ghost" size="icon" className="md:hidden" aria-label="Menü" onClick={() => setDrawer(true)}>
          <Menu />
        </Button>
        <Brand className="md:hidden" tagline={false} />
        <div className="ml-1 flex min-w-0 flex-1 items-center gap-2 overflow-x-auto md:ml-0 no-scrollbar">
          <StatusPills />
        </div>
        <KillSwitch />
      </div>
    </header>
  )
}
