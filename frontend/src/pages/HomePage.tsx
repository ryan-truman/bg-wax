import { Link } from 'react-router-dom'
import type { Tournament } from '../types'
import BracketView from '../components/BracketView'
import GroupStage from '../components/GroupStage'

interface Props {
  tournament: Tournament | null
}

export default function HomePage({ tournament }: Props) {
  if (!tournament) {
    return (
      <div className="flex flex-col items-center justify-center py-32 gap-3 text-center">
        <p className="text-2xl font-bold tracking-widest uppercase" style={{ color: '#444' }}>No Tournament</p>
        <p className="text-sm" style={{ color: '#888' }}>Import competitors from Ticket Tailor to get started.</p>
        <Link to="/settings" className="mt-2 text-sm transition-colors" style={{ color: 'var(--color-brand)' }}>
          Go to Settings →
        </Link>
      </div>
    )
  }

  if (tournament.status === 'setup') {
    return (
      <div className="flex flex-col items-center justify-center py-32 gap-3 text-center">
        <p className="text-2xl font-bold tracking-widest uppercase" style={{ color: '#444' }}>Setup</p>
        <p className="text-sm" style={{ color: '#888' }}>Tournament not yet started. Run the draw to begin the group stage.</p>
        <Link to="/matches" className="mt-2 text-sm transition-colors" style={{ color: 'var(--color-brand)' }}>
          Go to Match History →
        </Link>
      </div>
    )
  }

  if (tournament.status === 'group_stage') {
    return <GroupStage />
  }

  // Knockout and beyond: the bracket leads, with the finished group stage
  // tucked away underneath it.
  return (
    <div className="space-y-12">
      <BracketView />
      <GroupStage collapsible />
    </div>
  )
}
