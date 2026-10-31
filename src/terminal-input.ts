// Send the first keystroke immediately, then combine input arriving during an
// outstanding request. Only one request may be in flight so keys cannot race.
export function createTerminalInput(
  send: (text: string) => Promise<unknown>,
  onError: (error: unknown) => void,
) {
  let pending = ''
  let sending = false

  async function flush() {
    sending = true
    while (pending) {
      const text = pending
      pending = ''
      try {
        await send(text)
      } catch (error) {
        // A failed request may already have written to the PTY; never retry it.
        pending = ''
        onError(error)
        break
      }
    }
    sending = false
  }

  return (text: string) => {
    pending += text
    if (!sending) void flush()
  }
}
