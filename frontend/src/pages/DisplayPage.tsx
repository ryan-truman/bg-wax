import { useEffect } from 'react'
import type { Tournament } from '../types'
import GroupStage from '../components/GroupStage'
import BracketView from '../components/BracketView'

interface Props {
  tournament: Tournament | null
}

// DisplayPage is the tournament view stripped of navigation and admin controls,
// for a shared screen. It is a plain route rather than a copy of the window, so
// the organiser can carry on recording results next door: everything here polls
// and updates itself.
export default function DisplayPage({ tournament }: Props) {
  useEffect(() => {
    document.title = 'Backgammon and Wax — Tournament'
  }, [])

  return (
    <div className="min-h-screen text-white" style={{ backgroundColor: 'var(--color-surface-base)' }}>
      <header
        className="px-6 py-3 border-b flex items-center justify-center gap-3"
        style={{ backgroundColor: '#2a2a2a', borderColor: 'var(--color-border)' }}
      >
        <img src="/logo.png" alt="" className="w-8 h-8 shrink-0" />
        <h1 className="text-base font-black uppercase tracking-wider leading-none text-white">
          Backgammon and Wax
        </h1>
      </header>

      <main className="px-6 py-6">
        <Body tournament={tournament} />
      </main>
    </div>
  )
}

function Body({ tournament }: { tournament: Tournament | null }) {
  if (!tournament || tournament.status === 'setup') {
    return (
      <div className="flex flex-col items-center justify-center py-32 gap-3 text-center">
        <p className="text-2xl font-bold tracking-widest uppercase" style={{ color: '#444' }}>
          {tournament ? 'Setup' : 'No Tournament'}
        </p>
        <p className="text-sm" style={{ color: '#888' }}>
          {tournament
            ? 'Waiting for the draw to be run.'
            : 'Waiting for competitors to be imported.'}
        </p>
      </div>
    )
  }

  if (tournament.status === 'group_stage') {
    return <GroupStage />
  }

  // Only the bracket goes on the shared screen. The finished group stage is
  // kept to the main window, where the organiser can expand it on demand —
  // nobody is going to click a collapsed heading on a screen across the room.
  return <BracketView />
}
