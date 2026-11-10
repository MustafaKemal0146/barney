import { Check, KeyRound, Puzzle, UserPlus } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Brand } from '@/components/brand/Brand'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/kit'
import { ExtensionCard } from '@/components/ExtensionCard'
import { ProviderForm } from '@/components/ProviderForm'
import { AddPersonFlow } from '@/components/AddPerson'
import { useApp } from '@/store/app'
import { useState } from 'react'

const STEPS = [
  { id: 'ext', title: 'Uzantıyı kur', icon: Puzzle },
  { id: 'key', title: 'Model anahtarı', icon: KeyRound },
  { id: 'person', title: 'İlk kişi', icon: UserPlus },
] as const

export function SetupWizard() {
  const state = useApp((s) => s.state)
  const dismiss = useApp((s) => s.dismissSetup)
  const done = { ext: !!state?.connected, key: !!state?.key_set, person: !!state?.has_people }
  const firstOpen = STEPS.findIndex((s) => !done[s.id])
  const [manual, setManual] = useState<number | null>(null)
  const cur = manual ?? (firstOpen === -1 ? STEPS.length - 1 : firstOpen)
  const allDone = firstOpen === -1
  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-2xl px-4 py-8 sm:py-12">
        <div className="mb-6 text-center">
          <Brand className="text-2xl" />
          <h1 className="mt-4 text-xl font-semibold">Kuruluma hoş geldin</h1>
          <p className="mt-1 text-sm text-muted-foreground">Üç adım: uzantı, model anahtarı ve ilk kişi. İstediğin adımı atlayabilirsin.</p>
        </div>
        <ol className="mb-5 flex gap-2">
          {STEPS.map((s, i) => {
            const Icon = s.icon
            const ok = done[s.id]
            return (
              <li key={s.id} className="flex-1">
                <button
                  type="button"
                  onClick={() => setManual(i)}
                  className={cn('flex w-full items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm transition-colors', i === cur ? 'border-primary bg-primary/10' : 'hover:bg-accent/50')}
                >
                  <span className={cn('flex size-6 shrink-0 items-center justify-center rounded-full text-xs', ok ? 'bg-success/20 text-success' : 'bg-muted text-muted-foreground')}>
                    {ok ? <Check className="size-3.5" /> : <Icon className="size-3.5" />}
                  </span>
                  <span className="hidden truncate sm:block">{s.title}</span>
                  <span className="truncate sm:hidden">{i + 1}</span>
                </button>
              </li>
            )
          })}
        </ol>
        <Card>
          {cur === 0 && (
            <div>
              <h2 className="mb-3 text-base font-semibold">1 · Chrome uzantısını kur</h2>
              <ExtensionCard bare />
            </div>
          )}
          {cur === 1 && (
            <div>
              <h2 className="mb-3 text-base font-semibold">2 · Model anahtarı</h2>
              <ProviderForm compact />
            </div>
          )}
          {cur === 2 && (
            <div>
              <h2 className="mb-3 text-base font-semibold">3 · İlk kişiyi ekle</h2>
              <AddPersonFlow onClose={dismiss} />
            </div>
          )}
        </Card>
        <div className="mt-4 flex items-center justify-between">
          <Button variant="ghost" onClick={dismiss}>{allDone ? 'Bitir' : 'Şimdilik atla'}</Button>
          <div className="flex gap-2">
            {cur > 0 && <Button variant="outline" onClick={() => setManual(cur - 1)}>Geri</Button>}
            {cur < STEPS.length - 1 && <Button onClick={() => setManual(cur + 1)}>İleri</Button>}
          </div>
        </div>
      </div>
    </div>
  )
}
