import { describe, expect, it, vi } from 'vitest'
import { ShellinkHttpClient } from '../src/client.js'

describe('ShellinkHttpClient', () => {
  it('maps plugin requests to the Go REST API', async () => {
    const calls: Array<{ url: string; init: RequestInit }> = []
    const fetchImpl = vi.fn<typeof fetch>(async (input, init = {}) => {
      calls.push({ url: String(input), init })
      return Response.json({ ok: true })
    })
    const client = new ShellinkHttpClient('http://127.0.0.1:7070', 1000, fetchImpl)

    await client.request('profiles.update', { id: 'p/1', profile: { host: 'example.com' } })
    await client.request('sessions.execStatus', { jobId: 'job/1', since: 4, waitMs: 100 })
    await client.request('sessions.close', { id: 's/1' })

    expect(calls.map(({ url, init }) => [url, init.method])).toEqual([
      ['http://127.0.0.1:7070/shellink/api/profiles/p%2F1', 'PUT'],
      ['http://127.0.0.1:7070/shellink/api/sessions/job%2F1/exec-status', 'POST'],
      ['http://127.0.0.1:7070/shellink/api/sessions/s%2F1/close', 'POST'],
    ])
    expect(JSON.parse(String(calls[0]?.init.body))).toEqual({ host: 'example.com' })
    expect(JSON.parse(String(calls[1]?.init.body))).toEqual({ jobId: 'job/1', since: 4, waitMs: 100 })
  })

  it('sends file uploads as multipart form data', async () => {
    let requestBody: BodyInit | null | undefined
    const fetchImpl = vi.fn<typeof fetch>(async (_input, init = {}) => {
      requestBody = init.body
      return Response.json({ id: 'job1' })
    })
    const client = new ShellinkHttpClient('http://127.0.0.1:7070', 1000, fetchImpl)

    await client.request('sessions.uploadStart', {
      id: 's1',
      path: '/tmp/a.txt',
      data: new Uint8Array([1, 2, 3]),
      timeoutMs: 5000,
    })

    expect(String(fetchImpl.mock.calls[0]?.[0])).toBe(
      'http://127.0.0.1:7070/shellink/api/sessions/s1/upload-start?path=%2Ftmp%2Fa.txt&timeoutMs=5000',
    )
    expect(requestBody).toBeInstanceOf(FormData)
    const file = (requestBody as FormData).get('file') as File
    expect(file.size).toBe(3)
    expect([...new Uint8Array(await file.arrayBuffer())]).toEqual([1, 2, 3])
  })

  it('returns empty success bodies and reports HTTP errors', async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () => new Response(null, { status: 204 }))
    const client = new ShellinkHttpClient('http://127.0.0.1:7070', 1000, fetchImpl)
    await expect(client.request('profiles.delete', { id: 'p1' })).resolves.toEqual({ ok: true })
    client.close()
    await expect(client.request('sessions.list')).rejects.toThrow(/closed/)

    const failing = new ShellinkHttpClient(
      'http://127.0.0.1:7070',
      1000,
      vi.fn<typeof fetch>(async () => Response.json({ error: 'profile missing' }, { status: 404 })),
    )
    await expect(failing.request('profiles.get', { id: 'missing' })).rejects.toMatchObject({
      name: 'ShellinkHttpError',
      status: 404,
      message: 'profile missing',
    })
  })
})
