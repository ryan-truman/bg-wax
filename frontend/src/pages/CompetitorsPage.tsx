import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Competitor } from '../types'

export default function CompetitorsPage() {
  const [competitors, setCompetitors] = useState<Competitor[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.getCompetitors()
      .then(setCompetitors)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="text-sm" style={{ color: '#888' }}>Loading…</p>

  return (
    <div className="max-w-2xl">
      <h2 className="text-xs uppercase tracking-widest mb-4" style={{ color: '#777' }}>
        Competitors — {competitors.length} registered
      </h2>
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
              <span className="text-xs tabular-nums shrink-0" style={{ color: 'var(--color-brand)' }}>{c.wins}W</span>
              <span className="text-xs tabular-nums shrink-0" style={{ color: '#888' }}>{c.losses}L</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
