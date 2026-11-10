import { useApp } from '@/store/app'
import { cn } from '@/lib/utils'

export function StatusBar() {
  const state = useApp((s) => s.state)
  const sse = useApp((s) => s.sse)
  const sseLabel = sse === 'open' ? 'canlı akış açık' : sse === 'idle' ? '' : sse === 'connecting' ? 'bağlanıyor…' : 'yeniden bağlanıyor…'
  return (
    <footer className="flex h-7 shrink-0 items-center gap-4 overflow-x-auto border-t bg-background px-3 text-[11px] whitespace-nowrap text-muted-foreground">
      <span>Bugün <b className="font-semibold text-foreground tabular-nums">{state?.stats.calls_today ?? 0}</b> model çağrısı</span>
      <span>Son 1 saatte <b className="font-semibold text-foreground tabular-nums">{state?.stats.sent_last_hour ?? 0}</b> gönderim</span>
      <span className={cn(state?.stats.auto_send && 'text-gold')}>Otomatik: {state?.stats.auto_send ? 'açık' : 'kapalı'}</span>
      <span className="ml-auto flex items-center gap-1.5">
        {sseLabel && (
          <>
            <span className={cn('size-1.5 rounded-full', sse === 'open' ? 'bg-success' : 'dot-pulse bg-warning')} />
            {sseLabel}
          </>
        )}
        {state?.version && <span className="opacity-60">v{state.version}</span>}
      </span>
    </footer>
  )
}
