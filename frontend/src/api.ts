import type { Tournament, Competitor, RemovedCompetitor, Group, Match, TicketTailorEvent, Config, Settings, TieBreak } from './types'

// TieBreakRequired is thrown when advancing hits a tie the ranking rules can't
// settle. It carries the tied players so the caller can ask the organiser to
// choose, then repeat the advance with their answer.
export class TieBreakRequired extends Error {
  ties: TieBreak[]
  constructor(ties: TieBreak[]) {
    super('A tie needs to be settled before advancing')
    this.name = 'TieBreakRequired'
    this.ties = ties
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    if (res.status === 409 && Array.isArray(body.ties)) throw new TieBreakRequired(body.ties)
    throw new Error(body.error ?? 'Request failed')
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  getConfig: () => request<Config>('/api/config'),
  getSettings: () => request<Settings>('/api/settings'),
  // Partial updates are fine — the server merges over the stored settings.
  updateSettings: (patch: Partial<Settings>) =>
    request<void>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
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

  // Group sizing comes from the persisted settings; the draw takes no
  // parameters.
  runDraw: () =>
    request<void>('/api/tournament/draw', { method: 'POST' }),

  deleteCompetitor: (id: string) =>
    request<void>(`/api/competitors/${id}`, { method: 'DELETE' }),

  renameCompetitor: (id: string, name: string) =>
    request<void>(`/api/competitors/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ name }),
    }),

  // Moves a player into another group after the draw. Their fixtures are
  // rebuilt against the new group, which discards any result they had already
  // recorded — confirm with the organiser first when they have played.
  moveCompetitor: (id: string, groupId: string) =>
    request<void>(`/api/competitors/${id}/group`, {
      method: 'POST',
      body: JSON.stringify({ group_id: groupId }),
    }),

  // Qualifiers per bracket and bracket layout come from the persisted settings.
  // tieBreaks maps a tie ID to the finishing order the organiser chose; it is
  // only needed after an advance threw TieBreakRequired.
  advance: (tieBreaks?: Record<string, string[]>) =>
    request<void>('/api/tournament/advance', {
      method: 'POST',
      body: JSON.stringify({ tie_breaks: tieBreaks ?? {} }),
    }),

  updateMatch: (id: string, winner_id: string, points: number) =>
    request<Match>(`/api/matches/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ winner_id, points }),
    }),
}
