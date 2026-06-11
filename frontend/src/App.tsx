import { useState, useEffect } from 'react'
import { Routes, Route, Link, useLocation } from 'react-router-dom'
import { api } from './api'
import type { Tournament } from './types'
import HomePage from './pages/HomePage'
import SettingsPage from './pages/SettingsPage'

export default function App() {
  const [tournament, setTournament] = useState<Tournament | null>(null)

  function handleTournamentUpdate(t: Tournament | null) {
    setTournament(t)
  }
  const { pathname } = useLocation()

  useEffect(() => {
    api.getTournament().then(setTournament).catch(() => {})
  }, [])

  const statusLabel: Record<string, string> = {
    setup: 'Setup',
    group_stage: 'Group Stage',
    knockout: 'Knockout',
    complete: 'Complete',
  }

  const onSettings = pathname === '/settings'

  return (
    <div className="min-h-screen text-white" style={{ backgroundColor: 'var(--color-surface-base)' }}>
      <header className="px-6 py-4 border-b" style={{ backgroundColor: '#2a2a2a', borderColor: 'var(--color-border)' }}>
        <div className="max-w-7xl mx-auto flex items-center justify-between">
          {/* Logo + title — clicking returns to tournament */}
          <Link to="/" className="flex items-center gap-4 no-underline">
            <img src="/logo.png" alt="" className="w-10 h-10 shrink-0" />
            <div>
              <h1 className="text-lg font-black uppercase tracking-wider leading-none text-white">
                Backgammon and Wax
              </h1>
              <p className="text-xs font-semibold uppercase tracking-widest mt-0.5" style={{ color: '#a0a0a0' }}>
                Breaks, takes and the highest of stakes
              </p>
            </div>
          </Link>

          {/* Right side: status badge + settings button */}
          <div className="flex items-center gap-4">
            {tournament && (
              <span
                className="text-xs uppercase tracking-widest px-2 py-1 rounded border"
                style={{ color: 'var(--color-brand)', borderColor: 'var(--color-brand)', opacity: 0.8 }}
              >
                {statusLabel[tournament.status] ?? tournament.status}
              </span>
            )}
            <Link
              to={onSettings ? '/' : '/settings'}
              className="text-xs font-bold uppercase tracking-widest px-3 py-1.5 rounded border transition-colors"
              style={{
                borderColor: 'var(--color-border)',
                color: onSettings ? 'var(--color-brand)' : '#d0d0d0',
              }}
            >
              {onSettings ? '← Back' : 'Settings'}
            </Link>
          </div>
        </div>
      </header>

      <main className="px-6 py-8 max-w-7xl mx-auto">
        <Routes>
          <Route path="/" element={<HomePage tournament={tournament} onUpdate={handleTournamentUpdate} />} />
          <Route path="/settings" element={<SettingsPage tournament={tournament} onUpdate={handleTournamentUpdate} />} />
        </Routes>
      </main>
    </div>
  )
}
