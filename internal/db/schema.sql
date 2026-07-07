CREATE TABLE IF NOT EXISTS tournaments (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'setup',
    config_json TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- status values: 'setup' | 'group_stage' | 'knockout' | 'complete'
-- config_json is unused: organiser preferences live in the settings table,
-- which survives re-imports (this row is replaced wholesale on import).

CREATE TABLE IF NOT EXISTS competitors (
    id                TEXT PRIMARY KEY,
    tournament_id     TEXT NOT NULL REFERENCES tournaments(id),
    name              TEXT NOT NULL,
    email             TEXT,
    ticket_tailor_id  TEXT,
    order_id          TEXT,  -- Ticket Tailor order; tickets bought together share one, and the draw keeps them in different groups
    seed              INTEGER,
    group_id          TEXT REFERENCES groups(id),
    removed           INTEGER NOT NULL DEFAULT 0,
    name_edited       INTEGER NOT NULL DEFAULT 0,  -- renamed in-app; the local name wins over the ticket's on re-import
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS groups (
    id            TEXT PRIMARY KEY,
    tournament_id TEXT NOT NULL REFERENCES tournaments(id),
    name          TEXT NOT NULL  -- 'Group A', 'Group B', …
);

CREATE TABLE IF NOT EXISTS matches (
    id            TEXT PRIMARY KEY,
    tournament_id TEXT NOT NULL REFERENCES tournaments(id),
    stage         TEXT NOT NULL,  -- 'group' | 'knockout'
    group_id      TEXT REFERENCES groups(id),  -- NULL for knockout matches
    bracket       INTEGER,        -- knockout: 1 = main bracket, 2 = consolation; NULL for group matches
    round         INTEGER,        -- knockout: 1 = final, 2 = semis, etc.
    position      INTEGER,        -- slot within the round for bracket ordering
    player1_id    TEXT REFERENCES competitors(id),
    player2_id    TEXT REFERENCES competitors(id),
    winner_id     TEXT REFERENCES competitors(id),
    player1_score INTEGER,
    player2_score INTEGER,
    status        TEXT NOT NULL DEFAULT 'pending',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- status values: 'pending' | 'in_progress' | 'complete'

-- Organiser preferences that persist across sessions (e.g. the target number
-- of group-stage games per player). One row per setting, keyed by name.
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
