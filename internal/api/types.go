package api

// This file is the single source of truth for the JSON shapes returned by the
// API. The TypeScript definitions in frontend/src/types.ts are generated from
// these structs by tygo (see tygo.yaml + the `types` recipe in the justfile),
// so the json tags here must stay in sync with what the frontend expects.

// TournamentStatus tracks where a tournament is in its lifecycle. These consts
// are the canonical set of valid values; tygo turns them into the matching
// TypeScript string-literal union (the const names must be prefixed with the
// type name for tygo's enum detection to fire).
type TournamentStatus string

const (
	TournamentStatusSetup      TournamentStatus = "setup"
	TournamentStatusGroupStage TournamentStatus = "group_stage"
	TournamentStatusKnockout   TournamentStatus = "knockout"
	TournamentStatusComplete   TournamentStatus = "complete"
)

// MatchStatus tracks whether a single match has been played.
type MatchStatus string

const (
	MatchStatusPending    MatchStatus = "pending"
	MatchStatusInProgress MatchStatus = "in_progress"
	MatchStatusComplete   MatchStatus = "complete"
)

// MatchStage distinguishes group-stage fixtures from knockout fixtures.
type MatchStage string

const (
	MatchStageGroup    MatchStage = "group"
	MatchStageKnockout MatchStage = "knockout"
)

// Tournament is the latest tournament's top-level state.
type Tournament struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Status    TournamentStatus `json:"status"`
	Config    string           `json:"config"`
	CreatedAt string           `json:"created_at"`
}

// Competitor is a registered player plus their aggregate win/loss record.
type Competitor struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Email          *string `json:"email"`
	TicketTailorID *string `json:"ticket_tailor_id"`
	Seed           *int    `json:"seed"`
	GroupID        *string `json:"group_id"`
	Wins           int     `json:"wins"`
	Losses         int     `json:"losses"`
}

// CompetitorStanding is one row of a group's standings table.
type CompetitorStanding struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Played int    `json:"played"`
	Won    int    `json:"won"`
	Lost   int    `json:"lost"`
	Points int    `json:"points"`
}

// Group is a draw group with its current standings.
type Group struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Competitors []CompetitorStanding `json:"competitors"`
}

// Match is a single fixture in either the group or knockout stage. Player and
// score fields are nullable because a knockout slot may not be filled yet.
type Match struct {
	ID           string      `json:"id"`
	Stage        MatchStage  `json:"stage"`
	GroupID      *string     `json:"group_id"`
	Round        *int        `json:"round"`
	Position     *int        `json:"position"`
	Player1ID    *string     `json:"player1_id"`
	Player1Name  *string     `json:"player1_name"`
	Player2ID    *string     `json:"player2_id"`
	Player2Name  *string     `json:"player2_name"`
	WinnerID     *string     `json:"winner_id"`
	Player1Score *int        `json:"player1_score"`
	Player2Score *int        `json:"player2_score"`
	Status       MatchStatus `json:"status"`
}
