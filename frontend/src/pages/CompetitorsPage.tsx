import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Competitor, Group, RemovedCompetitor, Tournament } from '../types'
import ConfirmDialog from '../components/ConfirmDialog'
import MoveGroupDialog from '../components/MoveGroupDialog'

interface Props {
  tournament: Tournament | null
}

// groupLabel shortens "Group A" to "A" for the table column, where the heading
// already says Group.
function groupLabel(name: string): string {
  return name.replace(/^Group /, '')
}

export default function CompetitorsPage({ tournament }: Props) {
  const [competitors, setCompetitors] = useState<Competitor[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [removed, setRemoved] = useState<RemovedCompetitor[]>([])
  const [loading, setLoading] = useState(true)
  const [removing, setRemoving] = useState<string | null>(null)
  const [restoring, setRestoring] = useState<string | null>(null)
  const [confirm, setConfirm] = useState<{ id: string; name: string } | null>(null)
  const [editing, setEditing] = useState<{ id: string; name: string } | null>(null)
  const [renaming, setRenaming] = useState(false)
  const [moveTarget, setMoveTarget] = useState<Competitor | null>(null)
  const [moving, setMoving] = useState(false)

  useEffect(() => {
    // A missing draw is not an error here — the group column simply stays
    // empty — so the groups request must not sink the whole page.
    Promise.all([api.getCompetitors(), api.getRemovedCompetitors(), api.getGroups().catch(() => [])])
      .then(([active, gone, drawn]) => { setCompetitors(active); setRemoved(gone); setGroups(drawn) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  // Groups can only be reshuffled while the group stage is running: before the
  // draw there is nothing to move between, and once the knockout starts the
  // group results are history.
  const canMove = tournament?.status === 'group_stage' && groups.length > 1
  const groupOf = (c: Competitor) => groups.find(g => g.id === c.group_id) ?? null

  // The roster reads by group: A's players, then B's, and anyone the draw has
  // not placed at the end. The server already lists groups in draw order and
  // competitors by name, and sorting is stable, so ordering by the group's
  // position keeps the names alphabetical within each group.
  const groupOrder = new Map(groups.map((g, i) => [g.id, i]))
  const positionOf = (c: Competitor) =>
    c.group_id !== null && groupOrder.has(c.group_id) ? groupOrder.get(c.group_id)! : groups.length
  const roster = [...competitors].sort((a, b) => positionOf(a) - positionOf(b))

  async function handleMove(groupID: string) {
    if (!moveTarget) return
    setMoving(true)
    try {
      await api.moveCompetitor(moveTarget.id, groupID)
      const [active, drawn] = await Promise.all([api.getCompetitors(), api.getGroups()])
      setCompetitors(active)
      setGroups(drawn)
      setMoveTarget(null)
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to move player.')
    } finally {
      setMoving(false)
    }
  }

  async function handleConfirmRemove() {
    if (!confirm) return
    const target = confirm
    setRemoving(target.id)
    setConfirm(null)
    try {
      await api.deleteCompetitor(target.id)
      setCompetitors(prev => prev.filter(c => c.id !== target.id))
      setRemoved(prev => [...prev, { id: target.id, name: target.name }].sort((a, b) => a.name.localeCompare(b.name)))
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to remove competitor.')
    } finally {
      setRemoving(null)
    }
  }

  async function handleRename() {
    if (!editing) return
    const { id, name } = editing
    const trimmed = name.trim()
    if (!trimmed) return
    setRenaming(true)
    try {
      await api.renameCompetitor(id, trimmed)
      setCompetitors(prev => prev.map(c => c.id === id ? { ...c, name: trimmed } : c))
      setEditing(null)
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to rename competitor.')
    } finally {
      setRenaming(false)
    }
  }

  async function handleRestore(id: string, name: string) {
    setRestoring(id)
    try {
      await api.restoreCompetitor(id)
      setRemoved(prev => prev.filter(c => c.id !== id))
      const updated = await api.getCompetitors()
      setCompetitors(updated)
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Failed to restore competitor.')
    } finally {
      setRestoring(null)
    }
  }

  if (loading) return <p className="text-sm" style={{ color: '#888' }}>Loading…</p>

  return (
    <div className="max-w-2xl mx-auto">
      {confirm && (
        <ConfirmDialog
          title="Remove Competitor"
          message={`Are you sure you want to remove ${confirm.name} from the tournament?`}
          confirmLabel="Remove"
          danger
          onCancel={() => setConfirm(null)}
          onConfirm={handleConfirmRemove}
        />
      )}
      {moveTarget && (
        <MoveGroupDialog
          playerName={moveTarget.name}
          currentGroup={groupOf(moveTarget)}
          groups={groups}
          played={moveTarget.wins + moveTarget.losses}
          busy={moving}
          onMove={handleMove}
          onCancel={() => setMoveTarget(null)}
        />
      )}
      <h2 className="text-xs uppercase tracking-widest mb-4" style={{ color: '#777' }}>
        Competitors — {competitors.length} registered
      </h2>
      <div className="rounded border divide-y" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface-card)' }}>
        {competitors.length > 0 && (
          <div className="flex items-center gap-4 px-4 py-2 text-xs uppercase tracking-widest" style={{ color: '#555' }}>
            <span className="w-6 shrink-0" aria-hidden />
            <span className="flex-1">Player</span>
            <span className="w-8 text-center shrink-0">Group</span>
            <span className="w-12 text-right shrink-0">Pts</span>
            <span className="w-8 text-right shrink-0">W</span>
            <span className="w-8 text-right shrink-0">L</span>
            {canMove && <span className="w-6 shrink-0" aria-hidden />}
            <span className="w-6 shrink-0" aria-hidden />
            <span className="w-6 shrink-0" aria-hidden />
          </div>
        )}
        {competitors.length === 0 ? (
          <p className="px-4 py-6 text-sm text-center" style={{ color: '#888' }}>
            No competitors yet — import from Ticket Tailor in Settings.
          </p>
        ) : (
          roster.map((c, i) => (
            <div
              key={c.id}
              className="flex items-center gap-4 px-4 py-2.5"
              // A heavier rule where one group's block ends and the next
              // begins, so the blocks read apart at a glance.
              style={i > 0 && c.group_id !== roster[i - 1].group_id
                ? { borderTop: '2px solid var(--color-border)' }
                : undefined}
            >
              <span className="text-xs tabular-nums w-6 text-right shrink-0" style={{ color: '#555' }}>
                {i + 1}
              </span>
              {editing?.id === c.id ? (
                <input
                  autoFocus
                  value={editing.name}
                  onChange={e => setEditing({ id: c.id, name: e.target.value })}
                  onKeyDown={e => {
                    if (e.key === 'Enter') handleRename()
                    if (e.key === 'Escape') setEditing(null)
                  }}
                  onBlur={() => setEditing(null)}
                  disabled={renaming}
                  className="text-sm flex-1 rounded px-2 py-0.5 focus:outline-none"
                  style={{ backgroundColor: 'var(--color-surface-input)', border: '1px solid var(--color-brand)', color: '#f0f0f0' }}
                />
              ) : (
                <span className="text-sm flex-1">{c.name}</span>
              )}
              <span
                className="text-xs w-8 text-center shrink-0 font-bold"
                title={groupOf(c)?.name ?? 'Not in a group — the draw has not placed them'}
                style={{ color: groupOf(c) ? 'var(--color-brand-bright)' : '#555' }}
              >
                {groupOf(c) ? groupLabel(groupOf(c)!.name) : '—'}
              </span>
              <span className="text-xs tabular-nums w-12 text-right shrink-0 font-semibold" style={{ color: '#f0f0f0' }}>{c.points}pts</span>
              <span className="text-xs tabular-nums w-8 text-right shrink-0" style={{ color: 'var(--color-brand)' }}>{c.wins}W</span>
              <span className="text-xs tabular-nums w-8 text-right shrink-0" style={{ color: '#888' }}>{c.losses}L</span>
              {canMove && (
                <button
                  onClick={() => setMoveTarget(c)}
                  disabled={moving}
                  title={`Move ${c.name} to another group`}
                  className="text-xs w-6 h-6 flex items-center justify-center rounded border transition-colors disabled:opacity-40"
                  style={{ color: '#888', borderColor: 'var(--color-border)' }}
                >
                  ⇄
                </button>
              )}
              <button
                onClick={() => setEditing({ id: c.id, name: c.name })}
                disabled={renaming || editing?.id === c.id}
                title="Rename — e.g. when the ticket was bought under someone else's name"
                className="text-xs w-6 h-6 flex items-center justify-center rounded border transition-colors disabled:opacity-40"
                style={{ color: '#888', borderColor: 'var(--color-border)' }}
              >
                ✎
              </button>
              <button
                onClick={() => setConfirm({ id: c.id, name: c.name })}
                disabled={removing === c.id}
                className="text-xs w-6 h-6 flex items-center justify-center rounded border transition-colors disabled:opacity-40"
                style={{ color: 'var(--color-wax-red)', borderColor: 'rgba(232,20,46,0.4)' }}
              >
                {removing === c.id ? '…' : '✕'}
              </button>
            </div>
          ))
        )}
      </div>

      {removed.length > 0 && (
        <div className="mt-8">
          <h2 className="text-xs uppercase tracking-widest mb-4" style={{ color: '#555' }}>
            Removed — {removed.length}
          </h2>
          <div className="rounded border divide-y" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface-card)' }}>
            {removed.map(c => (
              <div key={c.id} className="flex items-center gap-4 px-4 py-2.5">
                <span className="text-sm flex-1" style={{ color: '#666' }}>{c.name}</span>
                <button
                  onClick={() => handleRestore(c.id, c.name)}
                  disabled={restoring === c.id}
                  className="text-xs font-bold uppercase tracking-widest px-3 py-1 rounded border shrink-0 transition-colors disabled:opacity-40"
                  style={{ borderColor: 'rgba(61,122,94,0.5)', color: 'var(--color-brand)' }}
                >
                  {restoring === c.id ? '…' : 'Re-seed'}
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
