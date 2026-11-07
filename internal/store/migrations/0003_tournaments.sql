-- Tournaments: a one-day knockout, an Americano night, a season's ladder.
-- A tournament has stages (groups, then a knockout); a stage has groups
-- (pools, or the brackets of a knockout); matches sit in a stage and a
-- group, by round. Who plays is an entry (a player, a pair, a team); in
-- formats where partners change every round the entries on each side of
-- a match are in a_entries / b_entries. Everyone in the group sees its tournaments;
-- its organisers (and whoever made it) run them.
CREATE TABLE tournaments (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id     INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id      INTEGER REFERENCES users(id) ON DELETE SET NULL,  -- who made it
  name         TEXT NOT NULL,
  format       TEXT NOT NULL,                     -- the preset: round_robin | knockout | groups_knockout | americano | mexicano | ranking | kotc | double_elim | swiss | team_ties | ladder | box_league | arena
  entry_kind   TEXT NOT NULL DEFAULT 'player',    -- player | pair | team
  status       TEXT NOT NULL DEFAULT 'signup',    -- signup | live | finished | cancelled
  rated        INTEGER NOT NULL DEFAULT 1,        -- results count in the group's table and ratings
  starts_at    TEXT NOT NULL DEFAULT '',
  ends_at      TEXT NOT NULL DEFAULT '',          -- arena: when pairing stops; ladder: the season's end
  venue_name   TEXT NOT NULL DEFAULT '',
  capacity     INTEGER NOT NULL DEFAULT 0,        -- 0 = no limit
  signup_until TEXT NOT NULL DEFAULT '',
  fee_cents    INTEGER NOT NULL DEFAULT 0,
  currency     TEXT NOT NULL DEFAULT 'EUR',
  checkin      INTEGER NOT NULL DEFAULT 0,        -- only checked-in entries are drawn
  share_token  TEXT NOT NULL UNIQUE,              -- the read-only page for anyone with the link
  settings     TEXT NOT NULL DEFAULT '{}',        -- format settings (points per match, groups, tie-breaks …)
  game_id      INTEGER REFERENCES games(id) ON DELETE SET NULL, -- the meeting it is played at
  winner       TEXT NOT NULL DEFAULT '',          -- the name, once finished
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX tournaments_group ON tournaments(group_id, created_at);

CREATE TABLE t_courts (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  position      INTEGER NOT NULL DEFAULT 0,
  active        INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE t_entries (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'in',       -- in | waiting | withdrawn | disqualified
  seed          INTEGER NOT NULL DEFAULT 0,       -- 0 = by rating / the draw
  rating        REAL NOT NULL DEFAULT 1000,       -- at the draw
  handicap      REAL,                             -- golf / bowling
  position      INTEGER NOT NULL DEFAULT 0,       -- ladder
  protected_until TEXT NOT NULL DEFAULT '',       -- ladder: can't be challenged until
  paid          INTEGER NOT NULL DEFAULT 0,
  checked_in    INTEGER NOT NULL DEFAULT 0,
  note          TEXT NOT NULL DEFAULT '',
  created_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  withdrawn_at  TEXT
);
CREATE INDEX t_entries_t ON t_entries(tournament_id);

CREATE TABLE t_entry_members (
  entry_id INTEGER NOT NULL REFERENCES t_entries(id) ON DELETE CASCADE,
  user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role     TEXT NOT NULL DEFAULT 'player',       -- player | captain | sub
  position INTEGER NOT NULL DEFAULT 0,           -- order of merit in a team
  PRIMARY KEY (entry_id, user_id)
);
CREATE INDEX t_entry_members_user ON t_entry_members(user_id);

CREATE TABLE t_stages (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
  seq           INTEGER NOT NULL,
  kind          TEXT NOT NULL,                    -- round_robin | knockout | double_elim | swiss | americano | mexicano | kotc | ranking | team_ties | ladder | arena
  name          TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'pending',  -- pending | live | done
  settings      TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE t_groups (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  stage_id INTEGER NOT NULL REFERENCES t_stages(id) ON DELETE CASCADE,
  name     TEXT NOT NULL,                         -- Group A | Main draw | Plate | Losers' bracket | Division 1
  kind     TEXT NOT NULL DEFAULT 'pool',          -- pool | main | third | plate | wb | lb | final
  position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE t_stage_entries (
  stage_id   INTEGER NOT NULL REFERENCES t_stages(id) ON DELETE CASCADE,
  group_id   INTEGER REFERENCES t_groups(id) ON DELETE CASCADE,
  entry_id   INTEGER NOT NULL REFERENCES t_entries(id) ON DELETE CASCADE,
  seed       INTEGER NOT NULL DEFAULT 0,
  final_rank INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (stage_id, entry_id)
);

CREATE TABLE t_matches (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
  stage_id      INTEGER NOT NULL REFERENCES t_stages(id) ON DELETE CASCADE,
  group_id      INTEGER REFERENCES t_groups(id) ON DELETE CASCADE,
  parent_id     INTEGER REFERENCES t_matches(id) ON DELETE CASCADE, -- a rubber of a team tie
  round         INTEGER NOT NULL DEFAULT 1,
  position      INTEGER NOT NULL DEFAULT 0,       -- its place in the round (brackets)
  label         TEXT NOT NULL DEFAULT '',         -- "Final", "Semi-final 1", "Singles 2", "Court 1"
  a_entry       INTEGER REFERENCES t_entries(id) ON DELETE SET NULL,
  b_entry       INTEGER REFERENCES t_entries(id) ON DELETE SET NULL,
  a_source      TEXT NOT NULL DEFAULT '',         -- before it is known: "Winner of 12", "A1", "bye"
  a_entries     TEXT NOT NULL DEFAULT '[]',       -- rotating formats: the entries (players) on each side
  b_entries     TEXT NOT NULL DEFAULT '[]',
  b_source      TEXT NOT NULL DEFAULT '',
  next_match    INTEGER REFERENCES t_matches(id) ON DELETE SET NULL,
  next_slot     TEXT NOT NULL DEFAULT '',         -- a | b
  loser_match   INTEGER REFERENCES t_matches(id) ON DELETE SET NULL,
  loser_slot    TEXT NOT NULL DEFAULT '',
  court_id      INTEGER REFERENCES t_courts(id) ON DELETE SET NULL,
  scheduled_at  TEXT NOT NULL DEFAULT '',
  called_at     TEXT,
  status        TEXT NOT NULL DEFAULT 'pending',  -- pending | ready | called | reported | done | void
  outcome       TEXT NOT NULL DEFAULT '',         -- normal | walkover | retired | bye | double_walkover | no_show
  winner        TEXT NOT NULL DEFAULT '',         -- a | b | draw
  score         TEXT NOT NULL DEFAULT '{}',       -- like a League result: {"sets":[[6,4]]} | {"a":3,"b":2}
  a_total       REAL,                             -- points of the side (Americano: the points scored)
  b_total       REAL,
  white         TEXT NOT NULL DEFAULT '',         -- chess: the side with white
  deadline      TEXT NOT NULL DEFAULT '',         -- ladder: play by
  reported_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  reported_at   TEXT,
  confirmed_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
  confirmed_at  TEXT,
  dispute       TEXT NOT NULL DEFAULT '',
  league_match  INTEGER REFERENCES matches(id) ON DELETE SET NULL, -- the result in the group's table (rated)
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX t_matches_t ON t_matches(tournament_id, stage_id, round, position);

-- Scored rounds (golf, bowling, a darts ranking night): one card per entry per round.
CREATE TABLE t_scorecards (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  stage_id   INTEGER NOT NULL REFERENCES t_stages(id) ON DELETE CASCADE,
  round      INTEGER NOT NULL DEFAULT 1,
  entry_id   INTEGER NOT NULL REFERENCES t_entries(id) ON DELETE CASCADE,
  gross      REAL,
  holes      TEXT NOT NULL DEFAULT '[]',          -- golf: strokes per hole; bowling: pins per game
  entered_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (stage_id, round, entry_id)
);

-- Ladder challenges.
CREATE TABLE t_challenges (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
  challenger    INTEGER NOT NULL REFERENCES t_entries(id) ON DELETE CASCADE,
  defender      INTEGER NOT NULL REFERENCES t_entries(id) ON DELETE CASCADE,
  status        TEXT NOT NULL DEFAULT 'open',     -- open | accepted | played | declined | expired | cancelled
  respond_by    TEXT NOT NULL DEFAULT '',
  play_by       TEXT NOT NULL DEFAULT '',
  match_id      INTEGER REFERENCES t_matches(id) ON DELETE SET NULL,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- What happened, for the record (and to undo a result).
CREATE TABLE t_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  tournament_id INTEGER NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
  user_id       INTEGER REFERENCES users(id) ON DELETE SET NULL,
  action        TEXT NOT NULL,
  detail        TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX t_log_t ON t_log(tournament_id, id);
