-- Phones: web push subscriptions (a player's phone, without Pocket Alerts).
CREATE TABLE push_subs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  endpoint   TEXT NOT NULL UNIQUE,
  p256dh     TEXT NOT NULL,
  auth       TEXT NOT NULL,
  agent      TEXT NOT NULL DEFAULT '',
  fails      INTEGER NOT NULL DEFAULT 0,
  last_ok    TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX push_subs_user ON push_subs(user_id);

-- Someone's friend for one game (+1), not in the group.
CREATE TABLE game_guests (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  game_id    INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  added_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  name       TEXT NOT NULL,
  answer     TEXT NOT NULL DEFAULT 'in',          -- in | waiting
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX game_guests_game ON game_guests(game_id);

-- Photos in the chat.
CREATE TABLE attachments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id   INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  message_id INTEGER REFERENCES messages(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  mime       TEXT NOT NULL,
  bytes      INTEGER NOT NULL DEFAULT 0,
  width      INTEGER NOT NULL DEFAULT 0,
  height     INTEGER NOT NULL DEFAULT 0,
  path       TEXT NOT NULL,
  thumb      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX attachments_group ON attachments(group_id);

-- Who owes what: charges (a share of the court, an entry fee) and payments.
CREATE TABLE ledger (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id      INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind          TEXT NOT NULL,                    -- charge | payment
  amount_cents  INTEGER NOT NULL,                 -- always positive
  currency      TEXT NOT NULL DEFAULT 'EUR',
  game_id       INTEGER REFERENCES games(id) ON DELETE SET NULL,
  tournament_id INTEGER REFERENCES tournaments(id) ON DELETE SET NULL,
  note          TEXT NOT NULL DEFAULT '',
  created_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  confirmed_by  INTEGER REFERENCES users(id) ON DELETE SET NULL, -- a payment a player says they made, until an organiser confirms
  confirmed_at  TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX ledger_group ON ledger(group_id, user_id);
CREATE UNIQUE INDEX ledger_game_once ON ledger(game_id, user_id, kind) WHERE game_id IS NOT NULL AND kind = 'charge';
CREATE UNIQUE INDEX ledger_t_once ON ledger(tournament_id, user_id, kind) WHERE tournament_id IS NOT NULL AND kind = 'charge';

ALTER TABLE groups ADD COLUMN pay_note TEXT NOT NULL DEFAULT '';       -- how to pay: an IBAN, a Revolut link
ALTER TABLE groups ADD COLUMN charge_games INTEGER NOT NULL DEFAULT 0; -- charge each player their share after a game
ALTER TABLE groups ADD COLUMN rating_scope TEXT NOT NULL DEFAULT 'all'; -- all | season: ratings start fresh each season
ALTER TABLE groups ADD COLUMN stats TEXT NOT NULL DEFAULT '[]';        -- the stats kept per player ("assists", "cards" …)

-- Seasons: a table and awards frozen when one closes.
CREATE TABLE seasons (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id  INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  name      TEXT NOT NULL,
  starts_on TEXT NOT NULL,
  ends_on   TEXT NOT NULL DEFAULT '',             -- '' = still on
  closed_at TEXT,
  snapshot  TEXT NOT NULL DEFAULT '{}',           -- the final table
  awards    TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX seasons_group ON seasons(group_id, starts_on);

-- Reliability: every change of an answer, and who did not come.
CREATE TABLE answer_log (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  from_a  TEXT NOT NULL DEFAULT '',
  to_a    TEXT NOT NULL DEFAULT '',
  at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  late    INTEGER NOT NULL DEFAULT 0              -- dropped out less than a day before
);
CREATE INDEX answer_log_user ON answer_log(user_id, game_id);
ALTER TABLE answers ADD COLUMN no_show INTEGER NOT NULL DEFAULT 0;

-- Finding a sub: a place offered to someone, first to take it plays.
CREATE TABLE sub_offers (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  game_id    INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  offered_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  taken_at   TEXT,
  UNIQUE (game_id, user_id)
);

-- Chores: who brings the balls, books the court … taking turns.
CREATE TABLE duties (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  name     TEXT NOT NULL,
  position INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE game_duties (
  game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  duty_id INTEGER NOT NULL REFERENCES duties(id) ON DELETE CASCADE,
  user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
  done    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (game_id, duty_id)
);

-- More per player and match: assists, cards, aces …
ALTER TABLE match_players ADD COLUMN stats TEXT NOT NULL DEFAULT '{}';

-- Live scoring, point by point.
CREATE TABLE live_scores (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id    INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  game_id     INTEGER REFERENCES games(id) ON DELETE CASCADE,
  tmatch_id   INTEGER REFERENCES t_matches(id) ON DELETE CASCADE,
  a_name      TEXT NOT NULL,
  b_name      TEXT NOT NULL,
  a_ids       TEXT NOT NULL DEFAULT '[]',
  b_ids       TEXT NOT NULL DEFAULT '[]',
  rules       TEXT NOT NULL DEFAULT '{}',
  state       TEXT NOT NULL DEFAULT '{}',
  scorer      INTEGER REFERENCES users(id) ON DELETE SET NULL,
  started_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  finished_at TEXT
);
CREATE INDEX live_scores_group ON live_scores(group_id, finished_at);

-- When someone can't play (holidays, an injury).
CREATE TABLE away (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  from_day   TEXT NOT NULL,
  to_day     TEXT NOT NULL,
  note       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX away_user ON away(user_id, from_day);
