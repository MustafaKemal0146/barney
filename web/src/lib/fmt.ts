import type { Person } from './types'

export const initial = (name: string) => (name.trim()[0] ?? '?').toLocaleUpperCase('tr-TR')

/** Kısa göreli zaman: "şimdi", "5 dk", "3 sa", "dün", "4 gün", tarih. Zaman damgası unix saniye. */
export function relTime(ts: number, now = Date.now() / 1000): string {
  if (!ts) return ''
  const d = Math.max(0, now - ts)
  if (d < 60) return 'şimdi'
  if (d < 3600) return `${Math.floor(d / 60)} dk`
  if (d < 86400) return `${Math.floor(d / 3600)} sa`
  if (d < 2 * 86400) return 'dün'
  if (d < 14 * 86400) return `${Math.floor(d / 86400)} gün`
  return new Date(ts * 1000).toLocaleDateString('tr-TR', { day: 'numeric', month: 'short' })
}

export function clock(ts: number): string {
  return new Date(ts * 1000).toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit' })
}
export function dateTime(ts: number): string {
  return new Date(ts * 1000).toLocaleString('tr-TR', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
}

/** Gerçek sessizlik gün sayısı (son mesajdan beri). */
export function silenceDays(p: Pick<Person, 'last_message_ts'>, now = Date.now() / 1000): number {
  if (!p.last_message_ts) return 0
  return Math.floor(Math.max(0, now - p.last_message_ts) / 86400)
}
/** Person.silence_days eşiktir (0 = kapalı); eşik aşıldıysa rozet gösterilir. */
export function silenceExceeded(p: Person, now = Date.now() / 1000): boolean {
  return p.silence_days > 0 && silenceDays(p, now) >= p.silence_days
}

export function secs(n: number): string {
  if (n < 90) return `${Math.round(n)} sn`
  if (n < 5400) return `${Math.round(n / 60)} dk`
  return `${(n / 3600).toFixed(1).replace('.', ',')} sa`
}

export const pct = (n: number) => `%${Math.round(n)}`
export const num = (n: number) => n.toLocaleString('tr-TR')

export const GOALS: { value: string; label: string }[] = [
  { value: '', label: 'Belirsiz' },
  { value: 'flort', label: 'Flört' },
  { value: 'arkadaslik', label: 'Arkadaşlık' },
  { value: 'is', label: 'İş' },
  { value: 'aile', label: 'Aile' },
  { value: 'barisma', label: 'Barışma' },
]
export const STAGES: { value: string; label: string }[] = [
  { value: '', label: 'Belirsiz' },
  { value: 'tanisma', label: 'Tanışma' },
  { value: 'yakinlasma', label: 'Yakınlaşma' },
  { value: 'yakin', label: 'Yakın' },
  { value: 'gerilim', label: 'Gerilim' },
  { value: 'soguma', label: 'Soğuma' },
]
export const TURS: Record<string, string> = { tek: 'Tek', guvenli: 'Güvenli', komik: 'Komik', cesur: 'Cesur' }
