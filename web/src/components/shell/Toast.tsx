import { Check, TriangleAlert, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useApp } from '@/store/app'

export function Toast() {
  const toast = useApp((s) => s.toast)
  const dismiss = useApp((s) => s.dismissToast)
  if (!toast) return null
  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-10 z-[60] flex justify-center px-4 sm:justify-end sm:pr-6" role="status" aria-live="polite">
      <div className={cn('rise pointer-events-auto flex max-w-md items-start gap-2.5 rounded-lg border bg-popover px-3 py-2 text-sm shadow-lg', toast.error ? 'border-destructive/40 text-destructive' : 'border-success/40')}>
        {toast.error ? <TriangleAlert className="mt-0.5 size-4 shrink-0" /> : <Check className="mt-0.5 size-4 shrink-0 text-success" />}
        <span className="min-w-0 flex-1 text-foreground">{toast.text}</span>
        <button type="button" onClick={dismiss} aria-label="Kapat" className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground">
          <X className="size-3.5" />
        </button>
      </div>
    </div>
  )
}
