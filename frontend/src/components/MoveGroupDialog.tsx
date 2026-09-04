import { useEffect, useState } from 'react'
import type { Group } from '../types'

interface Props {
  playerName: string
  // The group they are in now, so the dialog can name it and leave it out of
  // the choices. Null before the draw has placed them.
  currentGroup: Group | null
  groups: Group[]
  // Games already played. Moving discards them, so the dialog says so.
  played: number
  busy?: boolean
  onMove: (groupID: string) => void
  onCancel: () => void
}

// MoveGroupDialog asks which group a player should move to. Choosing a group
// arms the Move button rather than acting immediately: the move rebuilds
// fixtures and can discard results, so it takes a deliberate second press.
export default function MoveGroupDialog({ playerName, currentGroup, groups, played, busy = false, onMove, onCancel }: Props) {
  const [selected, setSelected] = useState<string | null>(null)

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onCancel()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onCancel])

  const targets = groups.filter(g => g.id !== currentGroup?.id)

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.7)' }}
      onClick={onCancel}
    >
      <div
        className="w-full max-w-md rounded-xl border p-8 space-y-5"
        style={{ backgroundColor: 'var(--color-surface-card)', borderColor: 'var(--color-border)' }}
        onClick={e => e.stopPropagation()}
      >
        <div className="space-y-1">
          <h2 className="text-sm uppercase tracking-widest font-black" style={{ color: '#f0f0f0' }}>
            Move Player
          </h2>
          <p className="text-sm" style={{ color: '#aaa' }}>
            {playerName}
            {currentGroup && <> — currently in <span style={{ color: '#f0f0f0' }}>{currentGroup.name}</span></>}
          </p>
        </div>

        {played > 0 && (
          <p
            className="text-xs leading-relaxed rounded border p-3"
            style={{ borderColor: 'rgba(232,20,46,0.4)', color: 'var(--color-wax-red-bright)' }}
          >
            {playerName} has already played {played} {played === 1 ? 'game' : 'games'}. Moving them discards those
            results — theirs and their opponents' — and gives them a fresh set of fixtures.
          </p>
        )}

        {targets.length === 0 ? (
          <p className="text-sm" style={{ color: '#888' }}>There is no other group to move them to.</p>
        ) : (
          <div className="max-h-64 overflow-y-auto grid grid-cols-2 gap-2">
            {targets.map(g => (
              <button
                key={g.id}
                onClick={() => setSelected(g.id)}
                disabled={busy}
                className="text-left px-3 py-2 rounded border text-sm transition-colors disabled:opacity-40"
                style={selected === g.id
                  ? { borderColor: 'var(--color-brand)', backgroundColor: 'rgba(61,122,94,0.15)', color: '#f0f0f0' }
                  : { borderColor: 'var(--color-border)', color: '#d0d0d0' }}
              >
                <span className="block font-bold">{g.name}</span>
                <span className="block text-xs" style={{ color: '#888' }}>
                  {g.competitors.length} {g.competitors.length === 1 ? 'player' : 'players'}
                </span>
              </button>
            ))}
          </div>
        )}

        <div className="flex gap-3 justify-end pt-2">
          <button
            onClick={onCancel}
            disabled={busy}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors disabled:opacity-40"
            style={{ borderColor: 'var(--color-border)', color: '#d0d0d0' }}
          >
            Cancel
          </button>
          <button
            onClick={() => selected && onMove(selected)}
            disabled={!selected || busy}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={played > 0
              ? { backgroundColor: 'var(--color-wax-red)', color: '#fff' }
              : { backgroundColor: 'var(--color-brand)', color: '#fff' }}
          >
            {busy ? 'Moving…' : 'Move'}
          </button>
        </div>
      </div>
    </div>
  )
}
