import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { Tournament, Group } from '../types'
import GroupCard from '../components/GroupCard'
import BracketView from '../components/BracketView'

interface Props {
  tournament: Tournament | null
}

export default function HomePage({ tournament }: Props) {
  const [groups, setGroups] = useState<Group[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    async function load() {
      try {
        if (!tournament) return
        if (tournament.status === 'group_stage') {
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
      <div className="flex flex-col items-center justify-center py-32 gap-3 text-center">
        <p className="text-2xl font-bold tracking-widest uppercase" style={{ color: '#444' }}>Setup</p>
        <p className="text-sm" style={{ color: '#888' }}>Tournament not yet started. Run the draw to begin the group stage.</p>
        <Link to="/matches" className="mt-2 text-sm transition-colors" style={{ color: 'var(--color-brand)' }}>
          Go to Match History →
        </Link>
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

  return <BracketView />
}
