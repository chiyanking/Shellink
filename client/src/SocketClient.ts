import { EventEmitter } from 'node:events'
import net from 'node:net'
import { AppError, FrameDecoder, FrameKind, PROTOCOL_VERSION, RpcErrorCode, VERSION, encodeFrame, type HelloResult, type RpcEvent, type RpcResponse } from '@shellink/protocol'
import { defaultSocketClientMessages, type SocketClientMessages } from './messages.js'

interface Pending { resolve: (value: unknown) => void; reject: (error: Error) => void; timer: NodeJS.Timeout }

/** Live event subscription returned by {@link SocketClient.subscribe}. */
export interface ClientSubscription {
  subscriptionId: string
  initial: unknown
  detach(): void
  unsubscribe(): Promise<unknown>
}

/** Unix-socket MessagePack RPC client for a Shellink daemon. */
export class SocketClient extends EventEmitter {
  private socket: net.Socket | null = null
  private readonly decoder = new FrameDecoder()
  private readonly pending = new Map<number, Pending>()
  private nextId = 1
  private readonly messages: SocketClientMessages

  constructor(
    private readonly socketPath: string,
    private readonly defaultTimeoutMs = 30_000,
    messages: SocketClientMessages = defaultSocketClientMessages,
  ) {
    super()
    this.messages = messages
  }

  async connect(helloTimeoutMs = this.defaultTimeoutMs): Promise<HelloResult> {
    if (this.socket && !this.socket.destroyed) {
      return await this.request('system.hello', { protocolVersion: PROTOCOL_VERSION, clientVersion: VERSION }, helloTimeoutMs) as HelloResult
    }
    const socket = net.createConnection(this.socketPath)
    this.socket = socket
    socket.on('data', (chunk) => this.onData(typeof chunk === 'string' ? Buffer.from(chunk) : chunk))
    socket.on('error', (error) => this.failAll(error))
    socket.on('close', () => {
      if (this.socket === socket) this.socket = null
      this.failAll(new AppError(RpcErrorCode.UNAVAILABLE, this.messages.daemonClosed, 503))
      this.emit('disconnected')
    })
    await new Promise<void>((resolve, reject) => {
      const onConnect = () => { socket.off('error', onError); resolve() }
      const onError = (error: Error) => { socket.off('connect', onConnect); reject(error) }
      socket.once('connect', onConnect); socket.once('error', onError)
    })
    return await this.request('system.hello', { protocolVersion: PROTOCOL_VERSION, clientVersion: VERSION }, helloTimeoutMs) as HelloResult
  }

  request<T = unknown>(method: string, params: unknown = {}, timeoutMs = this.defaultTimeoutMs): Promise<T> {
    if (!this.socket || this.socket.destroyed) {
      return Promise.reject(new AppError(RpcErrorCode.UNAVAILABLE, this.messages.daemonNotConnected, 503))
    }
    const id = this.nextId++
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(new AppError(RpcErrorCode.TIMEOUT, this.messages.requestTimedOut(method), 504))
      }, timeoutMs)
      this.pending.set(id, { resolve: resolve as (value: unknown) => void, reject, timer })
      this.socket!.write(encodeFrame(FrameKind.Request, { id, method, params }))
    })
  }

  async subscribe(params: { sessionId?: string; replay?: boolean }, listener: (event: RpcEvent) => void): Promise<ClientSubscription> {
    const result = await this.request<{ subscriptionId: string; initial: unknown }>('events.subscribe', params)
    const handler = (event: RpcEvent) => { if (event.subscriptionId === result.subscriptionId) listener(event) }
    this.on('event', handler)
    const detach = () => this.off('event', handler)
    return {
      ...result,
      detach,
      unsubscribe: async () => { detach(); await this.request('events.unsubscribe', { subscriptionId: result.subscriptionId }) },
    }
  }

  close(): void { this.socket?.end(); this.socket = null }

  private onData(chunk: Uint8Array): void {
    try {
      for (const frame of this.decoder.push(chunk)) {
        if (frame.kind === FrameKind.Event) { this.emit('event', frame.payload as RpcEvent); continue }
        if (frame.kind !== FrameKind.Response) continue
        const response = frame.payload as RpcResponse
        const pending = this.pending.get(Number(response.id))
        if (!pending) continue
        clearTimeout(pending.timer); this.pending.delete(Number(response.id))
        if (response.ok) pending.resolve(response.result)
        else pending.reject(new AppError(response.error.code, response.error.message, response.error.status, response.error.details))
      }
    } catch (error) { this.socket?.destroy(); this.failAll(error instanceof Error ? error : new Error(String(error))) }
  }

  private failAll(error: Error): void {
    for (const pending of this.pending.values()) { clearTimeout(pending.timer); pending.reject(error) }
    this.pending.clear()
  }
}
