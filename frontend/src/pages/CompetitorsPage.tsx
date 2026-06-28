import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Competitor, RemovedCompetitor } from '../types'

export default function CompetitorsPage() {
  const [competitors, setCompetitors] = useState<Competitor[]>([])
  const [removed, setRemoved] = useState<RemovedCompetitor[]>([])
  const [loading, setLoading] = useState(true)
  const [removing, setRemoving] = useState<string | null>(null)
  const [restoring, setRestoring] = useState<string | null>(null)
  const [confirm, setConfirm] = useState<{ id: string; name: string } | null>(null)

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
    <div className="max-w-2xl">
      {confirm && (
        <div className="fixed inset-0 flex items-center justify-center z-50" style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}>
          <div className="rounded-lg p-6 max-w-sm w-full mx-4 space-y-4 border" style={{ backgroundColor: 'var(--color-surface-card)', borderColor: 'var(--color-border)' }}>
            <p className="text-sm" style={{ color: '#f0f0f0' }}>
              Are you sure you want to remove <span className="font-semibold">{confirm.name}</span> from the tournament?
            </p>
            <div className="flex gap-2 justify-end">
              <button
                onClick={() => setConfirm(null)}
                className="text-xs font-bold uppercase tracking-widest px-4 py-2 rounded border transition-colors"
                style={{ borderColor: 'var(--color-border)', color: '#888' }}
              >
                Cancel
              </button>
              <button
                onClick={handleConfirmRemove}
                className="text-xs font-bold uppercase tracking-widest px-4 py-2 rounded transition-colors"
                style={{ backgroundColor: 'var(--color-wax-red)', color: '#fff' }}
              >
                Remove
              </button>
            </div>
          </div>
        </div>
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
              <span className="text-sm flex-1">{c.name}</span>
              <span className="text-xs tabular-nums shrink-0" style={{ color: 'var(--color-brand)' }}>{c.wins}W</span>
              <span className="text-xs tabular-nums shrink-0" style={{ color: '#888' }}>{c.losses}L</span>
              <span className="text-xs tabular-nums shrink-0 font-semibold" style={{ color: '#f0f0f0' }}>{c.points}pts</span>
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
