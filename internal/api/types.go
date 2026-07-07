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
	Points         int     `json:"points"`
}

// RemovedCompetitor is a competitor who has been removed from the tournament;
// they can be restored from the competitors page.
type RemovedCompetitor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
// Bracket is set for knockout matches only: 1 = main bracket, 2 = consolation.
type Match struct {
	ID           string      `json:"id"`
	Stage        MatchStage  `json:"stage"`
	GroupID      *string     `json:"group_id"`
	Bracket      *int        `json:"bracket"`
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

// TicketTailorEvent is one event on the connected Ticket Tailor account,
// listed so the user can pick which event to import attendees from.
type TicketTailorEvent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Settings are organiser preferences persisted in the database, so they
// survive restarts and apply wherever the buttons are pressed — the Settings
// page configures behaviour, the Match History page carries the buttons.
//
// MinGroupGames is the number of group-stage games each player must get at
// minimum: the automatic draw sizes groups at MinGroupGames+1 players, and
// when the numbers don't divide evenly the remainder makes some groups one
// player bigger — an extra game, never a shortfall. NumGroups overrides the
// automatic sizing with a fixed group count (0 = automatic). AdvancePerGroup
// is how many top finishers each group sends to the knockout. SingleBracket
// puts all qualifiers in one bracket instead of splitting into Champion's and
// Europa leagues.
type Settings struct {
	MinGroupGames   int  `json:"min_group_games"`
	NumGroups       int  `json:"num_groups"`
	AdvancePerGroup int  `json:"advance_per_group"`
	SingleBracket   bool `json:"single_bracket"`
}

// Config reports server-side runtime flags the frontend adapts to. Demo is
// true when the server was started by `just demo`; the Settings page then
// pre-fills the built-in demo API credentials if none are saved.
type Config struct {
	Demo bool `json:"demo"`
}
