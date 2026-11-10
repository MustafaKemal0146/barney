import { MessageCircle, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { navigate, personHref } from '@/lib/route'
import { useApp } from '@/store/app'

/**
 * Açık WhatsApp sohbeti seçili kişiden farklıysa bir şerit gösterir. Rotayı ASLA kendiliğinden
 * değiştirmez; geçiş yalnız 'Ona geç' tıklanınca olur.
 */
export function LiveStrip({ selectedId }: { selectedId: string | null }) {
  const live = useApp((s) => s.live)
  const openAdd = useApp((s) => s.openAdd)
  const chat = live.connected ? live.chat : null
  if (!chat) return null
  if (selectedId && chat.person_id === selectedId) {
    return (
      <div className="flex shrink-0 items-center gap-1.5 border-b px-4 py-1 text-[11px] text-muted-foreground" data-testid="live-strip-same">
        <span className="size-1.5 rounded-full bg-success" /> WhatsApp'ta açık
      </div>
    )
  }
  const wa = live.platform === 'whatsapp'
  return (
    <div className="flex shrink-0 items-center gap-2 border-b bg-gold/10 px-3 py-1.5 text-xs" data-testid="live-strip">
      <MessageCircle className="size-3.5 shrink-0 text-gold" />
      <span className="min-w-0 flex-1 truncate">
        {wa ? "WhatsApp'ta şu an" : 'Şu an açık'}: <b>{chat.partner}</b>
      </span>
      {chat.person_id ? (
        <Button size="xs" variant="outline" onClick={() => navigate(personHref(chat.person_id!))}>Ona geç</Button>
      ) : wa ? (
        <Button size="xs" variant="outline" onClick={() => openAdd('current')}><Plus /> Ekle</Button>
      ) : null}
    </div>
  )
}
