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
                    {round === 1 ? <FinalBoard match={m} /> : <MatchCard match={m} />}
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

// FinalBoard gives the final its own stage: a backgammon board drawn from the
// app palette. Alternating red/green points run along the top and bottom
// edges, a centre bar carries the VS badge, and each finalist owns a half of
// the board. The winner's half stays lit; the loser's fades.
function FinalBoard({ match }: { match: Match }) {
  const done = match.status === 'complete'
  const p1Won = done && match.winner_id === match.player1_id
  const p2Won = done && match.winner_id === match.player2_id

  return (
    <div
      className="w-full max-w-[520px] rounded-2xl p-2 border-2"
      style={{ backgroundColor: 'var(--color-surface-base)', borderColor: '#555' }}
    >
      <div className="flex rounded-lg overflow-hidden border" style={{ borderColor: 'var(--color-border)' }}>
        <BoardHalf
          name={match.player1_name}
          score={p1Won ? match.player1_score : null}
          side="red"
          faded={done && !p1Won}
          winner={p1Won}
          pointOffset={0}
        />
        {/* The bar: the raised divider between the board's halves. */}
        <div
          className="w-6 shrink-0 relative flex items-center justify-center border-x"
          style={{ backgroundColor: 'var(--color-surface-base)', borderColor: 'var(--color-border)' }}
        >
          <div
            className="absolute w-9 h-9 rounded-full flex items-center justify-center text-[10px] font-black uppercase border-2 z-10"
            style={{ backgroundColor: 'var(--color-surface-base)', borderColor: '#555', color: '#f0f0f0' }}
          >
            vs
          </div>
        </div>
        <BoardHalf
          name={match.player2_name}
          score={p2Won ? match.player2_score : null}
          side="green"
          faded={done && !p2Won}
          winner={p2Won}
          pointOffset={1}
        />
      </div>
    </div>
  )
}

function BoardHalf({ name, score, side, faded, winner, pointOffset }: {
  name: string | null
  score: number | null
  side: 'red' | 'green'
  faded: boolean
  winner: boolean
  pointOffset: number
}) {
  const nameColor = name
    ? side === 'red' ? 'var(--color-wax-red-bright)' : 'var(--color-brand-bright)'
    : '#666'

  return (
    <div
      className="flex-1 min-w-0 relative h-44 transition-opacity"
      style={{ backgroundColor: 'var(--color-surface-card)', opacity: faded ? 0.4 : 1 }}
    >
      {/* The points are the playing surface: a background layer running in
          from the top and bottom edges, passing behind the centre diamond. */}
      <PointsStrip direction="down" offset={pointOffset} side={side} className="absolute inset-x-0 top-0 h-[44%]" />
      <PointsStrip direction="up" offset={pointOffset + 1} side={side} className="absolute inset-x-0 bottom-0 h-[44%]" />
      {/* Table dressing: a dice cup and a thrown pair resting on the board. */}
      <DiceCluster side={side} />
      {/* The diamond inlay floats in the foreground, carrying the name. */}
      <div className="absolute inset-0 flex items-center justify-center px-3">
        <div className="relative w-[88%] max-w-[240px] h-20 flex flex-col items-center justify-center gap-0.5 text-center">
          <svg
            viewBox="0 0 200 80"
            preserveAspectRatio="none"
            className="absolute inset-0 w-full h-full"
            style={{ filter: 'drop-shadow(0 3px 8px rgba(0,0,0,0.55))' }}
            aria-hidden
          >
            <path
              d="M 6,40 Q 80,27 100,6 Q 120,27 194,40 Q 120,53 100,74 Q 80,53 6,40 Z"
              fill="var(--color-surface-base)"
              stroke="#555"
              strokeWidth="1.5"
            />
          </svg>
          {winner && (
            <p className="relative z-10 text-[10px] uppercase tracking-widest font-black" style={{ color: 'var(--color-brand-bright)' }}>
              ★ Winner ★
            </p>
          )}
          <p className="relative z-10 font-black text-base leading-tight truncate max-w-[70%]" style={{ color: nameColor }}>
            {name ?? <span className="italic font-normal">TBD</span>}
          </p>
          {score !== null && (
            <p className="relative z-10 text-xs tabular-nums font-semibold" style={{ color: '#ccc' }}>+{score}</p>
          )}
        </div>
      </div>
    </div>
  )
}

// DiceCluster scatters a thrown pair of dice on each half's playing surface.
// The halves mirror each other so the board looks casually used rather than
// patterned.
function DiceCluster({ side }: { side: 'red' | 'green' }) {
  return side === 'red' ? (
    <div className="absolute left-[8%] bottom-[10%] flex items-end gap-1.5" aria-hidden>
      <Die value={4} rotation={18} />
      <Die value={3} rotation={-11} />
    </div>
  ) : (
    <div className="absolute right-[8%] top-[10%] flex items-start gap-1.5" aria-hidden>
      <Die value={2} rotation={14} />
      <Die value={6} rotation={-19} />
    </div>
  )
}

const diePips: Record<number, [number, number][]> = {
  1: [[10, 10]],
  2: [[6, 6], [14, 14]],
  3: [[5.5, 5.5], [10, 10], [14.5, 14.5]],
  4: [[6, 6], [14, 6], [6, 14], [14, 14]],
  5: [[5.5, 5.5], [14.5, 5.5], [10, 10], [5.5, 14.5], [14.5, 14.5]],
  6: [[6, 5], [14, 5], [6, 10], [14, 10], [6, 15], [14, 15]],
}

function Die({ value, rotation }: { value: number; rotation: number }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 20 20"
      style={{ transform: `rotate(${rotation}deg)`, filter: 'drop-shadow(0 2px 3px rgba(0,0,0,0.5))' }}
      aria-hidden
    >
      <rect x="0.5" y="0.5" width="19" height="19" rx="4.5" fill="#f0f0f0" />
      {(diePips[value] ?? []).map(([cx, cy], i) => (
        <circle key={i} cx={cx} cy={cy} r="1.7" fill="var(--color-surface-base)" />
      ))}
    </svg>
  )
}

// PointsStrip draws one edge of a board half: six long triangular points
// stretched to fill whatever width the half has. The red player's half
// alternates red and white points, the green player's green and white,
// keeping the red-vs-green identity of the two sides. Proportions follow a
// real board: each strip is ~40% of the board's height, so a point is
// roughly 3–4× as long as its base is wide, and the fills are muted so the
// points read as the playing surface behind the names, not the foreground.
function PointsStrip({ direction, offset, side, className }: { direction: 'down' | 'up'; offset: number; side: 'red' | 'green'; className?: string }) {
  const tinted = side === 'red' ? 'rgba(232,20,46,0.28)' : 'rgba(61,122,94,0.38)'
  const white = 'rgba(240,240,240,0.15)'
  return (
    <svg viewBox="0 0 120 100" preserveAspectRatio="none" className={`w-full block ${className ?? ''}`} aria-hidden>
      {Array.from({ length: 6 }, (_, i) => {
        const x = i * 20
        const fill = (i + offset) % 2 === 0 ? tinted : white
        const points = direction === 'down'
          ? `${x},0 ${x + 20},0 ${x + 10},100`
          : `${x},100 ${x + 20},100 ${x + 10},0`
        return <polygon key={i} points={points} fill={fill} />
      })}
    </svg>
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
