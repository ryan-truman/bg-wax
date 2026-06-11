import type { Tournament, Competitor, Group, Match } from './types'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? 'Request failed')
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  getTournament: () => request<Tournament>('/api/tournament'),
  getCompetitors: () => request<Competitor[]>('/api/competitors'),
  getGroups: () => request<Group[]>('/api/groups'),
  getMatches: () => request<Match[]>('/api/matches'),
  getBracket: () => request<Match[]>('/api/bracket'),

  importFromTicketTailor: (apiKey: string, eventName: string) =>
    request<{ count: number; tournament: string }>('/api/tournament/import', {
      method: 'POST',
      body: JSON.stringify({ api_key: apiKey, event_name: eventName }),
    }),

  clearTournament: () =>
    request<void>('/api/tournament/clear', { method: 'POST' }),

  runDraw: () => request<void>('/api/tournament/draw', { method: 'POST' }),

  advance: () => request<void>('/api/tournament/advance', { method: 'POST' }),

  updateMatch: (id: string, data: { player1_score: number; player2_score: number; winner_id: string }) =>
    request<Match>(`/api/matches/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    }),
}
