import type { Tournament, Competitor, RemovedCompetitor, Group, Match, TicketTailorEvent, Config } from './types'

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
  getConfig: () => request<Config>('/api/config'),
  getTournament: () => request<Tournament>('/api/tournament'),
  getCompetitors: () => request<Competitor[]>('/api/competitors'),
  getRemovedCompetitors: () => request<RemovedCompetitor[]>('/api/competitors/removed'),
  restoreCompetitor: (id: string) =>
    request<void>(`/api/competitors/${id}/restore`, { method: 'POST' }),
  getGroups: () => request<Group[]>('/api/groups'),
  getMatches: () => request<Match[]>('/api/matches'),
  getBracket: () => request<Match[]>('/api/bracket'),

  listTicketTailorEvents: (apiKey: string) =>
    request<TicketTailorEvent[]>('/api/tickettailor/events', {
      method: 'POST',
      body: JSON.stringify({ api_key: apiKey }),
    }),

  importFromTicketTailor: (apiKey: string, eventId: string) =>
    request<{ count: number; tournament: string }>('/api/tournament/import', {
      method: 'POST',
      body: JSON.stringify({ api_key: apiKey, event_id: eventId }),
    }),

  clearTournament: () =>
    request<void>('/api/tournament/clear', { method: 'POST' }),

  runDraw: (numGroups: number) =>
    request<void>('/api/tournament/draw', {
      method: 'POST',
      body: JSON.stringify({ num_groups: numGroups }),
    }),

  deleteCompetitor: (id: string) =>
    request<void>(`/api/competitors/${id}`, { method: 'DELETE' }),

  advance: (advanceTotal: number, singleBracket: boolean) =>
    request<void>('/api/tournament/advance', {
      method: 'POST',
      body: JSON.stringify({ advance_total: advanceTotal, single_bracket: singleBracket }),
    }),

  updateMatch: (id: string, winner_id: string, points: number) =>
    request<Match>(`/api/matches/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ winner_id, points }),
    }),
}
