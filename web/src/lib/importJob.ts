import type { AppState, HistoryPrefs, ImportJob, ImportMode, ImportOpts } from './types'

export const MAX_BULK = 10
export const DEFAULT_HISTORY: HistoryPrefs = { max_messages: 3000, max_seconds: 300, all: false }

export const isActive = (j: ImportJob | null | undefined): boolean =>
  !!j && (j.state === 'queued' || j.state === 'running')
export const isTerminal = (j: ImportJob | null | undefined): boolean => !!j && !isActive(j)

export const STATE_LABEL: Record<ImportJob['state'], string> = {
  queued: 'Sırada',
  running: 'Okunuyor',
  done: 'Bitti',
  failed: 'Başarısız',
  canceled: 'İptal edildi',
}

/** Ayarlardaki (State.history) derinlikten içe aktarma seçeneği üretir. */
export function optsFrom(state: Pick<AppState, 'history'> | null | undefined, mode: ImportMode = 'reader'): ImportOpts {
  return { ...DEFAULT_HISTORY, ...(state?.history ?? {}), mode }
}

/**
 * SSE `import` olayını önceki işle karşılaştırır. `finished`, işin bitiş durumunu İLK gördüğümüz
 * olayda dolu gelir (aynı bitiş olayı tekrarlanırsa null); toast/yönlendirme böylece bir kez tetiklenir.
 * `GET /api/import/active` ile yüklenen işler bu yoldan geçmez, yani yenilemede yönlendirme olmaz.
 */
export function importTransition(prev: ImportJob | null, next: ImportJob): { job: ImportJob; finished: ImportJob | null } {
  const seenTerminal = !!prev && prev.id === next.id && isTerminal(prev)
  return { job: next, finished: isTerminal(next) && !seenTerminal ? next : null }
}
