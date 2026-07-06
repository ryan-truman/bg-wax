import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Match } from '../types'

export default function BracketView() {
  const [matches, setMatches] = useState<Match[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let active = true
    const load = () =>
      api.getBracket()
        .then(m => { if (active) setMatches(m) })
        .catch(() => {})

    load().finally(() => { if (active) setLoading(false) })

    // Poll so results recorded on the matches page (or another device) move
    // the highlighted round forward without a manual refresh.
    const timer = setInterval(load, 5000)
    return () => { active = false; clearInterval(timer) }
  }, [])

  if (loading) return <p className="text-sm" style={{ color: '#888' }}>Loading bracket…</p>
  if (!matches.length) return <p className="text-sm" style={{ color: '#888' }}>No bracket matches yet.</p>

  const brackets = [...new Set(matches.map(m => m.bracket ?? 1))].sort((a, b) => a - b)

  return (
    <div className="space-y-12">
      {brackets.map(b => (
        <section key={b} className="space-y-4">
          {brackets.length > 1 && (
            <h3 className="text-sm uppercase tracking-widest font-black pb-2 border-b" style={{ color: '#f0f0f0', borderColor: 'var(--color-border)' }}>
              {bracketLabel(b)}
            </h3>
          )}
          <BracketTree matches={matches.filter(m => (m.bracket ?? 1) === b)} />
        </section>
      ))}
    </div>
  )
}

// The current tier is the earliest round that still has unplayed matches;
// once everything is decided, stay on the final.
function currentRoundOf(ms: Match[]): number {
  const unfinished = ms.filter(m => m.status !== 'complete').map(m => m.round ?? 0)
  if (unfinished.length === 0) return 1
  return Math.max(...unfinished)
}

function BracketTree({ matches }: { matches: Match[] }) {
  const current = currentRoundOf(matches)
  // Only show rounds someone has reached — later all-TBD rounds appear as
  // winners propagate into them.
  const reached = matches.filter(m => m.player1_id !== null || m.player2_id !== null)
  // Ascending: the final (round 1) renders first, so the tournament climbs
  // upward from the earliest round at the bottom.
  let rounds = [...new Set(reached.map(m => m.round))].sort((a, b) => (a ?? 0) - (b ?? 0))
  // Hide rounds that are fully played — they live on in Match History. Once
  // the whole bracket is done, keep just the final to show the champion.
  const unfinishedRounds = rounds.filter(r => matches.some(m => m.round === r && m.status !== 'complete'))
  rounds = unfinishedRounds.length > 0 ? unfinishedRounds : rounds.slice(0, 1)
  const final = matches.find(m => m.round === 1)
  const champion = final?.status === 'complete'
    ? (final.winner_id === final.player1_id ? final.player1_name : final.player2_name)
    : null
  const widest = Math.max(...rounds.map(r => matches.filter(m => m.round === r).length))

  return (
    <div className="overflow-x-auto pt-2 pb-4">
      {/* Fluid: match slots share the available width equally, so the tree
          scales to the screen. The 110px-per-match floor only kicks in on
          very narrow screens (phones), where the bracket scrolls instead. */}
      <div className="flex flex-col gap-8" style={{ minWidth: widest * 110 }}>
        {rounds.map(round => {
          const roundMatches = matches
            .filter(m => m.round === round)
            .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
          const isCurrent = round === current
          const played = roundMatches.filter(m => m.status === 'complete').length

          return (
            <div key={round} className="space-y-3">
              <div className="text-center space-y-0.5">
                <p className="text-xs uppercase tracking-widest font-bold" style={{ color: isCurrent ? 'var(--color-brand-bright)' : '#777' }}>
                  {roundLabel(round)}
                </p>
                {round === 1 && champion ? (
                  <p className="text-[11px] font-bold" style={{ color: 'var(--color-brand-bright)' }}>
                    Champion: {champion}
                  </p>
                ) : isCurrent ? (
                  <p className="text-[11px]" style={{ color: '#777' }}>
                    {played} of {roundMatches.length} played
                  </p>
                ) : null}
              </div>
              {/* Equal flex slots keep each match centred over its two
                  feeder matches in the (wider) row below. */}
              <div className="flex">
                {roundMatches.map(m => (
                  <div key={m.id} className="flex-1 flex justify-center min-w-0 px-1">
                    <MatchCard match={m} />
                  </div>
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// MatchCard renders every knockout matchup: player 1 on wax red over
// player 2 on brand green, split by a VS badge. A completed match fades
// the loser and shows the winner's points.
function MatchCard({ match }: { match: Match }) {
  const done = match.status === 'complete'
  const p1Won = done && match.winner_id === match.player1_id
  const p2Won = done && match.winner_id === match.player2_id

  return (
    <div
      className="relative rounded-xl border w-full max-w-[220px]"
      style={{ borderColor: '#555', backgroundColor: 'var(--color-surface-card)' }}
    >
      <div className="p-2 flex flex-col">
        <PlayerPanel
          name={match.player1_name}
          score={p1Won ? match.player1_score : null}
          side="red"
          faded={done && !p1Won}
        />
        {/* In-flow with negative margins so the badge sits on the seam
            between the panels even when their heights differ. */}
        <div
          className="self-center -my-2 w-8 h-8 rounded-full flex items-center justify-center text-[10px] font-black uppercase border-2 z-10"
          style={{ backgroundColor: 'var(--color-surface-base)', borderColor: '#555', color: '#f0f0f0' }}
        >
          vs
        </div>
        <PlayerPanel
          name={match.player2_name}
          score={p2Won ? match.player2_score : null}
          side="green"
          faded={done && !p2Won}
        />
      </div>
    </div>
  )
}

function PlayerPanel({ name, score, side, faded }: {
  name: string | null
  score: number | null
  side: 'red' | 'green'
  faded: boolean
}) {
  const background = name
    ? side === 'red'
      ? 'linear-gradient(160deg, rgba(232,20,46,0.22), rgba(232,20,46,0.06))'
      : 'linear-gradient(200deg, rgba(61,122,94,0.28), rgba(61,122,94,0.08))'
    : 'var(--color-surface-input)'

  return (
    <div
      className="rounded-lg px-2 py-3 text-center min-w-0"
      style={{ background, opacity: faded ? 0.4 : 1 }}
    >
      <p
        className="font-bold text-sm truncate"
        style={{ color: name ? (side === 'red' ? 'var(--color-wax-red-bright)' : 'var(--color-brand-bright)') : '#666' }}
      >
        {name ?? <span className="italic font-normal">TBD</span>}
      </p>
      {score !== null && (
        <p className="text-xs mt-0.5 tabular-nums font-semibold" style={{ color: '#ccc' }}>+{score}</p>
      )}
    </div>
  )
}

export function bracketLabel(bracket: number): string {
  return bracket === 1 ? "Champion's League" : 'Europa League'
}

export function roundLabel(round: number | null): string {
  if (round === null) return ''
  if (round === 1) return 'Final'
  if (round === 2) return 'Semi-finals'
  if (round === 3) return 'Quarter-finals'
  return `Round of ${Math.pow(2, round)}`
}
