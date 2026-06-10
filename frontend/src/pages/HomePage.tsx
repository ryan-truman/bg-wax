import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Tournament, Competitor, Group } from '../types'
import GroupCard from '../components/GroupCard'
import BracketView from '../components/BracketView'

interface Props {
  tournament: Tournament | null
  onUpdate: (t: Tournament) => void
}

export default function HomePage({ tournament }: Props) {
  const [competitors, setCompetitors] = useState<Competitor[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    async function load() {
      try {
        if (!tournament) return
        if (tournament.status === 'setup') {
          setCompetitors(await api.getCompetitors())
        } else if (tournament.status === 'group_stage') {
          setGroups(await api.getGroups())
        }
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [tournament?.status])

  if (!tournament) {
    return (
      <div className="flex flex-col items-center justify-center py-32 gap-3 text-center">
        <p className="text-2xl font-bold tracking-widest uppercase" style={{ color: '#444' }}>No Tournament</p>
        <p className="text-sm" style={{ color: '#888' }}>Import competitors from Ticket Tailor to get started.</p>
        <Link to="/settings" className="mt-2 text-sm transition-colors" style={{ color: 'var(--color-brand)' }}>
          Go to Settings →
        </Link>
      </div>
    )
  }

  if (loading) {
    return <p className="text-sm" style={{ color: '#888' }}>Loading…</p>
  }

  if (tournament.status === 'setup') {
    return (
      <div className="max-w-2xl">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-xs uppercase tracking-widest" style={{ color: '#777' }}>
            Competitors — {competitors.length} registered
          </h2>
          <Link
            to="/settings"
            className="text-xs font-bold uppercase tracking-widest px-3 py-1.5 rounded transition-colors"
            style={{ backgroundColor: 'var(--color-brand)', color: '#fff' }}
          >
            Run Draw →
          </Link>
        </div>
        <div className="rounded border divide-y" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface-card)' }}>
          {competitors.length === 0 ? (
            <p className="px-4 py-6 text-sm text-center" style={{ color: '#888' }}>
              No competitors yet — import from Ticket Tailor in Settings.
            </p>
          ) : (
            competitors.map((c, i) => (
              <div key={c.id} className="flex items-center gap-4 px-4 py-2.5">
                <span className="text-xs tabular-nums w-6 text-right shrink-0" style={{ color: '#555' }}>
                  {i + 1}
                </span>
                <span className="text-sm flex-1">{c.name}</span>
                {c.email && (
                  <span className="text-xs hidden sm:block" style={{ color: '#666' }}>{c.email}</span>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    )
  }

  if (tournament.status === 'group_stage') {
    return (
      <div>
        <h2 className="text-xs uppercase tracking-widest mb-6" style={{ color: '#777' }}>Group Stage</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {groups.map(g => (
            <GroupCard key={g.id} group={g} />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div>
      <h2 className="text-xs uppercase tracking-widest mb-6" style={{ color: '#777' }}>
        {tournament.status === 'complete' ? 'Final Results' : 'Knockout Bracket'}
      </h2>
      <BracketView />
    </div>
  )
}
