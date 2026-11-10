import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, Bot, CheckCircle2, Download, ListChecks, MessageCircle, Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Spinner } from '@/components/kit'
import { Avatar } from '@/components/brand/Brand'
import { DepthControls } from '@/components/DepthControls'
import { AiConsent, ImportProgress } from '@/components/ImportProgress'
import { MAX_BULK, isActive, optsFrom } from '@/lib/importJob'
import { navigate, personHref } from '@/lib/route'
import { num, pct } from '@/lib/fmt'
import type { HistoryPrefs, ImportJob, Person } from '@/lib/types'
import { useApp, type AddTab } from '@/store/app'

export const LOW_CONFIDENCE_MSGS = 300

export function PersonSummary({ p }: { p: Person }) {
  const c = p.card.owner
  const low = p.confidence === 'düşük' || p.messages_total < LOW_CONFIDENCE_MSGS
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <Avatar name={p.name} size={44} />
        <div className="min-w-0">
          <div className="truncate font-semibold">{p.name}</div>
          <div className="text-xs text-muted-foreground">
            {num(p.messages_total)} mesaj · senin {num(p.messages_owner)} · onun {num(p.messages_partner)} · güven: {p.confidence}
          </div>
        </div>
      </div>
      {low && (
        <div className="flex gap-2 rounded-lg border border-warning/30 bg-warning/10 p-3 text-xs text-warning">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" />
          <span>Bu kişiyle {LOW_CONFIDENCE_MSGS} mesajdan az veri var. Stil kartı güvenilir olmayabilir; öneriler genel kalabilir. Daha derin bir geçmiş okursan sonuç iyileşir.</span>
        </div>
      )}
      <div>
        <div className="mb-1.5 text-xs font-medium text-muted-foreground">Senin stilin (özet)</div>
        <div className="grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
          {[
            ['Mesaj uzunluğu', `~${c.words_median} kelime`],
            ['Seri mesaj', `~${c.burst.median} arka arkaya`],
            ['Küçük harf başlangıç', pct(c.lower_start_pct)],
            ['Emoji', pct(c.emoji_pct)],
          ].map(([k, v]) => (
            <div key={k} className="rounded-lg border bg-raised/60 px-2.5 py-2">
              <div className="text-muted-foreground">{k}</div>
              <div className="mt-0.5 text-sm font-semibold">{v}</div>
            </div>
          ))}
        </div>
        {((c.signature?.length ?? 0) > 0 || (c.top_emoji?.length ?? 0) > 0) && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {(c.signature ?? []).map((s) => <span key={s} className="rounded-full border px-2 py-0.5 text-xs">{s}</span>)}
            {(c.top_emoji ?? []).map((s) => <span key={s} className="rounded-full border px-2 py-0.5 text-xs">{s}</span>)}
          </div>
        )}
      </div>
    </div>
  )
}

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

function OpenHelp({ extConnected }: { extConnected: boolean }) {
  return (
    <div className="space-y-1.5 rounded-lg border border-warning/30 bg-warning/10 p-3 text-xs text-warning" data-testid="open-help">
      <div className="flex items-center gap-2 font-medium"><AlertTriangle className="size-4 shrink-0" /> {extConnected ? 'Açık bir WhatsApp sohbeti görünmüyor' : 'Chrome uzantısı bağlı değil'}</div>
      <ol className="list-decimal space-y-0.5 pl-5 text-foreground/80">
        <li>Chrome'da <b>web.whatsapp.com</b> adresini aç ve giriş yap (QR ile).</li>
        {!extConnected && <li>Barney uzantısının kurulu ve açık olduğundan emin ol (Ayarlar → Uzantı).</li>}
        <li>Okumak istediğin sohbete tıkla; bu ekranda adı belirecek.</li>
        <li>Okuma sürerken WhatsApp sekmesini açık ve görünür tut.</li>
      </ol>
    </div>
  )
}

function useDepth() {
  const hist = useApp((s) => s.state?.history)
  const [depth, setDepth] = useState<HistoryPrefs>(() => {
    const o = optsFrom(useApp.getState().state)
    return { max_messages: o.max_messages, max_seconds: o.max_seconds, all: o.all }
  })
  // Ayarlar sonradan (ilk yükleme) gelirse, kullanıcı dokunmadıysa varsayılanı güncelle
  const [touched, setTouched] = useState(false)
  useEffect(() => {
    if (!touched && hist) setDepth({ max_messages: hist.max_messages, max_seconds: hist.max_seconds, all: hist.all })
  }, [hist, touched])
  return [depth, (v: HistoryPrefs) => { setTouched(true); setDepth(v) }] as const
}

function TabCurrent({ ai, onStarted }: { ai: boolean; onStarted: (j: ImportJob) => void }) {
  const live = useApp((s) => s.live)
  const ext = useApp((s) => s.state?.connected ?? false)
  const consent = useApp((s) => s.aiConsent)
  const start = useApp((s) => s.importCurrent)
  const [depth, setDepth] = useDepth()
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const chat = live.connected && live.platform === 'whatsapp' ? live.chat : null
  const connected = live.connected || ext
  // Okuyucu açık sohbet ister; AI modu sekmeyi kendisi gezer, yalnız uzantı bağlantısı yeter.
  const ready = ai ? connected : !!chat
  const go = async () => {
    setErr('')
    setBusy(true)
    try {
      onStarted(await start({ ...depth, mode: ai ? 'ai' : 'reader' }))
    } catch (e) {
      setErr(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-4">
      {ai ? (
        <>
          <p className="text-sm text-muted-foreground">Okuyucu işe yaramazsa ya da hangi sohbet olduğunu sen göstermek istemezsen: yapay zekâ WhatsApp Web'i gezip sohbeti bulur ve geçmişi okur.</p>
          <AiConsent />
          {!connected && <OpenHelp extConnected={false} />}
        </>
      ) : chat ? (
        <div className="flex items-center gap-3 rounded-lg border bg-raised/50 p-3" data-testid="live-chat-name">
          <Avatar name={chat.partner} />
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold">{chat.partner}</div>
            <div className="text-xs text-muted-foreground">
              Şu an WhatsApp'ta açık sohbet{chat.person_id ? ' · zaten kayıtlı (yeni mesajlar birleştirilir)' : ''}
            </div>
          </div>
        </div>
      ) : (
        <OpenHelp extConnected={ext} />
      )}
      <DepthControls value={depth} onChange={setDepth} />
      <p className="text-xs text-muted-foreground">Okuma sırasında WhatsApp sekmesi <b>açık ve görünür</b> kalmalı; arka plandaki sekmelerde kaydırma yavaşlar.</p>
      {err && <p className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{err}</p>}
      <Button size="lg" disabled={!ready || busy || (ai && !consent)} onClick={() => void go()}>
        {busy ? <Spinner className="size-4" /> : ai ? <Bot /> : <Download />}
        {ai ? 'AI ile okumayı başlat' : 'Okumayı başlat'}
      </Button>
    </div>
  )
}

function TabBulk({ onStarted }: { onStarted: (j: ImportJob) => void }) {
  const list = useApp((s) => s.chatList)
  const loading = useApp((s) => s.chatListLoading)
  const req = useApp((s) => s.requestChatList)
  const people = useApp((s) => s.people)
  const ext = useApp((s) => s.state?.connected ?? false)
  const liveConn = useApp((s) => s.live.connected)
  const startBulk = useApp((s) => s.importBulk)
  const [depth, setDepth] = useDepth()
  const [q, setQ] = useState('')
  const [sel, setSel] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [slow, setSlow] = useState(false)
  const known = useMemo(() => new Set(people.map((p) => p.name.toLocaleLowerCase('tr-TR'))), [people])
  const shown = useMemo(() => {
    const t = q.trim().toLocaleLowerCase('tr-TR')
    return (list ?? []).filter((c) => !t || c.name.toLocaleLowerCase('tr-TR').includes(t))
  }, [list, q])

  // Liste uzun süre gelmezse kullanıcıya bildir (sekme kapalı / uzantı yanıt vermiyor olabilir)
  useEffect(() => {
    setSlow(false)
    if (!loading) return
    const t = setTimeout(() => setSlow(true), 20000)
    return () => clearTimeout(t)
  }, [loading])

  const fetchList = async () => {
    setErr('')
    try {
      await req()
    } catch (e) {
      setErr(errText(e))
    }
  }
  const toggle = (name: string) =>
    setSel((s) => (s.includes(name) ? s.filter((x) => x !== name) : s.length >= MAX_BULK ? s : [...s, name]))
  const go = async () => {
    setErr('')
    setBusy(true)
    try {
      onStarted(await startBulk(sel, { ...depth, mode: 'reader' }))
    } catch (e) {
      setErr(errText(e))
    } finally {
      setBusy(false)
    }
  }
  const connected = liveConn || ext
  return (
    <div className="space-y-4">
      {!connected && <OpenHelp extConnected={false} />}
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" disabled={loading || !connected} onClick={() => void fetchList()}>
          {loading ? <Spinner className="size-4" /> : <ListChecks />} Sohbet listesini getir
        </Button>
        {loading && <span className="text-xs text-muted-foreground">Uzantı sol listeyi okuyor…</span>}
      </div>
      {slow && loading && <p className="text-xs text-warning">Liste gelmedi. WhatsApp Web sekmesinin açık olduğundan emin olup tekrar dene.</p>}
      {list && (
        <div className="space-y-2">
          <div className="relative">
            <Search className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground" />
            <Input className="pl-8" placeholder="Sohbetlerde ara" aria-label="Sohbetlerde ara" value={q} onChange={(e) => setQ(e.target.value)} />
          </div>
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span data-testid="sel-counter" className={cn(sel.length >= MAX_BULK && 'font-medium text-warning')}>{sel.length}/{MAX_BULK} seçili</span>
            {sel.length > 0 && <button type="button" className="hover:text-foreground hover:underline" onClick={() => setSel([])}>Seçimi temizle</button>}
          </div>
          <ul className="max-h-56 divide-y overflow-y-auto rounded-lg border" aria-label="Sohbet listesi">
            {shown.length === 0 && <li className="px-3 py-4 text-center text-xs text-muted-foreground">Eşleşen sohbet yok.</li>}
            {shown.map((c) => {
              const on = sel.includes(c.name)
              const full = !on && sel.length >= MAX_BULK
              return (
                <li key={c.name}>
                  <label className={cn('flex cursor-pointer items-center gap-3 px-3 py-2 text-sm hover:bg-accent/40', full && 'cursor-not-allowed opacity-50')}>
                    <input type="checkbox" className="size-4 shrink-0 accent-[var(--primary)]" checked={on} disabled={full} onChange={() => toggle(c.name)} />
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-2">
                        <span className="truncate font-medium">{c.name}</span>
                        {known.has(c.name.toLocaleLowerCase('tr-TR')) && <span className="shrink-0 rounded-full bg-muted px-1.5 py-px text-[10px] text-muted-foreground">kayıtlı</span>}
                        {c.unread > 0 && <span className="shrink-0 rounded-full bg-primary/20 px-1.5 text-[10px] text-primary">{c.unread}</span>}
                      </span>
                      <span className="block truncate text-xs text-muted-foreground">{c.last}</span>
                    </span>
                  </label>
                </li>
              )
            })}
          </ul>
        </div>
      )}
      {list && (
        <>
          <DepthControls value={depth} onChange={setDepth} />
          <p className="rounded-lg border bg-raised/50 px-3 py-2 text-xs text-muted-foreground">
            Toplu ekleme bilerek yavaş çalışır: sohbetler arasında 5–15 sn beklenir (WhatsApp'ı yormamak için). WhatsApp sekmesi açık ve görünür kalmalı.
          </p>
        </>
      )}
      {err && <p className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{err}</p>}
      {list && (
        <Button size="lg" disabled={sel.length === 0 || busy} onClick={() => void go()}>
          {busy ? <Spinner className="size-4" /> : <Download />} Seçilenleri ekle{sel.length > 0 ? ` (${sel.length})` : ''}
        </Button>
      )}
    </div>
  )
}

function DoneView({ job, onAgain, onClose }: { job: ImportJob; onAgain: () => void; onClose?: () => void }) {
  const people = useApp((s) => s.people)
  const loadPeople = useApp((s) => s.loadPeople)
  const found = job.person_ids.map((id) => people.find((p) => p.id === id)).filter((p): p is Person => !!p)
  const missing = found.length < job.person_ids.length
  useEffect(() => {
    if (missing) void loadPeople()
  }, [missing, loadPeople])
  if (found.length === 0) {
    return <div className="flex items-center justify-center gap-3 py-8 text-sm text-muted-foreground"><Spinner className="size-5" /> Stil kartı hazırlanıyor…</div>
  }
  const p = found[0]
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 text-sm font-medium text-success">
        <CheckCircle2 className="size-4" /> {job.kind === 'sync' ? 'Senkronize edildi' : found.length > 1 ? `${found.length} kişi eklendi` : 'Kişi eklendi'}
      </div>
      <PersonSummary p={p} />
      {found.length > 1 && (
        <p className="text-xs text-muted-foreground">Diğerleri: {found.slice(1).map((x) => x.name).join(', ')}</p>
      )}
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={onAgain}>Başka ekle</Button>
        <Button onClick={() => { onClose?.(); navigate(personHref(p.id)) }}>Kişiye git</Button>
      </div>
    </div>
  )
}

const TABS: { id: AddTab; label: string; icon: typeof MessageCircle }[] = [
  { id: 'current', label: "WhatsApp'tan oku", icon: MessageCircle },
  { id: 'bulk', label: 'Listeden seç (toplu)', icon: ListChecks },
  { id: 'ai', label: 'AI ile ekle', icon: Bot },
]

export function AddPersonFlow({ onClose, initialTab = 'current' }: { onClose?: () => void; initialTab?: AddTab }) {
  const job = useApp((s) => s.importJob)
  const [tab, setTab] = useState<AddTab>(initialTab)
  const [startedId, setStartedId] = useState<string | null>(null)
  const [dismissedId, setDismissedId] = useState<string | null>(null)

  // Akışta görünen aktif işi "bizim" say: AI'a geçiş gibi başka yerden başlatılan işler de bitince özet gösterilsin
  const activeId = job && isActive(job) ? job.id : null
  useEffect(() => {
    if (activeId) setStartedId(activeId)
  }, [activeId])
  const showJob = job && job.id !== dismissedId && (isActive(job) || job.id === startedId)
  if (showJob) {
    if (job.state === 'done') {
      return <DoneView job={job} onClose={onClose} onAgain={() => setDismissedId(job.id)} />
    }
    return <ImportProgress job={job} onDismiss={() => setDismissedId(job.id)} />
  }
  const started = (j: ImportJob) => {
    setStartedId(j.id)
    setDismissedId(null)
  }
  return (
    <div className="space-y-4">
      <div role="tablist" aria-label="Ekleme yöntemi" className="grid grid-cols-3 gap-1 rounded-lg border bg-background p-0.5">
        {TABS.map((t) => (
          <button
            key={t.id} type="button" role="tab" aria-selected={tab === t.id} onClick={() => setTab(t.id)}
            className={cn('flex flex-col items-center justify-center gap-0.5 rounded-md px-1.5 py-1.5 text-center text-xs leading-tight font-medium transition-colors sm:flex-row sm:gap-1.5 sm:text-sm', tab === t.id ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground')}
          >
            <t.icon className="size-3.5 shrink-0" /> <span>{t.label}</span>
          </button>
        ))}
      </div>
      {tab === 'current' && <TabCurrent ai={false} onStarted={started} />}
      {tab === 'bulk' && <TabBulk onStarted={started} />}
      {tab === 'ai' && <TabCurrent ai onStarted={started} />}
    </div>
  )
}

export function AddPersonDialog() {
  const open = useApp((s) => s.addOpen)
  const setOpen = useApp((s) => s.setAddOpen)
  const tab = useApp((s) => s.addTab)
  const [n, setN] = useState(0) // her açılışta akışı sıfırla
  return (
    <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (v) setN((x) => x + 1) }}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Kişi ekle</DialogTitle>
          <DialogDescription>Açık WhatsApp Web sohbetinin geçmişini okuyup o kişiyle yazışma stilini öğrenir. Dosya yüklemek gerekmez.</DialogDescription>
        </DialogHeader>
        <AddPersonFlow key={`${n}-${tab}`} initialTab={tab} onClose={() => setOpen(false)} />
      </DialogContent>
    </Dialog>
  )
}
