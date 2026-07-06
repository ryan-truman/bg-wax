import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import type { Tournament, TicketTailorEvent } from '../types'

const LS_API_KEY = 'tt_api_key'
const LS_EVENT_ID = 'tt_event_id'
const LS_EVENT_NAME = 'tt_event_name'

interface Props {
  tournament: Tournament | null
  onUpdate: (t: Tournament | null) => void
}

export default function SettingsPage({ tournament, onUpdate }: Props) {
  const navigate = useNavigate()
  const [showApiModal, setShowApiModal] = useState(false)

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      if (showApiModal) setShowApiModal(false)
      else navigate('/')
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate, showApiModal])

  const [apiKey, setApiKey] = useState(() => localStorage.getItem(LS_API_KEY) ?? '')
  const [eventId, setEventId] = useState(() => localStorage.getItem(LS_EVENT_ID) ?? '')

  // Under `just demo` the server reports demo mode; pre-fill the built-in
  // demo API key so the import workflow works with no setup. Never
  // overwrites a key the user has saved themselves.
  useEffect(() => {
    if (localStorage.getItem(LS_API_KEY)) return
    api.getConfig()
      .then(cfg => {
        if (!cfg.demo) return
        localStorage.setItem(LS_API_KEY, 'demo')
        setApiKey('demo')
      })
      .catch(() => {})
  }, [])

  // Draft edited in the modal; only persisted on Save.
  const [draftKey, setDraftKey] = useState('')
  const [events, setEvents] = useState<TicketTailorEvent[] | null>(null)
  const [loadingEvents, setLoadingEvents] = useState(false)
  const [eventsError, setEventsError] = useState<string | null>(null)

  // Whenever the saved key changes (including on load), fetch the account's
  // events for the dropdown. Keeps the saved selection if it still exists,
  // otherwise falls back to the first event.
  useEffect(() => {
    if (!apiKey) {
      setEvents(null)
      return
    }
    let active = true
    setLoadingEvents(true)
    setEventsError(null)
    api.listTicketTailorEvents(apiKey)
      .then(list => {
        if (!active) return
        setEvents(list)
        setEventId(prev => {
          if (list.some(e => e.id === prev)) return prev
          const first = list[0]
          if (!first) return ''
          localStorage.setItem(LS_EVENT_ID, first.id)
          localStorage.setItem(LS_EVENT_NAME, first.name)
          return first.id
        })
      })
      .catch(e => {
        if (!active) return
        setEvents(null)
        setEventsError(e instanceof Error ? e.message : 'Could not fetch events')
      })
      .finally(() => { if (active) setLoadingEvents(false) })
    return () => { active = false }
  }, [apiKey])

  function selectEvent(id: string) {
    const event = events?.find(e => e.id === id)
    if (!event) return
    localStorage.setItem(LS_EVENT_ID, event.id)
    localStorage.setItem(LS_EVENT_NAME, event.name)
    setEventId(event.id)
  }
  const [importing, setImporting] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [drawing, setDrawing] = useState(false)
  const [numGroups, setNumGroups] = useState(8)
  const [advancing, setAdvancing] = useState(false)
  const [groupCount, setGroupCount] = useState(0)
  const [totalCompetitors, setTotalCompetitors] = useState(0)
  const [advanceTotal, setAdvanceTotal] = useState(0)
  const [singleBracket, setSingleBracket] = useState(false)

  // True once any group match has a result — the point where redraw locks.
  const [anyPlayed, setAnyPlayed] = useState(false)

  // The advance counter is a total across all groups, stepping by the group
  // count so every group contributes the same number of qualifiers.
  useEffect(() => {
    if (tournament?.status !== 'group_stage') {
      setAnyPlayed(false)
      return
    }
    api.getGroups()
      .then(gs => {
        const total = gs.reduce((n, g) => n + g.competitors.length, 0)
        setGroupCount(gs.length)
        setTotalCompetitors(total)
        setAdvanceTotal(t => t || Math.min(4, Math.floor(total / gs.length)) * gs.length)
      })
      .catch(() => {})
    api.getMatches()
      .then(ms => setAnyPlayed(ms.some(m => m.status !== 'pending')))
      .catch(() => {})
  }, [tournament?.status])
  const [message, setMessage] = useState<{ text: string; ok: boolean } | null>(null)

  function openApiModal() {
    setDraftKey(apiKey)
    setShowApiModal(true)
  }

  // Saving the key persists it and (via the effect above) refreshes the
  // event dropdown on the main page.
  function saveApiSettings() {
    const key = draftKey.trim()
    if (!key) return
    localStorage.setItem(LS_API_KEY, key)
    setApiKey(key)
    setShowApiModal(false)
  }

  function clearApiSettings() {
    localStorage.removeItem(LS_API_KEY)
    localStorage.removeItem(LS_EVENT_ID)
    localStorage.removeItem(LS_EVENT_NAME)
    setApiKey('')
    setEventId('')
    setDraftKey('')
    setEvents(null)
    setEventsError(null)
  }

  async function handleRefresh() {
    if (!apiKey || !eventId) return
    setImporting(true)
    setMessage(null)
    try {
      const result = await api.importFromTicketTailor(apiKey, eventId)
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
      await api.runDraw(numGroups)
      const t = await api.getTournament()
      onUpdate(t)
      // A redraw can change the group count, so recompute the advance
      // default rather than keeping a stale total.
      const gs = await api.getGroups()
      const total = gs.reduce((n, g) => n + g.competitors.length, 0)
      setGroupCount(gs.length)
      setTotalCompetitors(total)
      setAdvanceTotal(Math.min(4, Math.floor(total / gs.length)) * gs.length)
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
      await api.advance(advanceTotal, singleBracket)
      const t = await api.getTournament()
      onUpdate(t)
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : 'Advance failed', ok: false })
    } finally {
      setAdvancing(false)
    }
  }

  const isRedraw = tournament?.status === 'group_stage'
  const canDraw = tournament?.status === 'setup' || (isRedraw && !anyPlayed)
  const canAdvance = tournament?.status === 'group_stage'
  const canRefresh = !!apiKey && !!eventId && !importing

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

      {/* Event picker + import */}
      <section className="space-y-4">
        {events && events.length > 0 ? (
          <div className="relative">
            <select
              value={eventId}
              onChange={e => selectEvent(e.target.value)}
              className="w-full rounded-lg pl-4 pr-10 py-3 text-sm focus:outline-none appearance-none cursor-pointer"
              style={{ backgroundColor: 'var(--color-surface-input)', border: '1px solid var(--color-border)', color: '#f0f0f0' }}
            >
              {events.map(e => (
                <option key={e.id} value={e.id}>{e.name}</option>
              ))}
            </select>
            <svg
              className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 pointer-events-none"
              viewBox="0 0 16 16" fill="none" stroke="#888" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
            >
              <path d="M4 6l4 4 4-4" />
            </svg>
          </div>
        ) : (
          <p className="text-sm rounded-lg px-4 py-3" style={{ backgroundColor: 'var(--color-surface-input)', color: eventsError ? 'var(--color-wax-red-bright)' : '#666' }}>
            {eventsError ?? (loadingEvents
              ? 'Fetching events…'
              : apiKey
                ? 'No events found on this account.'
                : 'No API key saved — open API Settings to connect your account.')}
          </p>
        )}
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
            onClick={openApiModal}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors"
            style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
          >
            API Settings
          </button>
        </div>
      </section>

      {/* API settings modal */}
      {showApiModal && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4"
          style={{ backgroundColor: 'rgba(0,0,0,0.7)' }}
          onClick={() => setShowApiModal(false)}
        >
          <div
            className="w-full max-w-md rounded-xl border p-8 space-y-6"
            style={{ backgroundColor: 'var(--color-surface-card)', borderColor: 'var(--color-border)' }}
            onClick={e => e.stopPropagation()}
          >
            <img src="/logo.png" alt="" className="w-24 h-24 mx-auto" />

            <div className="space-y-2">
              <label className="text-xs uppercase tracking-widest font-bold" style={{ color: '#888' }}>API Key</label>
              <input
                type="password"
                placeholder="tt_live_…"
                value={draftKey}
                onChange={e => setDraftKey(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && saveApiSettings()}
                autoFocus
                className="w-full rounded-lg px-4 py-3 text-sm focus:outline-none"
                style={{ backgroundColor: 'var(--color-surface-input)', border: '1px solid var(--color-border)', color: '#f0f0f0' }}
              />
            </div>

            <div className="flex gap-3 pt-2">
              <button
                onClick={clearApiSettings}
                className="px-4 py-3 text-sm font-bold uppercase tracking-widest rounded border transition-colors"
                style={{ borderColor: 'rgba(232,20,46,0.5)', color: 'var(--color-wax-red-bright)' }}
              >
                Clear Saved Data
              </button>
              <button
                onClick={saveApiSettings}
                disabled={!draftKey.trim()}
                className="flex-1 px-4 py-3 text-sm font-bold uppercase tracking-widest rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                style={{ backgroundColor: '#f0f0f0', color: '#1a1a1a' }}
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Draw — only available before the draw has been run */}
      {canDraw && (
        <section className="space-y-4">
          <h2 className="text-xs uppercase tracking-widest font-bold" style={{ color: '#888' }}>Group Draw</h2>
          <p className="text-sm" style={{ color: '#888' }}>
            {isRedraw
              ? 'Re-randomizes the groups and regenerates all round-robin matches. Available until the first result is recorded.'
              : 'Randomly assigns competitors to groups and generates all round-robin matches.'}
          </p>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setNumGroups(n => Math.max(2, n - 1))}
              disabled={drawing || numGroups <= 2}
              className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
            >
              −
            </button>
            <span className="w-6 text-center text-sm tabular-nums" style={{ color: '#f0f0f0' }}>{numGroups}</span>
            <button
              onClick={() => setNumGroups(n => Math.min(26, n + 1))}
              disabled={drawing || numGroups >= 26}
              className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
            >
              +
            </button>
            <button
              onClick={handleDraw}
              disabled={drawing}
              className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
            >
              {drawing ? 'Running draw…' : isRedraw ? 'Redraw Groups' : 'Run Draw'}
            </button>
          </div>
        </section>
      )}

      {/* Advance */}
      {canAdvance && groupCount > 0 && (() => {
        const perGroup = advanceTotal / groupCount
        const mainTotal = Math.ceil(perGroup / 2) * groupCount
        return (
        <section className="space-y-4">
          <h2 className="text-xs uppercase tracking-widest font-bold" style={{ color: '#888' }}>Advance to Knockout</h2>
          <p className="text-sm" style={{ color: '#888' }}>
            {singleBracket || perGroup < 2
              ? `The top ${perGroup === 1 ? 'competitor' : `${perGroup} competitors`} from each group (${advanceTotal} total) will be seeded by score into a single knockout bracket.`
              : `${mainTotal} players go to the Champion's League and ${advanceTotal - mainTotal} to the Europa League, taking the top ${perGroup} from each group. Seeding is by score, keeping group-mates apart for as long as possible.`}
          </p>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setAdvanceTotal(n => Math.max(groupCount, n - groupCount))}
              disabled={advancing || advanceTotal <= groupCount}
              className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
            >
              −
            </button>
            <span className="w-8 text-center text-sm tabular-nums" style={{ color: '#f0f0f0' }}>{advanceTotal}</span>
            <button
              onClick={() => setAdvanceTotal(n => Math.min(totalCompetitors, n + groupCount))}
              disabled={advancing || advanceTotal + groupCount > totalCompetitors}
              className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
            >
              +
            </button>
            <span className="text-xs uppercase tracking-widest" style={{ color: '#666' }}>progress to knockout</span>
          </div>
          <label className="flex items-center gap-2 text-sm cursor-pointer select-none" style={{ color: '#f0f0f0' }}>
            <input
              type="checkbox"
              checked={singleBracket}
              onChange={e => setSingleBracket(e.target.checked)}
              disabled={advancing}
              className="w-4 h-4 accent-[var(--color-brand)]"
            />
            Single knockout bracket
          </label>
          <button
            onClick={handleAdvance}
            disabled={advancing}
            className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ backgroundColor: 'var(--color-brand)', color: '#fff' }}
          >
            {advancing ? 'Advancing…' : 'Advance to Knockout'}
          </button>
        </section>
        )
      })()}

      {/* Reset — destructive, kept at the bottom away from the routine actions */}
      <section className="pt-6 border-t" style={{ borderColor: 'var(--color-border)' }}>
        <button
          onClick={handleClear}
          disabled={clearing || !tournament}
          className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          style={{ borderColor: 'rgba(232,20,46,0.5)', color: 'var(--color-wax-red)' }}
        >
          {clearing ? 'Resetting…' : 'Reset Tournament'}
        </button>
      </section>

    </div>
  )
}
