import { useState } from 'react'
import { KeyRound, ShieldAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, NativeSelect } from '@/components/kit'
import { useApp } from '@/store/app'

export const PRIVACY_NOTE =
  'Sohbet metinleri, seçtiğin model sağlayıcısına gönderilir. Telefon numaraları ve e-posta adresleri gönderilmeden önce maskelenir. Anahtarın yalnız bu bilgisayarda saklanır ve arayüze asla geri gösterilmez.'

export function ProviderForm({ compact }: { compact?: boolean }) {
  const state = useApp((s) => s.state)
  const patch = useApp((s) => s.patchSettings)
  const toast = useApp((s) => s.showToast)
  const [key, setKey] = useState('')
  const [model, setModel] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  if (!state) return null
  const run = async (fn: () => Promise<void>, ok: string) => {
    setBusy(true)
    try {
      await fn()
      toast(ok)
    } catch {
      /* store zaten hata gösterdi */
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-4">
      <div className={compact ? 'grid gap-3' : 'grid gap-3 sm:grid-cols-2'}>
        <Field label="Sağlayıcı">
          <NativeSelect
            label="Sağlayıcı"
            className="w-full"
            value={state.provider}
            options={state.providers.map((p) => ({ value: p, label: p }))}
            onChange={(v) => run(() => patch({ provider: v }), 'Sağlayıcı değişti')}
          />
        </Field>
        <Field label="Model">
          <div className="flex gap-2">
            <Input value={model ?? state.model} onChange={(e) => setModel(e.target.value)} placeholder="model adı" aria-label="Model" />
            {model !== null && model !== state.model && (
              <Button variant="outline" disabled={busy} onClick={() => run(async () => { await patch({ model: model! }); setModel(null) }, 'Model kaydedildi')}>Kaydet</Button>
            )}
          </div>
        </Field>
      </div>
      <Field
        label="API anahtarı"
        hint={state.key_set ? 'Bir anahtar kayıtlı. Güvenlik için gösterilmez; değiştirmek için yenisini gir.' : 'Henüz anahtar yok.'}
      >
        <div className="flex flex-wrap gap-2">
          <div className="relative min-w-48 flex-1">
            <KeyRound className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground" />
            <Input
              type="password"
              autoComplete="off"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder={state.key_set ? '••••••••••••' : 'sk-…'}
              aria-label="API anahtarı"
              className="pl-8"
            />
          </div>
          <Button disabled={busy || !key.trim()} onClick={() => run(async () => { await patch({ key: key.trim() }); setKey('') }, 'Anahtar kaydedildi')}>Kaydet</Button>
          {state.key_set && (
            <Button variant="destructive" disabled={busy} onClick={() => run(() => patch({ key: '' }), 'Anahtar silindi')}>Anahtarı sil</Button>
          )}
        </div>
      </Field>
      <div className="flex gap-2 rounded-lg border border-warning/30 bg-warning/10 p-3 text-xs text-warning">
        <ShieldAlert className="mt-0.5 size-4 shrink-0" />
        <span>{PRIVACY_NOTE}</span>
      </div>
    </div>
  )
}
