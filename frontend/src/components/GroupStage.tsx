import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Group } from '../types'
import GroupCard from './GroupCard'

interface Props {
  // Once the knockout is under way the group stage is history: it drops to the
  // foot of the page as a collapsed heading, the way completed matches do, so
  // the final standings stay available without crowding out the bracket.
  collapsible?: boolean
}

export default function GroupStage({ collapsible = false }: Props) {
  const [groups, setGroups] = useState<Group[]>([])
  const [expanded, setExpanded] = useState(false)

  useEffect(() => {
    let active = true
    const load = () =>
      api.getGroups()
        .then(g => { if (active) setGroups(g) })
        .catch(() => {})
    load()
    // Group results are locked once the knockout starts, so the archived view
    // is fetched once rather than polled all evening.
    if (collapsible) return () => { active = false }
    const timer = setInterval(load, 5000)
    return () => { active = false; clearInterval(timer) }
  }, [collapsible])

  if (!groups.length) return null

  const collapsed = collapsible && !expanded
  const heading = (
    <h2 className="text-xs uppercase tracking-widest" style={{ color: '#777' }}>Group Stage</h2>
  )

  return (
    <section>
      {collapsible ? (
        <button
          onClick={() => setExpanded(e => !e)}
          className={`flex items-center gap-3 w-full text-left ${collapsed ? '' : 'mb-6'}`}
        >
          <span className="text-base leading-none" style={{ color: '#888' }}>
            {collapsed ? '▸' : '▾'}
          </span>
          {heading}
        </button>
      ) : (
        <div className="mb-6">{heading}</div>
      )}
      {!collapsed && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {groups.map(g => <GroupCard key={g.id} group={g} />)}
        </div>
      )}
    </section>
  )
}
