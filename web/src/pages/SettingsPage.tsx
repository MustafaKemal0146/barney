import { useEffect, useRef, useState } from 'react'
import { Bell, Bot, Smartphone } from 'lucide-react'
import { Switch } from '@/components/ui/switch'
import { Card, Row, SectionTitle, Segmented } from '@/components/kit'
import { DepthControls } from '@/components/DepthControls'
import { DEFAULT_HISTORY } from '@/lib/importJob'
import { ProviderForm } from '@/components/ProviderForm'
import { ExtensionCard } from '@/components/ExtensionCard'
import { askPermission } from '@/lib/notify'
import { useApp } from '@/store/app'

/** Geçmiş derinliği: yerel taslak tutar, kullanıcı durunca (500 ms) kaydeder. */
function HistorySettings() {
  const state = useApp((s) => s.state)
  const patch = useApp((s) => s.patchSettings)
  const h = state?.history ?? DEFAULT_HISTORY
  const [draft, setDraft] = useState(h)
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => { setDraft(h) }, [h.max_messages, h.max_seconds, h.all]) // eslint-disable-line react-hooks/exhaustive-deps
  const change = (v: typeof draft) => {
    setDraft(v)
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      void patch({ history_max_messages: v.max_messages, history_max_seconds: v.max_seconds, history_all: v.all }).catch(() => {})
    }, 500)
  }
  return (
    <div className="space-y-3">
      <DepthControls value={draft} onChange={change} />
      <Row title="Tüm geçmişi oku" hint="Açıkken mesaj sınırı kalkar; süre sınırı yine geçerlidir.">
        <Switch checked={draft.all} onCheckedChange={(v) => change({ ...draft, all: v })} aria-label="Tüm geçmiş" />
      </Row>
    </div>
  )
}

export function SettingsPage() {
  const state = useApp((s) => s.state)
  const patch = useApp((s) => s.patchSettings)
  const toast = useApp((s) => s.showToast)
  const [perm, setPerm] = useState<string>(typeof Notification === 'undefined' ? 'unsupported' : Notification.permission)
  if (!state) return null

  const toggleNotif = async (v: boolean) => {
    if (v) {
      const r = await askPermission()
      setPerm(r)
      if (r !== 'granted') {
        toast(r === 'unsupported' ? 'Bu tarayıcı bildirimleri desteklemiyor.' : 'Tarayıcı bildirim iznini vermedi. Adres çubuğundaki site ayarlarından izin ver.', true)
        return
      }
    }
    try { await patch({ notifications: v }) } catch { /* toast gösterildi */ }
  }
  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-3xl space-y-4 p-3 sm:p-4">
        <h1 className="text-lg font-semibold">Ayarlar</h1>
        <Card>
          <SectionTitle hint="Cevapları üreten model.">Model sağlayıcısı</SectionTitle>
          <ProviderForm />
        </Card>
        <Card className="divide-y py-2">
          <Row
            title={<span className="inline-flex items-center gap-2"><Bell className="size-4 text-muted-foreground" /> Bildirimler</span>}
            hint={perm === 'denied' ? 'Tarayıcı izni reddedilmiş; site ayarlarından aç.' : 'Sekme arka plandayken yeni öneri / onay bekleyen cevap için tarayıcı bildirimi.'}
          >
            <Switch checked={state.notifications} onCheckedChange={(v) => void toggleNotif(v)} aria-label="Bildirimler" />
          </Row>
          <Row title="Geri bildirimden öğren" hint="Düzenlediğin ve reddettiğin cevaplar, sonraki önerileri senin tarzına yaklaştırır.">
            <Switch checked={state.learn_feedback} onCheckedChange={(v) => void patch({ learn_feedback: v }).catch(() => {})} aria-label="Geri bildirimden öğren" />
          </Row>
          <Row
            title={<span className="inline-flex items-center gap-2"><Smartphone className="size-4 text-muted-foreground" /> Telefon erişimi (LAN)</span>}
            hint={state.lan ? 'Açık: aynı Wi-Fi ağındaki cihazlar belirteçle erişebilir.' : 'Kapalı: Barney yalnız bu bilgisayardan (127.0.0.1) erişilebilir.'}
          >
            <span className={state.lan ? 'rounded-full bg-gold/15 px-2.5 py-0.5 text-xs font-medium text-gold' : 'rounded-full bg-muted px-2.5 py-0.5 text-xs font-medium text-muted-foreground'}>
              {state.lan ? 'Açık' : 'Kapalı'}
            </span>
          </Row>
        </Card>
        <Card>
          <SectionTitle hint="WhatsApp'tan kişi eklerken ve senkronize ederken varsayılan derinlik.">Geçmiş okuma</SectionTitle>
          <HistorySettings />
        </Card>
        <Card className="divide-y py-2">
          <Row
            title="Toplu ekleme hızı"
            hint={state.bulk_speed === 'hizli' ? 'Hızlı: sohbetler arasında bekleme kısa; WhatsApp otomasyonu fark edebilir.' : 'Yavaş (önerilen): sohbetler arasında 5–15 sn beklenir.'}
          >
            <Segmented
              value={state.bulk_speed ?? 'yavas'}
              onChange={(v) => void patch({ bulk_speed: v }).catch(() => {})}
              options={[{ value: 'yavas', label: 'Yavaş (önerilen)' }, { value: 'hizli', label: 'Hızlı' }]}
            />
          </Row>
          <Row title="Sürekli öğrenme" hint="Kayıtlı kişilerin canlı sohbetindeki yeni mesajlar geçmişe eklenir (yalnız bu bilgisayarda saklanır).">
            <Switch checked={!!state.auto_learn} onCheckedChange={(v) => void patch({ auto_learn: v }).catch(() => {})} aria-label="Sürekli öğrenme" />
          </Row>
        </Card>
        <Card>
          <SectionTitle hint="Kişi eklerken 'AI ile ekle' sekmesi.">
            <span className="inline-flex items-center gap-2"><Bot className="size-4 text-primary" /> AI tarayıcı modu</span>
          </SectionTitle>
          <div className="space-y-2 text-xs text-muted-foreground">
            <p>Normal okuyucu çalışmazsa yapay zekâ, WhatsApp Web sekmesini sizin için gezer: yalnız kaydırır, tıklar ve okur; <b className="text-foreground">asla mesaj göndermez</b>.</p>
            <p>Sayfa içeriği (isimler ve mesajlar) seçtiğiniz model sağlayıcısına gönderilir; maskeleme yapılmaz. Her kullanımda 'Anladım' onayı istenir.</p>
          </div>
        </Card>
        <ExtensionCard />
        <p className="text-center text-xs text-muted-foreground">Barney · v{state.version}</p>
      </div>
    </div>
  )
}
