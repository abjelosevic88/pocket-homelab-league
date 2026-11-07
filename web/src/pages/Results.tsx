// Every result of every group, newest first; "only mine" narrows it.
import { useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useLiveEvents, useOnChange } from '../lib/data'
import type { ResultItem } from '../lib/types'
import { MatchRow } from '../components/Bits'
import { Icon } from '../components/icons'
import { openCreate } from '../components/Layout'
import { Empty, Loading, Segmented, useAsync } from '../components/ui'

export default function Results() {
  const { user } = useApp()
  const [mine, setMine] = useState(false)
  const { data, reload } = useAsync(() => api.get<ResultItem[]>(`${V1}/matches?limit=200${mine ? `&player=${user?.id}` : ''}`), [mine])
  useOnChange(reload)
  useLiveEvents(e => { if (e.type === 'match') reload() })
  return (
    <div className="page stack loose">
      <div className="page-head"><h1 className="page-title grow">Results</h1>
        <Segmented size="sm" value={mine ? 'mine' : 'all'} onChange={v => setMine(v === 'mine')} options={[{ value: 'all', label: 'Everyone' }, { value: 'mine', label: 'Only mine' }]} />
        <button type="button" className="btn primary hide-sm" onClick={() => openCreate('result')}><Icon name="trophy" size={15} />Enter a result</button>
      </div>
      {!data ? <Loading /> : data.length === 0 ? <Empty icon="trophy" title="No results yet"><button type="button" className="btn primary" onClick={() => openCreate('result')}>Enter a result</button></Empty>
        : <div className="card flush">{data.map(r => <MatchRow key={r.match.id} m={r.match} format="singles" group={r.group_name} emoji={r.emoji} onChanged={reload} />)}</div>}
    </div>
  )
}
