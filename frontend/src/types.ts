export type TournamentStatus = 'setup' | 'group_stage' | 'knockout' | 'complete'

export interface Tournament {
  id: string
  name: string
  status: TournamentStatus
  config: string
  created_at: string
}

export interface Competitor {
  id: string
  name: string
  email: string | null
  ticket_tailor_id: string | null
  seed: number | null
  group_id: string | null
  wins: number
  losses: number
}

export interface CompetitorStanding {
  id: string
  name: string
  played: number
  won: number
  lost: number
  points: number
}

export interface Group {
  id: string
  name: string
  competitors: CompetitorStanding[]
}

export type MatchStatus = 'pending' | 'in_progress' | 'complete'

export interface Match {
  id: string
  stage: 'group' | 'knockout'
  group_id: string | null
  round: number | null
  position: number | null
  player1_id: string | null
  player1_name: string | null
  player2_id: string | null
  player2_name: string | null
  winner_id: string | null
  player1_score: number | null
  player2_score: number | null
  status: MatchStatus
}
