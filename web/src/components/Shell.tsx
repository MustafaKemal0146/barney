import { useEffect, useRef, useState } from 'react'
import { Sidebar } from '@/components/shell/Sidebar'
import { Topbar } from '@/components/shell/Topbar'
import { StatusBar } from '@/components/shell/StatusBar'
import { Toast } from '@/components/shell/Toast'
import { EmbedBar } from '@/components/shell/EmbedBar'
import { ImportBar } from '@/components/ImportProgress'
import { LiveStrip } from '@/components/LiveStrip'
import { SetupWizard } from '@/components/SetupWizard'
import { AddPersonDialog } from '@/components/AddPerson'
import { PersonPage } from '@/pages/PersonPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { Empty, Loading } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { TooltipProvider } from '@/components/ui/tooltip'
import { isEmbed, navigate, personHref, useRoute } from '@/lib/route'
import { useApp } from '@/store/app'

function Home() {
  const people = useApp((s) => s.people)
  const openAdd = useApp((s) => s.openAdd)
  useEffect(() => {
    if (people.length) {
      const last = [...people].sort((a, b) => b.last_message_ts - a.last_message_ts)[0]
      navigate(personHref(last.id))
    }
  }, [people])
  return (
    <div className="flex h-full items-center justify-center p-6">
      <Empty>
        Bir kişi seç ya da yeni kişi ekle.
        <div className="mt-3"><Button onClick={() => openAdd()}>Kişi ekle</Button></div>
      </Empty>
    </div>
  )
}

export function Shell() {
  const route = useRoute()
  const embed = isEmbed()
  const state = useApp((s) => s.state)
  const stateError = useApp((s) => s.stateError)
  const dismissed = useApp((s) => s.setupDismissed)
  const loaded = useApp((s) => s.peopleLoaded)
  const needs = !!state && (!state.has_people || !state.key_set)
  // Sihirbaz bir kez açıldıysa (kişi eklenip özet görülürken) kapatılana dek açık kalır.
  // Yalnız ilk yüklemede karar verilir (sonradan anahtar silinince sihirbaz fırlamasın).
  const [wizard, setWizard] = useState(false)
  const decided = useRef(false)
  useEffect(() => {
    if (decided.current || !loaded || !state) return
    decided.current = true
    if (needs && !dismissed) setWizard(true)
  }, [needs, dismissed, loaded, state])
  const showWizard = wizard && !dismissed

  let body
  if (!state) body = stateError ? <div className="p-6"><Empty>Barney sunucusuna ulaşılamadı: {stateError}</Empty></div> : <Loading />
  else if (showWizard) body = <SetupWizard />
  else if (route.page === 'settings') body = <SettingsPage />
  else if (route.page === 'person') body = <PersonPage id={route.id} tab={route.tab} />
  else body = <Home />

  return (
    <TooltipProvider>
      <div className="flex h-dvh overflow-hidden bg-background text-foreground">
        {!embed && <Sidebar />}
        <div className="flex min-w-0 flex-1 flex-col">
          {embed ? <EmbedBar /> : <Topbar />}
          <ImportBar />
          {embed && route.page !== 'person' && <LiveStrip selectedId={null} />}
          <main className="min-h-0 flex-1 overflow-hidden bg-surface/40">{body}</main>
          {!embed && <StatusBar />}
        </div>
      </div>
      <AddPersonDialog />
      <Toast />
    </TooltipProvider>
  )
}
