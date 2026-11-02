import { useEffect, useRef, useState } from 'react'
import { ShieldCheck } from 'lucide-react'
import { errorText, onCoreEvent, rpc } from './backend'
import { Button, Dialog, Input } from './components/ui'
import type { Session } from './data'

type AuthPrompt = { id: number; sessionId: string; prompt: string; echo: boolean; informational: boolean }

export default function SshAuth({ enabled, sessions, onChange }: { enabled: boolean; sessions: Session[]; onChange: () => Promise<void> }) {
  const [prompts, setPrompts] = useState<AuthPrompt[]>([])
  const [error, setError] = useState('')
  const reload = useRef<() => Promise<void>>(async () => {})
  useEffect(() => {
    if (!enabled) { setPrompts([]); return }
    let alive = true
    let running = false
    let rerun = false
    let off: (() => void) | undefined
    const load = async () => {
      if (running) { rerun = true; return }
      running = true
      try {
        do {
          rerun = false
          const pending = await rpc<AuthPrompt[]>('sessions.authPending')
          if (alive) setPrompts(current => {
            const active = current[0]
            const first = active && pending.find(p => p.sessionId === active.sessionId)
            return first ? [first, ...pending.filter(p => p !== first)] : pending
          })
        } while (alive && rerun)
      } catch { /* The main application handles backend connectivity errors. */ }
      finally { running = false }
    }
    reload.current = load
    void load()
    const interval = setInterval(() => void load(), 1000)
    void onCoreEvent(event => { if (event.event === 'session.auth') void load() }).then(dispose => { if (alive) off = dispose; else dispose() }).catch(() => {})
    return () => { alive = false; clearInterval(interval); off?.(); reload.current = async () => {} }
  }, [enabled])
  const prompt = prompts[0]
  const isPassword = !!prompt && /(?:^|\s)password:\s*$/i.test(prompt.prompt)
  const isSms = !!prompt && /\bsms\b/i.test(prompt.prompt)
  const session = sessions.find(item => item.id === prompt?.sessionId)
  useEffect(() => setError(''), [prompt?.sessionId, prompt?.id])
  const [cancelling, setCancelling] = useState(false)
  async function cancel() {
    if (!prompt || cancelling) return
    setCancelling(true)
    try {
      await rpc('sessions.close', { id: prompt.sessionId })
      await reload.current()
      await onChange()
    } catch (error) { setError(errorText(error)) }
    finally { setCancelling(false) }
  }
  return <Dialog
    open={!!prompt}
    onOpenChange={open => { if (!open) void cancel() }}
    title={isPassword ? 'SSH 登录密码' : '第二步验证 · MFA / 2FA'}
    description={(session?.name ?? prompt?.sessionId ?? 'SSH') + ' · 按服务器提示完成认证。响应仅用于本次连接，不保存到配置或终端历史。'}
  >
    {prompt && <AuthForm
      key={prompt.sessionId + ':' + prompt.id}
      prompt={prompt}
      error={error}
      cancelling={cancelling}
      label={isPassword ? '登录密码' : isSms ? '短信验证码' : '验证码或认证响应'}
      cancel={() => void cancel()}
      submit={async response => {
        setError('')
        await rpc('sessions.authRespond', { id: prompt.sessionId, promptId: prompt.id, response })
        await reload.current()
      }}
    />}
  </Dialog>
}

function AuthForm({ prompt, label, error, cancelling, cancel, submit }: { prompt: AuthPrompt; label: string; error: string; cancelling: boolean; cancel: () => void; submit: (response: string) => Promise<void> }) {
  const [response, setResponse] = useState('')
  const [sending, setSending] = useState(false)
  const [failure, setFailure] = useState('')
  return <form className="connection-form" onSubmit={async event => {
    event.preventDefault()
    if (sending || cancelling) return
    setSending(true); setFailure('')
    const answer = response
    setResponse('')
    try { await submit(answer) }
    catch (error) { setFailure(errorText(error)); setSending(false) }
  }}>
    <div className="ssh-auth-prompt"><ShieldCheck size={18} aria-hidden="true" /><pre>{prompt.prompt}</pre></div>
    {!prompt.informational && <label className="form-field">
      {label}
      <Input autoFocus name="ssh-auth-response" type={prompt.echo ? 'text' : 'password'} autoComplete="one-time-code" value={response} onChange={event => setResponse(event.target.value)} maxLength={8192} disabled={sending || cancelling} spellCheck={false} autoCapitalize="none" placeholder="输入服务器要求的验证码、密码或选项" />
    </label>}
    <p className="ssh-auth-hint">支持动态验证码、备用码，以及 Duo 等服务的选项提示。手机推送请按服务器提示操作。认证等待时间为 5 分钟。</p>
    {(failure || error) && <p className="ssh-auth-error" role="alert">{failure || error}</p>}
    <div className="dialog-footer">
      <Button variant="outline" onClick={cancel} disabled={cancelling}>{cancelling ? '正在取消…' : '取消连接'}</Button>
      <Button type="submit" disabled={sending || cancelling}>{sending ? '正在提交…' : prompt.informational ? '继续' : '提交认证'}</Button>
    </div>
  </form>
}
