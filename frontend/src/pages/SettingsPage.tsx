import { useState } from 'react'
import { api } from '../api'
import type { Tournament } from '../types'

const LS_API_KEY = 'tt_api_key'
const LS_EVENT_NAME = 'tt_event_name'

interface Props {
  tournament: Tournament | null
  onUpdate: (t: Tournament | null) => void
}

export default function SettingsPage({ tournament, onUpdate }: Props) {
  const [apiKey, setApiKey] = useState(() => localStorage.getItem(LS_API_KEY) ?? '')
  const [eventName, setEventName] = useState(() => localStorage.getItem(LS_EVENT_NAME) ?? '')
  const [importing, setImporting] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [drawing, setDrawing] = useState(false)
  const [advancing, setAdvancing] = useState(false)
  const [message, setMessage] = useState<{ text: string; ok: boolean } | null>(null)

  function saveApiKey(val: string) {
    setApiKey(val)
    localStorage.setItem(LS_API_KEY, val)
  }

  function saveEventName(val: string) {
    setEventName(val)
    localStorage.setItem(LS_EVENT_NAME, val)
  }

  async function handleRefresh() {
    if (!apiKey.trim() || !eventName.trim()) return
    setImporting(true)
    setMessage(null)
    try {
      const result = await api.importFromTicketTailor(apiKey.trim(), eventName.trim())
      setMessage({ text: `Imported ${result.count} competitors from "${result.tournament}".`, ok: true })
      const t = await api.getTournament()
      onUpdate(t)
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : 'Import failed', ok: false })
    } finally {
      setImporting(false)
    }
  }

  async function handleClear() {
    if (!confirm('This will delete all competitors, groups, and matches. Are you sure?')) return
    setClearing(true)
    setMessage(null)
    try {
      await api.clearTournament()
      setMessage({ text: 'All data cleared.', ok: true })
      onUpdate(null)
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : 'Clear failed', ok: false })
    } finally {
      setClearing(false)
    }
  }

  async function handleDraw() {
    setDrawing(true)
    setMessage(null)
    try {
      await api.runDraw()
      const t = await api.getTournament()
      onUpdate(t)
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : 'Draw failed', ok: false })
    } finally {
      setDrawing(false)
    }
  }

  async function handleAdvance() {
    setAdvancing(true)
    setMessage(null)
    try {
      await api.advance()
      const t = await api.getTournament()
      onUpdate(t)
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : 'Advance failed', ok: false })
    } finally {
      setAdvancing(false)
    }
  }

  const canDraw = tournament?.status === 'setup'
  const canAdvance = tournament?.status === 'group_stage'
  const canRefresh = !!apiKey.trim() && !!eventName.trim() && !importing

  return (
    <div className="max-w-lg space-y-10">

      {message && (
        <p
          className="text-sm rounded px-4 py-2 border"
          style={message.ok
            ? { color: 'var(--color-brand)', backgroundColor: 'rgba(61,122,94,0.1)', borderColor: 'rgba(61,122,94,0.3)' }
            : { color: 'var(--color-wax-red)', backgroundColor: 'rgba(232,20,46,0.08)', borderColor: 'rgba(232,20,46,0.3)' }
          }
        >
          {message.text}
        </p>
      )}

      {/* Ticket Tailor */}
      <section className="space-y-4">
        <h2 className="text-xs uppercase tracking-widest font-bold" style={{ color: '#888' }}>Ticket Tailor</h2>

        <div className="space-y-2">
          <label className="text-xs uppercase tracking-widest" style={{ color: '#666' }}>API Key</label>
          <input
            type="password"
            placeholder="tt_live_…"
            value={apiKey}
            onChange={e => saveApiKey(e.target.value)}
            className="w-full rounded px-3 py-2 text-sm focus:outline-none"
            style={{ backgroundColor: 'var(--color-surface-input)', border: '1px solid var(--color-border)', color: '#f0f0f0' }}
          />
        </div>

        <div className="space-y-2">
          <label className="text-xs uppercase tracking-widest" style={{ color: '#666' }}>Event Name</label>
          <input
            type="text"
            placeholder="Backgammon and Wax — Summer Open 2026"
            value={eventName}
            onChange={e => saveEventName(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleRefresh()}
            className="w-full rounded px-3 py-2 text-sm focus:outline-none"
            style={{ backgroundColor: 'var(--color-surface-input)', border: '1px solid var(--color-border)', color: '#f0f0f0' }}
          />
        </div>

        <div className="flex gap-2">
          <button
            onClick={handleRefresh}
            disabled={!canRefresh}
            className="flex-1 px-4 py-2 text-sm font-bold uppercase tracking-wide rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ backgroundColor: 'var(--color-brand)', color: '#fff' }}
          >
            {importing ? 'Importing…' : 'Refresh Contestants'}
          </button>
          <button
            onClick={handleClear}
            disabled={clearing || !tournament}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ borderColor: 'rgba(232,20,46,0.5)', color: 'var(--color-wax-red)' }}
          >
            {clearing ? 'Clearing…' : 'Clear'}
          </button>
        </div>
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
            Top competitors from each group will be seeded into the knockout bracket.
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

    </div>
  )
}
