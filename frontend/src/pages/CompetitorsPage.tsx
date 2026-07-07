import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Competitor, RemovedCompetitor } from '../types'
import ConfirmDialog from '../components/ConfirmDialog'

export default function CompetitorsPage() {
  const [competitors, setCompetitors] = useState<Competitor[]>([])
  const [removed, setRemoved] = useState<RemovedCompetitor[]>([])
  const [loading, setLoading] = useState(true)
  const [removing, setRemoving] = useState<string | null>(null)
  const [restoring, setRestoring] = useState<string | null>(null)
  const [confirm, setConfirm] = useState<{ id: string; name: string } | null>(null)
  const [editing, setEditing] = useState<{ id: string; name: string } | null>(null)
  const [renaming, setRenaming] = useState(false)

  useEffect(() => {
    Promise.all([api.getCompetitors(), api.getRemovedCompetitors()])
      .then(([active, gone]) => { setCompetitors(active); setRemoved(gone) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

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
              <span className="text-xs tabular-nums shrink-0" style={{ color: 'var(--color-brand)' }}>{c.wins}W</span>
              <span className="text-xs tabular-nums shrink-0" style={{ color: '#888' }}>{c.losses}L</span>
              <span className="text-xs tabular-nums shrink-0 font-semibold" style={{ color: '#f0f0f0' }}>{c.points}pts</span>
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
