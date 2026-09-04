package api

// Single source of truth for the API's JSON shapes: frontend/src/types.ts is
// generated from these structs by tygo (see tygo.yaml and the `types` recipe),
// so the json tags must stay in sync with what the frontend expects.

// TournamentStatus tracks where a tournament is in its lifecycle. These consts
// are the canonical set of values; tygo renders them as a TypeScript
// string-literal union (names must carry the type prefix for it to detect them).
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
	GroupName    *string     `json:"group_name"`
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

// TieBreak is a tie the ranking rules cannot settle where the order decides who
// progresses, reported so the organiser can choose. Only outcome-changing ties
// are raised: players level on points who all qualify anyway keep the order the
// standings gave them.
type TieBreak struct {
	ID string `json:"id"`
	// "group" for a tie within one group's standings, or "pool" for one between
	// the next-place finishers of different groups competing for the last few
	// places — those players have never met, so head-to-head cannot help.
	Scope     string `json:"scope"`
	GroupName string `json:"group_name"`
	Bracket   int    `json:"bracket"`
	// 1-based finishing place the tie starts at; for a "pool" tie this is only a
	// position within the pool.
	Place int `json:"place"`
	// How many of the tied players take the remaining qualifying places.
	Slots  int `json:"slots"`
	Points int `json:"points"`
	// Where the players who miss out land, so the organiser can see what they
	// are deciding. A group tie may drop them into the runners-up pool for the
	// same bracket; otherwise they fall to DropsToBracket, which is 0 when there
	// is no bracket below and missing out means not progressing at all.
	DropsToPool    bool           `json:"drops_to_pool"`
	DropsToBracket int            `json:"drops_to_bracket"`
	Competitors    []TieCandidate `json:"competitors"`
}

// TieCandidate is one competitor caught in a TieBreak, with the group-stage
// record shown alongside their name so the organiser can judge the call.
type TieCandidate struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	GroupName string `json:"group_name"`
	Played    int    `json:"played"`
	Won       int    `json:"won"`
	Points    int    `json:"points"`
}

// TicketTailorEvent is one event on the connected Ticket Tailor account,
// listed so the user can pick which event to import attendees from.
type TicketTailorEvent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Settings are organiser preferences persisted in the database, so they survive
// restarts and apply wherever the buttons are pressed — the Settings page
// configures behaviour, the Match History page carries the buttons.
type Settings struct {
	// Minimum group-stage games per player: the automatic draw sizes groups at
	// MinGroupGames+1, and an uneven remainder makes some groups one player
	// bigger — an extra game, never a shortfall.
	MinGroupGames int `json:"min_group_games"`
	// Fixed group count overriding the automatic sizing (0 = automatic).
	NumGroups int `json:"num_groups"`
	// Finishers advancing into each knockout bracket in total: the top places
	// from every group, with the best next-place finishers filling any remainder
	// (16 from 6 groups = top 2 each + the 4 best thirds). Must be a power of
	// two (see advanceTotals) so the bracket fills exactly.
	AdvanceTotal int `json:"advance_total"`
	// Put all qualifiers in one bracket rather than splitting into Champion's
	// and Europa leagues.
	SingleBracket bool `json:"single_bracket"`
}

// Config reports server-side runtime flags the frontend adapts to. Demo is
// true when the server was started by `just demo`; the Settings page then
// pre-fills the built-in demo API credentials if none are saved.
type Config struct {
	Demo bool `json:"demo"`
}
