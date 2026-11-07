import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { pushOff, pushOn, pushState, type PushState } from '../lib/push'
import { Icon } from './icons'
import { SettingRow, Switch, useToast } from './ui'

const WORDS: Record<PushState, string> = {
  unsupported: 'This browser can’t show League’s notifications.',
  'ios-home': 'On an iPhone: Share → “Add to Home Screen”, open League from there, then switch this on.',
  denied: 'Notifications are blocked for League in this browser’s settings. Allow them there, then switch this on.',
  off: 'New games, reminders, results and the chat pop up on this phone, even when League is closed.',
  on: 'This phone gets League’s notifications.',
}

/** The “On this phone” switch (Settings → Notifications). */
export function PushRow() {
  const [st, setSt] = useState<PushState | null>(null)
  const [busy, setBusy] = useState(false)
  const [test, setTest] = useState('')
  const { toast } = useToast()
  useEffect(() => { pushState().then(setSt) }, [])
  if (!st) return null
  const flip = async (v: boolean) => {
    setBusy(true)
    try { setSt(v ? await pushOn() : await pushOff()) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const send = async () => {
    setTest('sending…')
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/push/test`); setTest(r.ok ? `✓ ${r.message}` : `✕ ${r.error}`) } catch (e) { setTest(`✕ ${(e as Error).message}`) }
  }
  return (
    <>
      <SettingRow title="On this phone" help={WORDS[st]}>
        <Switch checked={st === 'on'} onChange={flip} label="On this phone" disabled={busy || st === 'unsupported' || st === 'ios-home' || st === 'denied'} />
      </SettingRow>
      {st === 'on' && <div className="row"><button type="button" className="btn sm" onClick={send}><Icon name="bell" size={14} />Send a test</button><span className={`small ${test.startsWith('✓') ? 'ok' : test.startsWith('✕') ? 'bad' : 'muted'}`}>{test}</span></div>}
    </>
  )
}

const SNOOZE = 'plg-push-asked'

/** A one-time nudge on Home: “Get notifications on this phone?” */
export function PushNudge() {
  const [st, setSt] = useState<PushState | null>(null)
  const [hidden, setHidden] = useState(() => localStorage.getItem(SNOOZE) === '1')
  useEffect(() => { if (!hidden) pushState().then(setSt) }, [hidden])
  if (hidden || !st || st === 'on' || st === 'unsupported' || st === 'denied') return null
  const close = () => { localStorage.setItem(SNOOZE, '1'); setHidden(true) }
  return (
    <div className="card push-nudge">
      <span className="set-app-ico"><Icon name="bell" size={17} /></span>
      <div className="grow">
        <b>Get notifications on this phone?</b>
        <p className="small muted">{st === 'ios-home' ? WORDS['ios-home'] : 'When a game is planned, the day before you play, when a result needs checking and when someone writes in the chat.'}</p>
      </div>
      <div className="row">
        {st === 'off' && <button type="button" className="btn primary sm" onClick={async () => { const r = await pushOn().catch(() => 'off' as PushState); setSt(r); if (r === 'on') close() }}>Turn on</button>}
        <button type="button" className="btn ghost sm" onClick={close}>Not now</button>
      </div>
    </div>
  )
}
