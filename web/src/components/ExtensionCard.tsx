import { Puzzle } from 'lucide-react'
import { Card, CopyButton, SectionTitle } from '@/components/kit'
import { useApp } from '@/store/app'

export function ExtensionCard({ bare }: { bare?: boolean }) {
  const state = useApp((s) => s.state)
  const dir = state?.ext_dir ?? ''
  const body = (
    <>
      <div className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border bg-background p-2">
        <code className="min-w-0 flex-1 break-all px-1 font-mono text-xs">{dir || '(uzantı klasörü bilinmiyor)'}</code>
        {dir && <CopyButton text={dir} label="Yolu kopyala" />}
      </div>
      <ol className="list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground">
        <li>
          Chrome'da <code className="rounded bg-muted px-1 font-mono text-xs text-foreground">chrome://extensions</code> aç{' '}
          <CopyButton text="chrome://extensions" label="Adresi kopyala" />
        </li>
        <li>Sağ üstten <b className="text-foreground">Geliştirici modu</b>nu aç.</li>
        <li><b className="text-foreground">Paketlenmemiş öğe yükle</b>ye bas ve yukarıdaki klasörü seç.</li>
        <li>web.whatsapp.com'u aç; uzantı simgesine tıklayınca Barney yan paneli açılır.</li>
      </ol>
      <p className="mt-3 text-xs text-muted-foreground">
        Durum: {state?.connected ? <span className="text-success">uzantı bağlı</span> : <span className="text-warning">henüz bağlı değil — kurduktan sonra WhatsApp Web sekmesini yenile</span>}
      </p>
    </>
  )
  if (bare) return body
  return (
    <Card>
      <SectionTitle hint="Barney, WhatsApp Web / Instagram ile bu uzantı üzerinden konuşur.">
        <span className="inline-flex items-center gap-2"><Puzzle className="size-4 text-primary" /> Chrome uzantısı</span>
      </SectionTitle>
      {body}
    </Card>
  )
}
