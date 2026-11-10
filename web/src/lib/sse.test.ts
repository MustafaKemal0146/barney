import { describe, expect, it } from 'vitest'
import { SSEParser } from './sse'

describe('SSEParser', () => {
  it('parçalara bölünmüş olayı birleştirir', () => {
    const p = new SSEParser()
    expect(p.feed('data: {"type":"sta')).toEqual([])
    const out = p.feed('te"}\n\n')
    expect(out).toHaveLength(1)
    expect(JSON.parse(out[0].data)).toEqual({ type: 'state' })
  })
  it('yorum satırlarını ve CRLF’yi işler, çok satırlı data birleşir', () => {
    const p = new SSEParser()
    const out = p.feed(': ping\r\ndata: a\r\ndata: b\r\n\r\ndata: c\n\n')
    expect(out.map((m) => m.data)).toEqual(['a\nb', 'c'])
  })
  it('event ve id alanlarını okur', () => {
    const out = new SSEParser().feed('id: 7\nevent: x\ndata: 1\n\n')
    expect(out[0]).toMatchObject({ id: '7', event: 'x', data: '1' })
  })
})
