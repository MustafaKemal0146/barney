import { cn } from '@/lib/utils'

/** Barney — altın noktalı kelime işareti. */
export function Brand({ className, tagline = true }: { className?: string; tagline?: boolean }) {
  return (
    <span className={cn('inline-flex items-baseline gap-1.5 select-none', className)}>
      <span className="text-lg font-bold tracking-tight">Barney</span>
      {tagline && (
        <>
          <span aria-hidden className="relative -top-px size-1.5 self-center rounded-full bg-gold shadow-[0_0_8px_var(--gold)]" />
          <span className="text-sm font-medium text-muted-foreground">senin gibi yazar</span>
        </>
      )}
    </span>
  )
}

export function Avatar({ name, size = 36, className }: { name: string; size?: number; className?: string }) {
  const ch = (name.trim()[0] ?? '?').toLocaleUpperCase('tr-TR')
  return (
    <span
      aria-hidden
      style={{ width: size, height: size, fontSize: size * 0.42 }}
      className={cn('inline-flex shrink-0 items-center justify-center rounded-full bg-primary/20 font-semibold text-primary ring-1 ring-primary/30', className)}
    >
      {ch}
    </span>
  )
}
