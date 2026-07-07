import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Match, Tournament } from '../types'
import { roundLabel, bracketLabel } from '../components/BracketView'
import DrawControl from '../components/DrawControl'
import ConfirmDialog from '../components/ConfirmDialog'

interface Props {
  tournament: Tournament | null
  onUpdate: (t: Tournament) => void
}

export default function MatchHistoryPage({ tournament, onUpdate }: Props) {
  const [matches, setMatches] = useState<Match[]>([])
  const [loading, setLoading] = useState(true)
  const [advancing, setAdvancing] = useState(false)
  const [advanceError, setAdvanceError] = useState<string | null>(null)
  // Set to the number of unfinished group games while the confirmation
  // dialog is open; null when it isn't.
  const [confirmAdvance, setConfirmAdvance] = useState<number | null>(null)

  useEffect(() => {
    let active = true
    const load = () =>
      api.getMatches()
        .then(m => { if (active) setMatches(m) })
        .catch(() => {})

    load().finally(() => { if (active) setLoading(false) })

    // Poll so newly-revealed matchups (e.g. the next knockout round, or results
    // entered on another device) appear without a manual refresh.
    const timer = setInterval(load, 5000)
    return () => { active = false; clearInterval(timer) }
  }, [])

  function handleUpdate(updated: Match) {
    // Flip the just-recorded match immediately for snappy feedback…
    setMatches(prev => prev.map(m => m.id === updated.id ? updated : m))
    // …then refetch, since recording a result can reveal the next match.
    api.getMatches().then(setMatches).catch(() => {})
  }

  // After a draw, redraw, or advance the tournament status and the fixture
  // list both change; pull both fresh.
  async function refresh() {
    const [t, m] = await Promise.all([api.getTournament(), api.getMatches()])
    onUpdate(t)
    setMatches(m)
  }

  if (loading) return <p className="text-sm" style={{ color: '#888' }}>Loading…</p>

  if (!matches.length) {
    // Before the draw there are no matches — offer to run it from here so the
    // organiser doesn't have to detour via Settings.
    if (tournament?.status === 'setup') {
      return (
        <div className="flex flex-col items-center justify-center py-32 gap-6 text-center">
          <p className="text-sm" style={{ color: '#888' }}>
            No matches yet — run the group draw to generate the round-robin fixtures.
          </p>
          <DrawControl onDrawn={refresh} />
        </div>
      )
    }
    return <p className="text-sm" style={{ color: '#888' }}>No matches yet.</p>
  }

  const knockout = matches.filter(m => m.stage === 'knockout')
  const group = matches.filter(m => m.stage === 'group')
  const brackets = [...new Set(knockout.map(m => m.bracket ?? 1))].sort((a, b) => a - b)

  // Mirrors the backend rules for correcting a completed result: group scores
  // lock once the knockout has started (they decided the seeding), and a
  // knockout score locks once the next round's match has been played.
  function canEditCompleted(m: Match): boolean {
    if (m.status !== 'complete') return false
    if (m.stage === 'group') return tournament?.status === 'group_stage'
    if (m.round == null || m.position == null) return false
    if (m.round === 1) return true
    const parent = matches.find(p =>
      p.stage === 'knockout' &&
      (p.bracket ?? 1) === (m.bracket ?? 1) &&
      p.round === m.round! - 1 &&
      p.position === Math.floor(m.position! / 2)
    )
    return !parent || parent.status !== 'complete'
  }

  function handleAdvanceClick() {
    const unfinished = matches.filter(m => m.stage === 'group' && m.status !== 'complete').length
    if (unfinished > 0) setConfirmAdvance(unfinished)
    else handleAdvance()
  }

  async function handleAdvance() {
    setAdvancing(true)
    setAdvanceError(null)
    try {
      await api.advance() // uses the persisted settings
      await refresh()
    } catch (e) {
      setAdvanceError(e instanceof Error ? e.message : 'Advance failed')
    } finally {
      setAdvancing(false)
    }
  }

  // Redraw is possible until the first result is recorded; advancing to the
  // knockout is possible throughout the group stage. Both are configured on
  // the Settings page — these buttons just fire them.
  const inGroupStage = tournament?.status === 'group_stage'
  const canRedraw = inGroupStage && matches.every(m => m.status === 'pending')

  return (
    <div className="max-w-2xl mx-auto space-y-12">
      {inGroupStage && (
        <div className="flex flex-col items-center gap-3">
          <div className="flex items-center gap-3">
            {canRedraw && <DrawControl buttonLabel="Redraw Groups" onDrawn={refresh} />}
            <button
              onClick={handleAdvanceClick}
              disabled={advancing}
              className="px-4 py-2 text-sm font-bold uppercase tracking-wide rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              style={{ backgroundColor: 'var(--color-brand)', color: '#fff' }}
            >
              {advancing ? 'Advancing…' : 'Advance to Knockout'}
            </button>
          </div>
          {advanceError && (
            <p
              className="text-sm rounded px-4 py-2 border"
              style={{ color: 'var(--color-wax-red)', backgroundColor: 'rgba(232,20,46,0.08)', borderColor: 'rgba(232,20,46,0.3)' }}
            >
              {advanceError}
            </p>
          )}
        </div>
      )}
      {confirmAdvance !== null && (
        <ConfirmDialog
          title="Advance to Knockout"
          message={`${confirmAdvance} game${confirmAdvance === 1 ? ' is' : 's are'} still in progress — are you sure you want to progress to knockout? Unplayed group games will be abandoned.`}
          confirmLabel="Advance"
          onCancel={() => setConfirmAdvance(null)}
          onConfirm={() => { setConfirmAdvance(null); handleAdvance() }}
        />
      )}
      {brackets.map(b => (
        <KnockoutRounds
          key={b}
          label={brackets.length > 1 ? `Knockout — ${bracketLabel(b)}` : 'Knockout'}
          matches={knockout.filter(m => (m.bracket ?? 1) === b)}
          onUpdate={handleUpdate}
          canEdit={canEditCompleted}
        />
      ))}
      {group.length > 0 && (
        <StageGroup
          label={knockout.length > 0 ? 'Group Stage' : null}
          matches={group}
          onUpdate={handleUpdate}
          canEdit={canEditCompleted}
          stageOver={!inGroupStage}
        />
      )}
    </div>
  )
}

function StageHeader({ label }: { label: string }) {
  return (
    <h2 className="text-sm uppercase tracking-widest font-black pb-2 border-b" style={{ color: 'var(--color-brand-bright)', borderColor: 'var(--color-border)' }}>
      {label}
    </h2>
  )
}

function KnockoutRounds({ label, matches, onUpdate, canEdit }: { label: string; matches: Match[]; onUpdate: (m: Match) => void; canEdit: (m: Match) => boolean }) {
  // Latest round first: the final (round 1) is the furthest-progressed, so
  // sort ascending by round number — whatever round is currently live sits on top.
  const rounds = [...new Set(matches.map(m => m.round))].sort((a, b) => (a ?? 0) - (b ?? 0))

  return (
    <div className="space-y-8">
      <StageHeader label={label} />
      {rounds.map(round => {
        const roundMatches = matches
          .filter(m => m.round === round)
          .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
        return (
          <MatchSection
            key={round ?? 'x'}
            title={roundLabel(round)}
            count={roundMatches.length}
            matches={roundMatches}
            onUpdate={onUpdate}
            canEdit={canEdit}
            collapsible={roundMatches.every(m => m.status === 'complete')}
          />
        )
      })}
    </div>
  )
}

function StageGroup({ label, matches, onUpdate, canEdit, stageOver = false }: { label: string | null; matches: Match[]; onUpdate: (m: Match) => void; canEdit: (m: Match) => boolean; stageOver?: boolean }) {
  // Once the knockout has started the group stage is history: everything —
  // including games that were never played — files into the collapsed
  // Completed section, and nothing can be recorded any more.
  if (stageOver) {
    return (
      <div className="space-y-8">
        {label && <StageHeader label={label} />}
        <MatchSection
          title="Completed"
          count={matches.length}
          matches={matches}
          onUpdate={onUpdate}
          canEdit={canEdit}
          recordable={false}
          collapsible
        />
      </div>
    )
  }

  const inProgress = matches.filter(m => m.status === 'in_progress')
  const upcoming = matches.filter(m => m.status === 'pending')
  const completed = matches.filter(m => m.status === 'complete')

  return (
    <div className="space-y-8">
      {label && <StageHeader label={label} />}
      {inProgress.length > 0 && (
        <MatchSection
          title="In Progress"
          count={inProgress.length}
          matches={inProgress}
          onUpdate={onUpdate}
          canEdit={canEdit}
          accentColor="var(--color-brand)"
        />
      )}
      {upcoming.length > 0 && (
        <MatchSection
          title="Upcoming"
          count={upcoming.length}
          matches={upcoming}
          onUpdate={onUpdate}
          canEdit={canEdit}
        />
      )}
      {completed.length > 0 && (
        <MatchSection
          title="Completed"
          count={completed.length}
          matches={completed}
          onUpdate={onUpdate}
          canEdit={canEdit}
          collapsible
        />
      )}
    </div>
  )
}

function MatchSection({
  title,
  count,
  matches,
  onUpdate,
  canEdit,
  accentColor,
  collapsible = false,
  recordable = true,
}: {
  title: string
  count: number
  matches: Match[]
  onUpdate: (m: Match) => void
  canEdit: (m: Match) => boolean
  accentColor?: string
  // Collapsible sections hold completed games; they start collapsed so the
  // page stays focused on what's still to play.
  collapsible?: boolean
  // False for group games once the knockout has started — their results are
  // locked, so unplayed ones must not offer a Record button.
  recordable?: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const collapsed = collapsible && !expanded

  const header = (
    <>
      {accentColor && (
        <span className="w-1.5 h-4 rounded-full shrink-0" style={{ backgroundColor: accentColor }} />
      )}
      <h2 className="text-xs uppercase tracking-widest" style={{ color: accentColor ?? '#777' }}>
        {title}
      </h2>
      <span
        className="text-xs tabular-nums px-1.5 py-0.5 rounded"
        style={{ backgroundColor: 'var(--color-border)', color: '#888' }}
      >
        {count}
      </span>
    </>
  )

  return (
    <section>
      {collapsible ? (
        <button
          onClick={() => setExpanded(e => !e)}
          className={`flex items-center gap-3 w-full text-left ${collapsed ? '' : 'mb-4'}`}
        >
          <span className="text-base leading-none" style={{ color: '#888' }}>
            {collapsed ? '▸' : '▾'}
          </span>
          {header}
        </button>
      ) : (
        <div className="flex items-center gap-3 mb-4">{header}</div>
      )}
      {!collapsed && (
        <div className="rounded-xl overflow-hidden border divide-y" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface-card)' }}>
          {matches.map(m => <MatchRow key={m.id} match={m} onUpdate={onUpdate} canEdit={canEdit(m)} recordable={recordable} />)}
        </div>
      )}
    </section>
  )
}

function MatchRow({ match, onUpdate, canEdit, recordable = true }: { match: Match; onUpdate: (m: Match) => void; canEdit: boolean; recordable?: boolean }) {
  const [recording, setRecording] = useState(false)
  const [saving, setSaving] = useState(false)

  const p1Won = match.winner_id === match.player1_id
  const p2Won = match.winner_id === match.player2_id
  const winnerPoints = p1Won ? match.player1_score : p2Won ? match.player2_score : null

  async function handleResult(winnerID: string, points: number) {
    setSaving(true)
    try {
      const updated = await api.updateMatch(match.id, winnerID, points)
      setRecording(false)
      setSaving(false)
      onUpdate(updated)
    } catch {
      setSaving(false)
    }
  }

  const stageBadge = (
    <span className="text-xs px-1.5 py-0.5 rounded shrink-0" style={{ backgroundColor: 'var(--color-border)', color: '#aaa' }}>
      {match.stage === 'group' ? 'Group' : 'KO'}
    </span>
  )

  if (recording && (match.status !== 'complete' || canEdit)) {
    return (
      <div className="px-4 py-3 space-y-3">
        <div className="flex items-center gap-2">
          {stageBadge}
          <p className="text-xs uppercase tracking-widest" style={{ color: '#666' }}>
            {match.status === 'complete' ? 'Correct result — who won?' : 'Who won?'}
          </p>
          <button
            onClick={() => setRecording(false)}
            disabled={saving}
            className="ml-auto text-xs px-2 py-1 rounded"
            style={{ color: '#666' }}
          >
            ✕
          </button>
        </div>
        <div className="grid grid-cols-2 gap-3">
          {([
            [match.player1_id!, match.player1_name],
            [match.player2_id!, match.player2_name],
          ] as [string, string | null][]).map(([pid, name], col) => (
            <div key={pid} className="space-y-1.5">
              <p className="text-xs truncate font-semibold" style={{ color: '#f0f0f0' }}>{name ?? 'TBD'}</p>
              <div className="flex gap-1">
                {(col === 0 ? [3, 2, 1] : [1, 2, 3]).map(pts => (
                  <button
                    key={pts}
                    onClick={() => handleResult(pid, pts)}
                    disabled={saving}
                    className="flex-1 text-sm font-bold py-1.5 rounded border transition-colors disabled:opacity-40"
                    style={col === 0
                      ? { borderColor: 'var(--color-wax-red)', color: 'var(--color-wax-red)' }
                      : { borderColor: 'var(--color-brand)', color: 'var(--color-brand)' }}
                  >
                    +{pts}
                  </button>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-4 px-4 py-3 text-sm">
      {stageBadge}
      <div className="flex-1 flex items-center gap-2 min-w-0">
        <span className={`truncate ${p1Won ? 'font-semibold' : ''}`} style={{ color: p1Won ? 'var(--color-brand-bright)' : '#f0f0f0' }}>
          {match.player1_name ?? 'TBD'}
        </span>
        <span className="text-xs shrink-0" style={{ color: '#555' }}>
          {match.status === 'complete' ? 'def.' : 'vs'}
        </span>
        <span className={`truncate ${p2Won ? 'font-semibold' : ''}`} style={{ color: p2Won ? 'var(--color-brand-bright)' : '#f0f0f0' }}>
          {match.player2_name ?? 'TBD'}
        </span>
      </div>
      {match.status === 'complete' && winnerPoints !== null && (
        <span className="text-xs tabular-nums shrink-0" style={{ color: '#666' }}>+{winnerPoints}</span>
      )}
      {match.status !== 'complete' && recordable && (
        <button
          onClick={() => setRecording(true)}
          className="text-xs font-bold uppercase tracking-widest px-3 py-1 rounded border shrink-0 transition-colors"
          style={{ borderColor: 'var(--color-border)', color: '#d0d0d0' }}
        >
          Record
        </button>
      )}
      {match.status !== 'complete' && !recordable && (
        <span className="text-xs uppercase tracking-widest shrink-0" style={{ color: '#555' }}>
          not played
        </span>
      )}
      {match.status === 'complete' && canEdit && (
        <button
          onClick={() => setRecording(true)}
          title="Correct this result"
          className="text-xs w-6 h-6 flex items-center justify-center rounded border shrink-0 transition-colors"
          style={{ color: '#888', borderColor: 'var(--color-border)' }}
        >
          ✎
        </button>
      )}
    </div>
  )
}
