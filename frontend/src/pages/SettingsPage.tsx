import { useState } from 'react'
import { api } from '../api'
import type { Tournament } from '../types'

interface Props {
  tournament: Tournament | null
  onUpdate: (t: Tournament) => void
}

export default function SettingsPage({ tournament, onUpdate }: Props) {
  const [eventName, setEventName] = useState('')
  const [importing, setImporting] = useState(false)
  const [importMsg, setImportMsg] = useState<string | null>(null)
  const [drawing, setDrawing] = useState(false)
  const [advancing, setAdvancing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleImport() {
    if (!eventName.trim()) return
    setImporting(true)
    setImportMsg(null)
    setError(null)
    try {
      const result = await api.importFromTicketTailor(eventName.trim())
      setImportMsg(`Imported ${result.count} competitors.`)
      const t = await api.getTournament()
      onUpdate(t)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Import failed')
    } finally {
      setImporting(false)
    }
  }

  async function handleDraw() {
    setDrawing(true)
    setError(null)
    try {
      await api.runDraw()
      const t = await api.getTournament()
      onUpdate(t)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Draw failed')
    } finally {
      setDrawing(false)
    }
  }

  async function handleAdvance() {
    setAdvancing(true)
    setError(null)
    try {
      await api.advance()
      const t = await api.getTournament()
      onUpdate(t)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Advance failed')
    } finally {
      setAdvancing(false)
    }
  }

  const canDraw = tournament?.status === 'setup'
  const canAdvance = tournament?.status === 'group_stage'

  return (
    <div className="max-w-lg space-y-10">

      {error && (
        <p className="text-sm rounded px-4 py-2 border" style={{ color: 'var(--color-wax-red)', backgroundColor: 'rgba(232,20,46,0.08)', borderColor: 'rgba(232,20,46,0.3)' }}>
          {error}
        </p>
      )}

      {/* Import */}
      <section className="space-y-4">
        <h2 className="text-xs uppercase tracking-widest font-bold" style={{ color: '#888' }}>Import Competitors</h2>
        <div className="flex gap-2">
          <input
            type="text"
            placeholder="Ticket Tailor event name"
            value={eventName}
            onChange={e => setEventName(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleImport()}
            className="flex-1 rounded px-3 py-2 text-sm focus:outline-none"
            style={{ backgroundColor: 'var(--color-surface-input)', border: '1px solid var(--color-border)', color: '#f0f0f0' }}
          />
          <button
            onClick={handleImport}
            disabled={importing || !eventName.trim()}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ backgroundColor: 'var(--color-brand)', color: '#fff' }}
          >
            {importing ? 'Importing…' : 'Import'}
          </button>
        </div>
        {importMsg && <p className="text-sm" style={{ color: 'var(--color-brand)' }}>{importMsg}</p>}
      </section>

      {/* Draw */}
      <section className="space-y-4">
        <h2 className="text-xs uppercase tracking-widest font-bold" style={{ color: '#888' }}>Group Draw</h2>
        <p className="text-sm" style={{ color: '#888' }}>
          Randomly assigns competitors to groups and generates all round-robin matches.
        </p>
        <button
          onClick={handleDraw}
          disabled={drawing || !canDraw}
          className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
        >
          {drawing ? 'Running draw…' : 'Run Draw'}
        </button>
        {!canDraw && tournament && (
          <p className="text-xs" style={{ color: '#666' }}>Draw has already been run.</p>
        )}
      </section>

      {/* Advance */}
      {canAdvance && (
        <section className="space-y-4">
          <h2 className="text-xs uppercase tracking-widest font-bold" style={{ color: '#888' }}>Advance to Knockout</h2>
          <p className="text-sm" style={{ color: '#888' }}>
            Complete all group matches before advancing. Top competitors from each group
            will be seeded into the knockout bracket.
          </p>
          <button
            onClick={handleAdvance}
            disabled={advancing}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ backgroundColor: 'var(--color-brand)', color: '#fff' }}
          >
            {advancing ? 'Advancing…' : 'Advance to Knockout'}
          </button>
        </section>
      )}

      {/* Danger zone */}
      <section className="space-y-4 pt-4 border-t" style={{ borderColor: 'var(--color-border)' }}>
        <h2 className="text-xs uppercase tracking-widest font-bold" style={{ color: 'var(--color-wax-red)' }}>Danger Zone</h2>
        <p className="text-sm" style={{ color: '#888' }}>
          Re-importing from Ticket Tailor will wipe all existing data and start fresh.
        </p>
      </section>

    </div>
  )
}
