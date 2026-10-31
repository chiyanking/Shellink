import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createTerminalInput } from '../src/terminal-input.ts'

function deferred() {
  let resolve!: () => void
  let reject!: (error: Error) => void
  const promise = new Promise<void>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

test('sends immediately and preserves typing, paste, and control-key order', async () => {
  const calls: string[] = []
  const requests: ReturnType<typeof deferred>[] = []
  const input = createTerminalInput(text => {
    calls.push(text)
    const request = deferred()
    requests.push(request)
    return request.promise
  }, error => { throw error })

  input('p')
  assert.deepEqual(calls, ['p'])
  input('s')
  input(' -ef')
  input('\x1b[D')
  input('\r')
  assert.deepEqual(calls, ['p'])

  requests[0].resolve()
  await Promise.resolve()
  assert.deepEqual(calls, ['p', 's -ef\x1b[D\r'])
  input('中文')
  requests[1].resolve()
  await Promise.resolve()
  assert.deepEqual(calls, ['p', 's -ef\x1b[D\r', '中文'])
  requests[2].resolve()
  await Promise.resolve()
})

test('does not retry uncertain input after a failure and accepts fresh input', async () => {
  const calls: string[] = []
  const errors: unknown[] = []
  const request = deferred()
  const input = createTerminalInput(text => {
    calls.push(text)
    return calls.length === 1 ? request.promise : Promise.resolve()
  }, error => errors.push(error))

  input('a')
  input('b')
  const failure = new Error('write failed')
  request.reject(failure)
  await Promise.resolve()
  assert.deepEqual(calls, ['a'])
  assert.deepEqual(errors, [failure])
  input('c')
  assert.deepEqual(calls, ['a', 'c'])
  await Promise.resolve()
})
