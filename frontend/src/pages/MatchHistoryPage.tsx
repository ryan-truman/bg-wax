import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Match } from '../types'
import { roundLabel, bracketLabel } from '../components/BracketView'

export default function MatchHistoryPage() {
  const [matches, setMatches] = useState<Match[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let active = true
    const load = () =>
      api.getMatches()
        .then(m => { if (active) setMatches(m) })
        .catch(() => {})

    load().finally(() => { if (active) setLoading(false) })

    // Poll so newly-revealed matchups (e.g. the next knockout round, or results
    // entered on another device) appear without a manual refresh.
    const timer = setInterval(load, 5000)
    return () => { active = false; clearInterval(timer) }
  }, [])

  function handleUpdate(updated: Match) {
    // Flip the just-recorded match immediately for snappy feedback…
    setMatches(prev => prev.map(m => m.id === updated.id ? updated : m))
    // …then refetch, since recording a result can reveal the next match.
    api.getMatches().then(setMatches).catch(() => {})
  }

  if (loading) return <p className="text-sm" style={{ color: '#888' }}>Loading…</p>

  if (!matches.length) {
    return <p className="text-sm" style={{ color: '#888' }}>No matches yet.</p>
  }

  const knockout = matches.filter(m => m.stage === 'knockout')
  const group = matches.filter(m => m.stage === 'group')
  const brackets = [...new Set(knockout.map(m => m.bracket ?? 1))].sort((a, b) => a - b)

  return (
    <div className="max-w-2xl space-y-12">
      {brackets.map(b => (
        <KnockoutRounds
          key={b}
          label={brackets.length > 1 ? `Knockout — ${bracketLabel(b)}` : 'Knockout'}
          matches={knockout.filter(m => (m.bracket ?? 1) === b)}
          onUpdate={handleUpdate}
        />
      ))}
      {group.length > 0 && (
        <StageGroup label={knockout.length > 0 ? 'Group Stage' : null} matches={group} onUpdate={handleUpdate} />
      )}
    </div>
  )
}

function StageHeader({ label }: { label: string }) {
  return (
    <h2 className="text-sm uppercase tracking-widest font-black pb-2 border-b" style={{ color: 'var(--color-brand-bright)', borderColor: 'var(--color-border)' }}>
      {label}
    </h2>
  )
}

function KnockoutRounds({ label, matches, onUpdate }: { label: string; matches: Match[]; onUpdate: (m: Match) => void }) {
  // Latest round first: the final (round 1) is the furthest-progressed, so
  // sort ascending by round number — whatever round is currently live sits on top.
  const rounds = [...new Set(matches.map(m => m.round))].sort((a, b) => (a ?? 0) - (b ?? 0))

  return (
    <div className="space-y-8">
      <StageHeader label={label} />
      {rounds.map(round => {
        const roundMatches = matches
          .filter(m => m.round === round)
          .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
        return (
          <MatchSection
            key={round ?? 'x'}
            title={roundLabel(round)}
            count={roundMatches.length}
            matches={roundMatches}
            onUpdate={onUpdate}
          />
        )
      })}
    </div>
  )
}

function StageGroup({ label, matches, onUpdate }: { label: string | null; matches: Match[]; onUpdate: (m: Match) => void }) {
  const inProgress = matches.filter(m => m.status === 'in_progress')
  const upcoming = matches.filter(m => m.status === 'pending')
  const completed = matches.filter(m => m.status === 'complete')

  return (
    <div className="space-y-8">
      {label && <StageHeader label={label} />}
      {inProgress.length > 0 && (
        <MatchSection
          title="In Progress"
          count={inProgress.length}
          matches={inProgress}
          onUpdate={onUpdate}
          accentColor="var(--color-brand)"
        />
      )}
      {upcoming.length > 0 && (
        <MatchSection
          title="Upcoming"
          count={upcoming.length}
          matches={upcoming}
          onUpdate={onUpdate}
        />
      )}
      {completed.length > 0 && (
        <MatchSection
          title="Completed"
          count={completed.length}
          matches={completed}
          onUpdate={onUpdate}
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
      <div className="rounded-xl overflow-hidden border divide-y" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface-card)' }}>
        {matches.map(m => <MatchRow key={m.id} match={m} onUpdate={onUpdate} />)}
      </div>
    </section>
  )
}

function MatchRow({ match, onUpdate }: { match: Match; onUpdate: (m: Match) => void }) {
  const [recording, setRecording] = useState(false)
  const [saving, setSaving] = useState(false)

  const p1Won = match.winner_id === match.player1_id
  const p2Won = match.winner_id === match.player2_id
  const winnerPoints = p1Won ? match.player1_score : p2Won ? match.player2_score : null

  async function handleResult(winnerID: string, points: number) {
    setSaving(true)
    try {
      const updated = await api.updateMatch(match.id, winnerID, points)
      setRecording(false)
      setSaving(false)
      onUpdate(updated)
    } catch {
      setSaving(false)
    }
  }

  const stageBadge = (
    <span className="text-xs px-1.5 py-0.5 rounded shrink-0" style={{ backgroundColor: 'var(--color-border)', color: '#aaa' }}>
      {match.stage === 'group' ? 'Group' : 'KO'}
    </span>
  )

  if (recording && match.status !== 'complete') {
    return (
      <div className="px-4 py-3 space-y-3">
        <div className="flex items-center gap-2">
          {stageBadge}
          <p className="text-xs uppercase tracking-widest" style={{ color: '#666' }}>Who won?</p>
          <button
            onClick={() => setRecording(false)}
            disabled={saving}
            className="ml-auto text-xs px-2 py-1 rounded"
            style={{ color: '#666' }}
          >
            ✕
          </button>
        </div>
        <div className="grid grid-cols-2 gap-3">
          {([
            [match.player1_id!, match.player1_name],
            [match.player2_id!, match.player2_name],
          ] as [string, string | null][]).map(([pid, name], col) => (
            <div key={pid} className="space-y-1.5">
              <p className="text-xs truncate font-semibold" style={{ color: '#f0f0f0' }}>{name ?? 'TBD'}</p>
              <div className="flex gap-1">
                {(col === 0 ? [3, 2, 1] : [1, 2, 3]).map(pts => (
                  <button
                    key={pts}
                    onClick={() => handleResult(pid, pts)}
                    disabled={saving}
                    className="flex-1 text-sm font-bold py-1.5 rounded border transition-colors disabled:opacity-40"
                    style={col === 0
                      ? { borderColor: 'var(--color-wax-red)', color: 'var(--color-wax-red)' }
                      : { borderColor: 'var(--color-brand)', color: 'var(--color-brand)' }}
                  >
                    +{pts}
                  </button>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-4 px-4 py-3 text-sm">
      {stageBadge}
      <div className="flex-1 flex items-center gap-2 min-w-0">
        <span className={`truncate ${p1Won ? 'font-semibold' : ''}`} style={{ color: p1Won ? 'var(--color-brand-bright)' : '#f0f0f0' }}>
          {match.player1_name ?? 'TBD'}
        </span>
        <span className="text-xs shrink-0" style={{ color: '#555' }}>
          {match.status === 'complete' ? 'def.' : 'vs'}
        </span>
        <span className={`truncate ${p2Won ? 'font-semibold' : ''}`} style={{ color: p2Won ? 'var(--color-brand-bright)' : '#f0f0f0' }}>
          {match.player2_name ?? 'TBD'}
        </span>
      </div>
      {match.status === 'complete' && winnerPoints !== null && (
        <span className="text-xs tabular-nums shrink-0" style={{ color: '#666' }}>+{winnerPoints}</span>
      )}
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
