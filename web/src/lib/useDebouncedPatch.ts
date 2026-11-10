import { useCallback, useEffect, useRef } from 'react'
import { useApp } from '@/store/app'
import type { PersonPatch } from './types'

/** Aynı kişiye ait ardışık PATCH'leri birleştirip `ms` sonra tek istek olarak yollar; çıkışta boşaltır. */
export function useDebouncedPatch(personId: string, ms = 600) {
  const pending = useRef<PersonPatch>({})
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const idRef = useRef(personId)
  idRef.current = personId

  const flush = useCallback(() => {
    if (timer.current) clearTimeout(timer.current)
    timer.current = null
    const p = pending.current
    pending.current = {}
    if (Object.keys(p).length) void useApp.getState().patchPerson(idRef.current, p)
  }, [])

  const schedule = useCallback(
    (patch: PersonPatch) => {
      pending.current = { ...pending.current, ...patch }
      if (timer.current) clearTimeout(timer.current)
      timer.current = setTimeout(flush, ms)
    },
    [flush, ms],
  )
  useEffect(() => flush, [flush, personId])
  return { schedule, flush }
}
