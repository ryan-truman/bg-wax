import { useEffect, useState } from 'react'
import type { TieBreak } from '../types'
import { bracketLabel } from './BracketView'

interface Props {
  ties: TieBreak[]
  onCancel: () => void
  // Receives each tie ID mapped to the finishing order chosen: the players
  // picked to progress first, then the rest.
  onResolve: (tieBreaks: Record<string, string[]>) => void
}

function ordinal(n: number) {
  const rest = n % 100
  if (rest >= 11 && rest <= 13) return `${n}th`
  switch (n % 10) {
    case 1: return `${n}st`
    case 2: return `${n}nd`
    case 3: return `${n}rd`
    default: return `${n}th`
  }
}

function plural(n: number, word: string) {
  return `${n} ${word}${n === 1 ? '' : 's'}`
}

// What the tie is competing for. A group tie is for a real finishing place, so it
// can be named; a pool tie is for the places left over once every group has sent
// its guaranteed qualifiers, where a position within the pool means nothing.
function title(tie: TieBreak) {
  if (tie.scope === 'pool') return `Last ${plural(tie.slots, 'place')} — best runners-up`
  return `${tie.group_name} — ${ordinal(tie.place)} place`
}

// What happens to the players who miss out, spelled out so the choice is clear.
function consequence(tie: TieBreak) {
  const missing = tie.competitors.length - tie.slots
  const who = missing === 1 ? "The one you don't pick" : `The ${missing} you don't pick`
  if (tie.drops_to_pool) return `${who} still ${missing === 1 ? 'competes' : 'compete'} for the leftover places in this bracket.`
  if (tie.drops_to_bracket) return `${who} ${missing === 1 ? 'drops' : 'drop'} to the ${bracketLabel(tie.drops_to_bracket)}.`
  return `${who} ${missing === 1 ? 'does' : 'do'} not progress.`
}

// TieBreakDialog asks the organiser to settle ties that decide who progresses
// to the knockout. It only appears for ties points and head-to-head cannot
// split, and every tie must be answered before the advance can go ahead.
export default function TieBreakDialog({ ties, onCancel, onResolve }: Props) {
  // Per tie, the players picked to progress, in the order they were clicked.
  const [picked, setPicked] = useState<Record<string, string[]>>({})

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onCancel()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onCancel])

  function toggle(tie: TieBreak, id: string) {
    setPicked(prev => {
      const current = prev[tie.id] ?? []
      if (current.includes(id)) return { ...prev, [tie.id]: current.filter(x => x !== id) }
      // A single place behaves like a radio button; several places fill up in
      // click order and then stop.
      if (tie.slots === 1) return { ...prev, [tie.id]: [id] }
      if (current.length >= tie.slots) return prev
      return { ...prev, [tie.id]: [...current, id] }
    })
  }

  const done = ties.every(t => (picked[t.id] ?? []).length === t.slots)

  function confirm() {
    const answer: Record<string, string[]> = {}
    for (const tie of ties) {
      const chosen = picked[tie.id] ?? []
      const rest = tie.competitors.map(c => c.id).filter(id => !chosen.includes(id))
      answer[tie.id] = [...chosen, ...rest]
    }
    onResolve(answer)
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.7)' }}
      onClick={onCancel}
    >
      <div
        className="w-full max-w-md rounded-xl border p-8 space-y-5 max-h-[85vh] overflow-y-auto"
        style={{ backgroundColor: 'var(--color-surface-card)', borderColor: 'var(--color-border)' }}
        onClick={e => e.stopPropagation()}
      >
        <h2 className="text-sm uppercase tracking-widest font-black" style={{ color: '#f0f0f0' }}>
          {ties.length > 1 ? 'Settle the ties' : 'Settle the tie'}
        </h2>
        <p className="text-sm leading-relaxed" style={{ color: '#aaa' }}>
          {ties.length > 1
            ? `${ties.length} ties decide who progresses and can't be settled on points, head-to-head or wins. Choose who goes through.`
            : "This tie decides who progresses and can't be settled on points, head-to-head or wins. Choose who goes through."}
        </p>

        {ties.map((tie, i) => {
          const chosen = picked[tie.id] ?? []
          // Ties arrive grouped by bracket; name the bracket when it changes so
          // it is never a guess which one a tie is deciding.
          const newBracket = i === 0 || ties[i - 1].bracket !== tie.bracket
          return (
            <div key={tie.id} className="space-y-2">
              {newBracket && (
                <div
                  className="text-xs uppercase tracking-widest font-black pt-2 pb-1 border-b"
                  style={{ color: 'var(--color-brand)', borderColor: 'var(--color-border)' }}
                >
                  {bracketLabel(tie.bracket)}
                </div>
              )}
              <div className="space-y-1">
                <h3 className="text-xs uppercase tracking-widest font-bold" style={{ color: '#f0f0f0' }}>
                  {title(tie)}
                </h3>
                <p className="text-xs" style={{ color: '#888' }}>
                  {plural(tie.competitors.length, 'player')} level on {plural(tie.points, 'point')} —
                  {' '}pick {tie.slots} to go through{tie.slots > 1 ? ` (${chosen.length}/${tie.slots} chosen)` : ''}
                </p>
                <p className="text-xs" style={{ color: '#777' }}>{consequence(tie)}</p>
              </div>
              <div className="space-y-2">
                {tie.competitors.map(c => {
                  const rank = chosen.indexOf(c.id)
                  const on = rank >= 0
                  return (
                    <button
                      key={c.id}
                      onClick={() => toggle(tie, c.id)}
                      className="w-full flex items-center gap-3 px-3 py-2 rounded border text-left transition-colors"
                      style={{
                        borderColor: on ? 'var(--color-brand)' : 'var(--color-border)',
                        backgroundColor: on ? 'rgba(232,20,46,0.10)' : 'transparent',
                      }}
                    >
                      <span
                        className="w-5 h-5 shrink-0 rounded-full border flex items-center justify-center text-[10px] font-bold"
                        style={{
                          borderColor: on ? 'var(--color-brand)' : 'var(--color-border)',
                          backgroundColor: on ? 'var(--color-brand)' : 'transparent',
                          color: '#fff',
                        }}
                      >
                        {on && tie.slots > 1 ? rank + 1 : ''}
                      </span>
                      <span className="flex-1 text-sm" style={{ color: on ? '#f0f0f0' : '#d0d0d0' }}>
                        {c.name}
                        {tie.scope === 'pool' && (
                          <span className="ml-2 text-xs" style={{ color: '#777' }}>{c.group_name}</span>
                        )}
                      </span>
                      <span className="text-xs tabular-nums" style={{ color: '#888' }}>
                        {c.won}W · {c.points} pts
                      </span>
                    </button>
                  )
                })}
              </div>
            </div>
          )
        })}

        <div className="flex gap-3 justify-end pt-2">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors"
            style={{ borderColor: 'var(--color-border)', color: '#d0d0d0' }}
          >
            Cancel
          </button>
          <button
            onClick={confirm}
            disabled={!done}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ backgroundColor: 'var(--color-brand)', color: '#fff' }}
          >
            Advance
          </button>
        </div>
      </div>
    </div>
  )
}
