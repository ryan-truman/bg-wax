import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Match } from '../types'

export default function BracketView() {
  const [matches, setMatches] = useState<Match[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.getBracket()
      .then(setMatches)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="text-sm" style={{ color: '#888' }}>Loading bracket…</p>
  if (!matches.length) return <p className="text-sm" style={{ color: '#888' }}>No bracket matches yet.</p>

  const rounds = [...new Set(matches.map(m => m.round))].sort((a, b) => (b ?? 0) - (a ?? 0))

  return (
    <div className="flex gap-8 overflow-x-auto pb-4">
      {rounds.map(round => (
        <div key={round} className="flex flex-col gap-4 min-w-[180px]">
          <p className="text-xs uppercase tracking-widest text-center" style={{ color: '#777' }}>
            {roundLabel(round)}
          </p>
          <div className="flex flex-col justify-around h-full gap-6">
            {matches
              .filter(m => m.round === round)
              .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
              .map(m => (
                <BracketMatch key={m.id} match={m} />
              ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function BracketMatch({ match }: { match: Match }) {
  const p1Won = match.winner_id === match.player1_id
  const p2Won = match.winner_id === match.player2_id

  return (
    <div className="rounded overflow-hidden text-sm border" style={{ borderColor: 'var(--color-border)' }}>
      <MatchRow name={match.player1_name} score={match.player1_score} winner={p1Won} />
      <div className="border-t" style={{ borderColor: 'var(--color-border)' }} />
      <MatchRow name={match.player2_name} score={match.player2_score} winner={p2Won} />
    </div>
  )
}

function MatchRow({ name, score, winner }: { name: string | null; score: number | null; winner: boolean }) {
  return (
    <div
      className="flex items-center justify-between px-3 py-2"
      style={{
        backgroundColor: winner ? 'var(--color-border)' : 'var(--color-surface-card)',
        color: winner ? 'var(--color-brand)' : '#f0f0f0',
      }}
    >
      <span className="truncate flex-1 mr-2">
        {name ?? <span style={{ color: '#666' }} className="italic">TBD</span>}
      </span>
      {score !== null && (
        <span className="tabular-nums font-semibold text-xs">{score}</span>
      )}
    </div>
  )
}

export function roundLabel(round: number | null): string {
  if (round === null) return ''
  if (round === 1) return 'Final'
  if (round === 2) return 'Semi-finals'
  if (round === 3) return 'Quarter-finals'
  return `Round of ${Math.pow(2, round)}`
}
