import { useState, useEffect } from 'react'
import { Routes, Route, Link, NavLink, useLocation } from 'react-router-dom'
import { api } from './api'
import type { Tournament } from './types'
import HomePage from './pages/HomePage'
import MatchHistoryPage from './pages/MatchHistoryPage'
import CompetitorsPage from './pages/CompetitorsPage'
import SettingsPage from './pages/SettingsPage'

export default function App() {
  const [tournament, setTournament] = useState<Tournament | null>(null)

  function handleTournamentUpdate(t: Tournament | null) {
    setTournament(t)
  }
  const { pathname } = useLocation()

  // Poll the tournament so external changes (a reset or import from another
  // device, a reseeded database) are picked up without a reload. A 404 means
  // no tournament exists — clear it rather than keeping a stale one.
  useEffect(() => {
    const load = () =>
      api.getTournament().then(setTournament).catch(() => setTournament(null))
    load()
    const timer = setInterval(load, 5000)
    return () => clearInterval(timer)
  }, [])

  const onSettings = pathname === '/settings'

  return (
    <div className="min-h-screen text-white" style={{ backgroundColor: 'var(--color-surface-base)' }}>
      <header className="px-6 py-4 border-b" style={{ backgroundColor: '#2a2a2a', borderColor: 'var(--color-border)' }}>
        <div className="max-w-7xl mx-auto flex items-center justify-between">
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

          <div className="flex items-center gap-4">
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

        {!onSettings && (
          <div className="max-w-7xl mx-auto mt-4 flex">
            {[
              { to: '/', label: 'Tournament' },
              { to: '/matches', label: 'Match History' },
              { to: '/competitors', label: 'Competitors' },
            ].map(({ to, label }) => (
              <NavLink
                key={to}
                to={to}
                end
                className={({ isActive }) =>
                  `flex-1 text-center text-xs font-bold uppercase tracking-widest px-3 py-1.5 rounded transition-colors ${
                    isActive
                      ? 'text-white'
                      : 'text-gray-500 hover:text-gray-300'
                  }`
                }
                style={({ isActive }) =>
                  isActive
                    ? { backgroundColor: 'var(--color-border)', color: '#fff' }
                    : {}
                }
              >
                {label}
              </NavLink>
            ))}
          </div>
        )}
      </header>

      <main className="px-6 py-8 max-w-7xl mx-auto">
        <Routes>
          <Route path="/" element={<HomePage tournament={tournament} />} />
          <Route path="/matches" element={<MatchHistoryPage tournament={tournament} onUpdate={handleTournamentUpdate} />} />
          <Route path="/competitors" element={<CompetitorsPage />} />
          <Route path="/settings" element={<SettingsPage tournament={tournament} onUpdate={handleTournamentUpdate} />} />
        </Routes>
      </main>
    </div>
  )
}
