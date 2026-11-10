/** Tarayıcı bildirimi: izin verilmişse ve sekme görünmüyorsa (ya da odakta değilse) gösterir. */
export function canNotify(): boolean {
  return typeof Notification !== 'undefined' && Notification.permission === 'granted'
}
export async function askPermission(): Promise<NotificationPermission | 'unsupported'> {
  if (typeof Notification === 'undefined') return 'unsupported'
  if (Notification.permission !== 'default') return Notification.permission
  try {
    return await Notification.requestPermission()
  } catch {
    return 'denied'
  }
}
export function pageHidden(): boolean {
  return document.hidden || !document.hasFocus()
}
export function fireNotification(title: string, body: string) {
  if (!canNotify()) return
  try {
    new Notification(title, { body, tag: 'barney' })
  } catch {
    /* bazı tarayıcılar yapıcıyı engeller */
  }
}
