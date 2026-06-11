import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Match } from '../types'

export default function MatchHistoryPage() {
  const [matches, setMatches] = useState<Match[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.getMatches()
      .then(setMatches)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  function handleUpdate(updated: Match) {
    setMatches(prev => prev.map(m => m.id === updated.id ? updated : m))
  }

  if (loading) return <p className="text-sm" style={{ color: '#888' }}>Loading…</p>

  const inProgress = matches.filter(m => m.status === 'in_progress')
  const upcoming = matches.filter(m => m.status === 'pending')
  const completed = matches.filter(m => m.status === 'complete')

  if (!matches.length) {
    return <p className="text-sm" style={{ color: '#888' }}>No matches yet.</p>
  }

  return (
    <div className="max-w-2xl space-y-8">
      {inProgress.length > 0 && (
        <MatchSection
          title="In Progress"
          count={inProgress.length}
          matches={inProgress}
          onUpdate={handleUpdate}
          accentColor="var(--color-brand)"
        />
      )}
      {upcoming.length > 0 && (
        <MatchSection
          title="Upcoming"
          count={upcoming.length}
          matches={upcoming}
          onUpdate={handleUpdate}
        />
      )}
      {completed.length > 0 && (
        <MatchSection
          title="Completed"
          count={completed.length}
          matches={completed}
          onUpdate={handleUpdate}
        />
      )}
    </div>
  )
}

function MatchSection({
  title,
  count,
  matches,
  onUpdate,
  accentColor,
}: {
  title: string
  count: number
  matches: Match[]
  onUpdate: (m: Match) => void
  accentColor?: string
}) {
  return (
    <section>
      <div className="flex items-center gap-3 mb-4">
        {accentColor && (
          <span className="w-1.5 h-4 rounded-full shrink-0" style={{ backgroundColor: accentColor }} />
        )}
        <h2 className="text-xs uppercase tracking-widest" style={{ color: accentColor ?? '#777' }}>
          {title}
        </h2>
        <span
          className="text-xs tabular-nums px-1.5 py-0.5 rounded"
          style={{ backgroundColor: 'var(--color-border)', color: '#888' }}
        >
          {count}
        </span>
      </div>
      <div className="rounded border divide-y" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface-card)' }}>
        {matches.map(m => <MatchRow key={m.id} match={m} onUpdate={onUpdate} />)}
      </div>
    </section>
  )
}

function MatchRow({ match, onUpdate }: { match: Match; onUpdate: (m: Match) => void }) {
  const [recording, setRecording] = useState(false)
  const [saving, setSaving] = useState<string | null>(null) // holds the winner_id being saved

  const p1Won = match.winner_id === match.player1_id
  const p2Won = match.winner_id === match.player2_id

  async function handleWinner(winnerID: string) {
    setSaving(winnerID)
    try {
      const updated = await api.updateMatch(match.id, winnerID)
      onUpdate(updated)
    } catch {
      setSaving(null)
    }
  }

  const stageBadge = (
    <span className="text-xs px-1.5 py-0.5 rounded shrink-0" style={{ backgroundColor: 'var(--color-border)', color: '#aaa' }}>
      {match.stage === 'group' ? 'Group' : 'KO'}
    </span>
  )

  if (recording) {
    return (
      <div className="px-4 py-3 space-y-2">
        <p className="text-xs uppercase tracking-widest" style={{ color: '#666' }}>Who won?</p>
        <div className="flex items-center gap-2">
          {stageBadge}
          <button
            onClick={() => handleWinner(match.player1_id!)}
            disabled={!!saving}
            className="flex-1 text-sm font-semibold px-3 py-2 rounded border transition-colors disabled:opacity-40 truncate"
            style={{ borderColor: 'var(--color-brand)', color: 'var(--color-brand)' }}
          >
            {saving === match.player1_id ? 'Saving…' : (match.player1_name ?? 'TBD')}
          </button>
          <span className="text-xs shrink-0" style={{ color: '#555' }}>vs</span>
          <button
            onClick={() => handleWinner(match.player2_id!)}
            disabled={!!saving}
            className="flex-1 text-sm font-semibold px-3 py-2 rounded border transition-colors disabled:opacity-40 truncate"
            style={{ borderColor: 'var(--color-brand)', color: 'var(--color-brand)' }}
          >
            {saving === match.player2_id ? 'Saving…' : (match.player2_name ?? 'TBD')}
          </button>
          <button
            onClick={() => setRecording(false)}
            disabled={!!saving}
            className="text-xs px-2 py-1 rounded shrink-0"
            style={{ color: '#666' }}
          >
            ✕
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-4 px-4 py-3 text-sm">
      {stageBadge}
      <div className="flex-1 flex items-center gap-2 min-w-0">
        <span className={`truncate ${p1Won ? 'font-semibold' : ''}`} style={{ color: p1Won ? 'var(--color-brand)' : '#f0f0f0' }}>
          {match.player1_name ?? 'TBD'}
        </span>
        <span className="text-xs shrink-0" style={{ color: '#555' }}>
          {match.status === 'complete' ? 'def.' : 'vs'}
        </span>
        <span className={`truncate ${p2Won ? 'font-semibold' : ''}`} style={{ color: p2Won ? 'var(--color-brand)' : '#f0f0f0' }}>
          {match.player2_name ?? 'TBD'}
        </span>
      </div>
      {match.status !== 'complete' && (
        <button
          onClick={() => setRecording(true)}
          className="text-xs font-bold uppercase tracking-widest px-3 py-1 rounded border shrink-0 transition-colors"
          style={{ borderColor: 'var(--color-border)', color: '#d0d0d0' }}
        >
          Record
        </button>
      )}
    </div>
  )
}
