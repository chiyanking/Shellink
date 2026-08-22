import fs from 'node:fs'
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { AppError, FrameDecoder, FrameKind, PROTOCOL_VERSION, RpcErrorCode, encodeFrame, type RpcEvent } from '@shellink/protocol'
import { SocketClient } from '../src/SocketClient.js'
import { defaultSocketClientMessages } from '../src/messages.js'

interface PendingRequest { id: number; method: string; params: unknown }

async function createFakeDaemon(handler: (request: PendingRequest, reply: (ok: boolean, payload: unknown) => void, pushEvent: (event: RpcEvent) => void, writeRaw: (chunk: Buffer) => void) => void): Promise<{ socketPath: string; close: () => Promise<void>; dropConnections: () => void }> {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'shellink-client-'))
  const socketPath = path.join(dir, 'shellink.sock')
  const decoder = new FrameDecoder()
  const sockets = new Set<net.Socket>()
  const server = net.createServer((socket) => {
    sockets.add(socket)
    socket.on('close', () => sockets.delete(socket))
    const reply = (id: number, ok: boolean, payload: unknown) => {
      const frame = ok
        ? encodeFrame(FrameKind.Response, { id, ok: true, result: payload })
        : encodeFrame(FrameKind.Response, { id, ok: false, error: payload })
      socket.write(frame)
    }
    const pushEvent = (event: RpcEvent) => {
      socket.write(encodeFrame(FrameKind.Event, event))
    }
    socket.on('data', (chunk) => {
      for (const frame of decoder.push(typeof chunk === 'string' ? Buffer.from(chunk) : chunk)) {
        if (frame.kind !== FrameKind.Request) continue
        const request = frame.payload as { id: number; method: string; params: unknown }
        if (request.method === 'system.hello') {
          reply(request.id, true, {
            serviceVersion: 'test',
            serviceCommit: 'testhash',
            protocolVersion: PROTOCOL_VERSION,
            capabilities: [],
          })
          continue
        }
        handler(request, (ok, payload) => reply(request.id, ok, payload), pushEvent, (chunk) => socket.write(chunk))
      }
    })
  })
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject)
    server.listen(socketPath, () => resolve())
  })
  return {
    socketPath,
    dropConnections: () => {
      for (const socket of sockets) socket.destroy()
    },
    close: async () => {
      for (const socket of sockets) socket.destroy()
      await new Promise<void>((resolve) => server.close(() => resolve()))
      fs.rmSync(dir, { recursive: true, force: true })
    },
  }
}

const daemons: Array<{ close: () => Promise<void> }> = []

afterEach(async () => {
  while (daemons.length > 0) await daemons.pop()!.close()
})

describe('SocketClient', () => {
  it('connects, sends hello, and resolves a request', async () => {
    const daemon = await createFakeDaemon((request, reply) => {
      expect(request.method).toBe('profiles.list')
      reply(true, [{ id: 'p1', name: 'local' }])
    })
    daemons.push(daemon)
    const client = new SocketClient(daemon.socketPath, 2000)
    const hello = await client.connect()
    expect(hello.protocolVersion).toBe(PROTOCOL_VERSION)
    const profiles = await client.request<Array<{ id: string }>>('profiles.list', { q: 'local' })
    expect(profiles).toEqual([{ id: 'p1', name: 'local' }])
    client.close()
  })

  it('reconnects hello on an already-open socket', async () => {
    const daemon = await createFakeDaemon(() => undefined)
    daemons.push(daemon)
    const client = new SocketClient(daemon.socketPath, 2000)
    const first = await client.connect()
    const second = await client.connect()
    expect(first.serviceVersion).toBe('test')
    expect(second.serviceVersion).toBe('test')
    client.close()
  })

  it('rejects a request when not connected', async () => {
    const client = new SocketClient('/tmp/missing-shellink.sock', 200)
    await expect(client.request('system.ping')).rejects.toMatchObject({
      code: RpcErrorCode.UNAVAILABLE,
      message: defaultSocketClientMessages.daemonNotConnected,
    })
  })

  it('times out a pending request', async () => {
    const daemon = await createFakeDaemon(() => undefined)
    daemons.push(daemon)
    const client = new SocketClient(daemon.socketPath, 50)
    await client.connect()
    await expect(client.request('sessions.list', {}, 40)).rejects.toMatchObject({
      code: RpcErrorCode.TIMEOUT,
      message: 'sessions.list request timed out',
    })
    client.close()
  })

  it('rejects pending requests when the daemon closes', async () => {
    let drop: (() => void) | undefined
    const daemon = await createFakeDaemon((_request, _reply, _push) => {
      drop?.()
    })
    daemons.push(daemon)
    const client = new SocketClient(daemon.socketPath, 2000)
    await client.connect()
    const pending = client.request('sessions.list')
    await new Promise<void>((resolve) => {
      drop = () => {
        client.close()
        resolve()
      }
      void client.request('sessions.state', { id: 'x' }).catch(() => undefined)
    })
    await expect(pending).rejects.toBeInstanceOf(AppError)
  })

  it('ignores a response for an unknown id and a non-response frame', async () => {
    const daemon = await createFakeDaemon((request, reply, pushEvent, writeRaw) => {
      pushEvent({ subscriptionId: 'none', event: 'session.state', data: {} })
      writeRaw(encodeFrame(FrameKind.Request, { id: 1, method: 'noop', params: {} }))
      writeRaw(encodeFrame(FrameKind.Response, { id: 99999, ok: true, result: { stray: true } }))
      if (request.method === 'system.ping') {
        reply(true, { ok: true })
      }
    })
    daemons.push(daemon)
    const client = new SocketClient(daemon.socketPath, 2000)
    await client.connect()
    await expect(client.request('system.ping')).resolves.toEqual({ ok: true })
    client.close()
  })

  it('fails pending requests when the server drops the connection', async () => {
    const daemon = await createFakeDaemon(() => undefined)
    daemons.push(daemon)
    const client = new SocketClient(daemon.socketPath, 2000)
    await client.connect()
    const disconnected = new Promise<void>((resolve) => client.once('disconnected', () => resolve()))
    const pending = client.request('sessions.list')
    daemon.dropConnections()
    await disconnected
    await expect(pending).rejects.toMatchObject({
      code: RpcErrorCode.UNAVAILABLE,
      message: defaultSocketClientMessages.daemonClosed,
    })
    client.close()
  })

  it('maps a server error onto AppError', async () => {
    const daemon = await createFakeDaemon((request, reply) => {
      reply(false, { code: RpcErrorCode.NOT_FOUND, message: 'missing', status: 404 })
    })
    daemons.push(daemon)
    const client = new SocketClient(daemon.socketPath, 2000)
    await client.connect()
    await expect(client.request('profiles.get', { id: 'nope' })).rejects.toMatchObject({
      code: RpcErrorCode.NOT_FOUND,
      message: 'missing',
      status: 404,
    })
    client.close()
  })

  it('delivers subscribed events and unsubscribes', async () => {
    const daemon = await createFakeDaemon((request, reply, pushEvent) => {
      if (request.method === 'events.subscribe') {
        reply(true, { subscriptionId: 'sub-1', initial: { sessions: [] } })
        return
      }
      if (request.method === 'sessions.state') {
        pushEvent({ subscriptionId: 'sub-1', event: 'session.state', data: { sessionId: 's1', state: 'WAITING_INPUT' } })
        pushEvent({ subscriptionId: 'other', event: 'session.state', data: { sessionId: 'ignored' } })
        reply(true, { id: 's1', state: 'WAITING_INPUT' })
        return
      }
      if (request.method === 'events.unsubscribe') {
        reply(true, { ok: true })
      }
    })
    daemons.push(daemon)
    const client = new SocketClient(daemon.socketPath, 2000)
    await client.connect()
    const events: RpcEvent[] = []
    const sub = await client.subscribe({ sessionId: 's1' }, (event) => events.push(event))
    expect(sub.subscriptionId).toBe('sub-1')
    await client.request('sessions.state', { id: 's1' })
    expect(events).toEqual([
      { subscriptionId: 'sub-1', event: 'session.state', data: { sessionId: 's1', state: 'WAITING_INPUT' } },
    ])
    await sub.unsubscribe()
    client.close()
  })

  it('uses injected messages for a missing connection', async () => {
    const client = new SocketClient('/tmp/missing-shellink.sock', 200, {
      daemonClosed: 'closed-zh',
      daemonNotConnected: 'not-connected-zh',
      requestTimedOut: (method) => `timeout-${method}`,
    })
    await expect(client.request('system.ping')).rejects.toMatchObject({ message: 'not-connected-zh' })
  })

  it('fails pending requests on a socket error', async () => {
    const daemon = await createFakeDaemon(() => undefined)
    daemons.push(daemon)
    const client = new SocketClient(daemon.socketPath, 2000)
    await client.connect()
    const pending = client.request('sessions.list')
    const socket = (client as unknown as { socket: net.Socket }).socket
    socket.emit('error', new Error('boom'))
    await expect(pending).rejects.toMatchObject({ message: 'boom' })
    client.close()
  })

  it('fails all pending requests on a malformed frame', async () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'shellink-client-bad-'))
    const socketPath = path.join(dir, 'shellink.sock')
    const decoder = new FrameDecoder()
    const server = net.createServer((socket) => {
      socket.on('data', (chunk) => {
        for (const frame of decoder.push(typeof chunk === 'string' ? Buffer.from(chunk) : chunk)) {
          if (frame.kind !== FrameKind.Request) continue
          const request = frame.payload as { id: number; method: string }
          if (request.method === 'system.hello') {
            socket.write(encodeFrame(FrameKind.Response, {
              id: request.id,
              ok: true,
              result: { serviceVersion: 'test', serviceCommit: 'x', protocolVersion: PROTOCOL_VERSION, capabilities: [] },
            }))
            continue
          }
          socket.write(Buffer.from('INVALIDMAGIC!'))
        }
      })
    })
    await new Promise<void>((resolve) => server.listen(socketPath, () => resolve()))
    const client = new SocketClient(socketPath, 2000)
    await client.connect()
    await expect(client.request('sessions.list')).rejects.toBeInstanceOf(Error)
    client.close()
    await new Promise<void>((resolve) => server.close(() => resolve()))
    fs.rmSync(dir, { recursive: true, force: true })
  })
})
