import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'
import type { Tournament, TicketTailorEvent, Settings } from '../types'
import ConfirmDialog from '../components/ConfirmDialog'

// Bracket sizes on offer, mirroring advanceTotals in internal/api/handlers.go.
// Powers of two only, so a bracket fills exactly and nobody gets a bye.
const ADVANCE_TOTALS = [2, 4, 8, 16, 32, 64]

// MAX_GROUPS matches the server's cap on a fixed group count. It is a sanity
// guard only: the draw is what rejects a count the entrant list can't fill.
const MAX_GROUPS = 200

// DEFAULT_FIXED_GROUPS is where the fixed-count stepper starts when the
// organiser switches away from automatic sizing without a count of their own.
const DEFAULT_FIXED_GROUPS = 8

// stepAdvanceTotal moves one place along ADVANCE_TOTALS, so the −/+ buttons
// step 8 → 16 rather than through sizes no bracket can be built from. An
// unrecognised stored value (an older setting, say) snaps onto the list.
function stepAdvanceTotal(current: number, direction: 1 | -1): number {
  const i = ADVANCE_TOTALS.indexOf(current)
  if (i === -1) return ADVANCE_TOTALS[0]
  return ADVANCE_TOTALS[Math.min(ADVANCE_TOTALS.length - 1, Math.max(0, i + direction))]
}

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
  const [confirmReset, setConfirmReset] = useState(false)

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      if (showApiModal) setShowApiModal(false)
      else if (confirmReset) return // ConfirmDialog closes itself on Escape
      else navigate('/')
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate, showApiModal, confirmReset])

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

  // Organiser preferences, persisted server-side. This page only configures
  // behaviour — the Run Draw and Advance to Knockout buttons that act on
  // these settings live on the Match History page.
  const [settings, setSettings] = useState<Settings>({
    min_group_games: 4,
    num_groups: 0,
    advance_total: 16,
    single_bracket: false,
  })
  useEffect(() => {
    api.getSettings().then(setSettings).catch(() => {})
  }, [])
  function changeSettings(patch: Partial<Settings>) {
    setSettings(s => ({ ...s, ...patch }))
    api.updateSettings(patch).catch(() => {})
  }

  // num_groups is the whole story: 0 means the draw sizes groups itself, anything
  // else is a count the organiser set. Switching to automatic sends 0, which would
  // forget that count, so it is remembered here for the rest of the sitting.
  const fixedGroups = settings.num_groups > 0
  const [lastFixedGroups, setLastFixedGroups] = useState(DEFAULT_FIXED_GROUPS)
  useEffect(() => {
    if (settings.num_groups > 0) setLastFixedGroups(settings.num_groups)
  }, [settings.num_groups])
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
    setClearing(true)
    setMessage(null)
    try {
      await api.clearTournament()
      setMessage({ text: 'Tournament reset — competitors kept, ready for a new draw.', ok: true })
      onUpdate(await api.getTournament().catch(() => null))
    } catch (e) {
      setMessage({ text: e instanceof Error ? e.message : 'Clear failed', ok: false })
    } finally {
      setClearing(false)
    }
  }

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

      {/* Draw configuration — the Run Draw button itself lives on Match History */}
      <section className="space-y-4">
        <h2 className="text-xs uppercase tracking-widest font-bold" style={{ color: '#888' }}>Group Draw</h2>

        {/* The two sizing methods are alternatives, so only the chosen one's
            control is on screen: a games-per-player target the draw sizes
            groups from, or a group count set outright. */}
        <div className="flex gap-2">
          {([
            { fixed: false, label: 'Automatic', hint: 'Size groups from a games-per-player target' },
            { fixed: true, label: 'Fixed count', hint: 'Set the number of groups yourself' },
          ] as const).map(mode => {
            const active = fixedGroups === mode.fixed
            return (
              <button
                key={mode.label}
                onClick={() => changeSettings({ num_groups: mode.fixed ? lastFixedGroups : 0 })}
                title={mode.hint}
                className="flex-1 px-3 py-2 rounded border text-xs font-bold uppercase tracking-widest transition-colors"
                style={active
                  ? { borderColor: 'var(--color-brand)', backgroundColor: 'rgba(61,122,94,0.15)', color: '#f0f0f0' }
                  : { borderColor: 'var(--color-border)', color: '#888' }}
              >
                {mode.label}
              </button>
            )
          })}
        </div>

        {fixedGroups ? (
          <>
            <div className="flex items-center gap-2">
              <button
                onClick={() => changeSettings({ num_groups: Math.max(2, settings.num_groups - 1) })}
                disabled={settings.num_groups <= 2}
                className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
              >
                −
              </button>
              <span className="w-8 text-center text-sm tabular-nums" style={{ color: '#f0f0f0' }}>{settings.num_groups}</span>
              <button
                onClick={() => changeSettings({ num_groups: Math.min(MAX_GROUPS, settings.num_groups + 1) })}
                disabled={settings.num_groups >= MAX_GROUPS}
                className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
              >
                +
              </button>
              <span className="text-xs uppercase tracking-widest" style={{ color: '#666' }}>groups</span>
            </div>
            <p className="text-xs" style={{ color: '#888' }}>
              The field is split into exactly {settings.num_groups} groups, however many games that
              gives each player. The draw refuses a count that would leave any group below four players.
            </p>
          </>
        ) : (
          <>
            <div className="flex items-center gap-2">
              <button
                onClick={() => changeSettings({ min_group_games: Math.max(3, settings.min_group_games - 1) })}
                disabled={settings.min_group_games <= 3}
                className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
              >
                −
              </button>
              <span className="w-8 text-center text-sm tabular-nums" style={{ color: '#f0f0f0' }}>{settings.min_group_games}</span>
              <button
                onClick={() => changeSettings({ min_group_games: Math.min(10, settings.min_group_games + 1) })}
                disabled={settings.min_group_games >= 10}
                className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
              >
                +
              </button>
              <span className="text-xs uppercase tracking-widest" style={{ color: '#666' }}>group games per player</span>
            </div>
            <p className="text-xs" style={{ color: '#888' }}>
              A minimum: the draw makes groups of {settings.min_group_games + 1}. When the numbers don't
              split evenly, the leftovers make some groups one player bigger — an extra game, never fewer.
            </p>
          </>
        )}
      </section>

      {/* Knockout configuration — the Advance button lives on Match History */}
      <section className="space-y-4">
        <h2 className="text-xs uppercase tracking-widest font-bold" style={{ color: '#888' }}>Knockout</h2>
        <p className="text-sm" style={{ color: '#888' }}>
          {settings.single_bracket
            ? `The top ${settings.advance_total} finishers advance into a single knockout bracket. Each group sends its top places, and the best of the next-place finishers fill any remainder.`
            : `The top ${settings.advance_total} finishers advance into each of two brackets — the Champion's League, then the Europa League below it. Each group sends its top places, with the best of the next-place finishers filling any remainder. Seeding keeps group-mates apart for as long as possible.`}
        </p>
        <div className="flex items-center gap-2">
          <button
            onClick={() => changeSettings({ advance_total: stepAdvanceTotal(settings.advance_total, -1) })}
            disabled={settings.advance_total <= ADVANCE_TOTALS[0]}
            className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
          >
            −
          </button>
          <span className="w-8 text-center text-sm tabular-nums" style={{ color: '#f0f0f0' }}>{settings.advance_total}</span>
          <button
            onClick={() => changeSettings({ advance_total: stepAdvanceTotal(settings.advance_total, 1) })}
            disabled={settings.advance_total >= ADVANCE_TOTALS[ADVANCE_TOTALS.length - 1]}
            className="w-8 h-8 flex items-center justify-center rounded border text-sm font-bold transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ borderColor: 'var(--color-border)', color: '#f0f0f0' }}
          >
            +
          </button>
          <span className="text-xs uppercase tracking-widest" style={{ color: '#666' }}>players per bracket</span>
        </div>
        <label className="flex items-center gap-3 text-sm cursor-pointer select-none" style={{ color: '#f0f0f0' }}>
          <input
            type="checkbox"
            checked={settings.single_bracket}
            onChange={e => changeSettings({ single_bracket: e.target.checked })}
            className="sr-only peer"
          />
          <span
            className="relative w-9 h-5 rounded-full transition-colors shrink-0 peer-focus-visible:ring-2"
            style={{ backgroundColor: settings.single_bracket ? 'var(--color-brand)' : 'var(--color-border)' }}
          >
            <span
              className="absolute top-0.5 left-0.5 w-4 h-4 rounded-full transition-transform"
              style={{ backgroundColor: '#f0f0f0', transform: settings.single_bracket ? 'translateX(16px)' : 'translateX(0)' }}
            />
          </span>
          Single knockout bracket
        </label>
      </section>

      {/* Reset — destructive, kept at the bottom away from the routine actions */}
      <section className="pt-6 border-t" style={{ borderColor: 'var(--color-border)' }}>
        <button
          onClick={() => setConfirmReset(true)}
          disabled={clearing || !tournament}
          className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded border transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          style={{ borderColor: 'rgba(232,20,46,0.5)', color: 'var(--color-wax-red)' }}
        >
          {clearing ? 'Resetting…' : 'Reset Tournament'}
        </button>
      </section>

      {confirmReset && (
        <ConfirmDialog
          title="Reset Tournament"
          message="This will delete the draw and all recorded results. Competitors are kept."
          confirmLabel="Reset"
          danger
          onCancel={() => setConfirmReset(false)}
          onConfirm={() => { setConfirmReset(false); handleClear() }}
        />
      )}

    </div>
  )
}
