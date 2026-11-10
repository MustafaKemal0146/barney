// Küçük bir SSE ayrıştırıcı. EventSource başlık (Authorization) gönderemediği için akış
// fetch ile okunur; bu sınıf parça sınırlarından bağımsız olarak olayları çıkarır.

export interface SSEMessage {
  id?: string
  event?: string
  data: string
}

export class SSEParser {
  private buf = ''
  private cur: { id?: string; event?: string; data: string[] } = { data: [] }

  /** Yeni gelen metni işler ve tamamlanan olayları döndürür. */
  feed(chunk: string): SSEMessage[] {
    this.buf += chunk
    const out: SSEMessage[] = []
    for (;;) {
      const m = /\r\n|\n|\r/.exec(this.buf)
      if (!m) break
      // Satır sonu tam görünmüyorsa (ör. "\r" ile biten parça) bir sonraki parçayı bekle.
      if (m[0] === '\r' && m.index + 1 === this.buf.length) break
      const line = this.buf.slice(0, m.index)
      this.buf = this.buf.slice(m.index + m[0].length)
      if (line === '') {
        if (this.cur.data.length > 0 || this.cur.event) {
          out.push({ id: this.cur.id, event: this.cur.event, data: this.cur.data.join('\n') })
        }
        this.cur = { data: [] }
        continue
      }
      if (line.startsWith(':')) continue // yorum / ping
      const i = line.indexOf(':')
      const field = i === -1 ? line : line.slice(0, i)
      let value = i === -1 ? '' : line.slice(i + 1)
      if (value.startsWith(' ')) value = value.slice(1)
      if (field === 'data') this.cur.data.push(value)
      else if (field === 'event') this.cur.event = value
      else if (field === 'id') this.cur.id = value
    }
    return out
  }
}
