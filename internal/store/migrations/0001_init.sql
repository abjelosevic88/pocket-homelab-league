CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT UNIQUE,                    -- NULL for a player added with a phone number only
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',  -- admin | member | player (an account made for a group, signs in with a link)
  phone         TEXT NOT NULL DEFAULT '',
  invited_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  last_seen_at  TEXT,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);


-- ---------- Pocket League ----------
--
-- A group is a few people who play one sport together. Who sees a group is
-- who is in it (members): the access check for everything below. Players
-- are accounts an organiser made for the group (role 'player'); they sign
-- in with a link (sign_in_links) and see only their groups.

CREATE TABLE groups (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- who made it
  name          TEXT NOT NULL,
  sport         TEXT NOT NULL,                  -- a key from the sports list (sports.go)
  sport_name    TEXT NOT NULL DEFAULT '',       -- the name, for "other"
  scoring       TEXT NOT NULL DEFAULT '',       -- sets | goals | points | result | ranking
  format        TEXT NOT NULL DEFAULT '',       -- singles | doubles | teams | everyone
  lower_better  INTEGER NOT NULL DEFAULT 0,     -- ranking: the lowest score wins (golf)
  color         TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  venue_name    TEXT NOT NULL DEFAULT '',
  venue_address TEXT NOT NULL DEFAULT '',
  venue_lat     REAL,
  venue_lon     REAL,
  venue_ref     TEXT NOT NULL DEFAULT '',       -- the place in Pocket Places
  outdoor       INTEGER NOT NULL DEFAULT 0,
  weekday       INTEGER NOT NULL DEFAULT 0,     -- the usual day, 1 = Monday … 7 = Sunday, 0 = none
  time          TEXT NOT NULL DEFAULT '',       -- the usual time, HH:MM
  duration_min  INTEGER NOT NULL DEFAULT 90,
  plan_days     INTEGER NOT NULL DEFAULT 0,     -- plan the usual game this many days before (0 = by hand)
  min_players   INTEGER NOT NULL DEFAULT 2,
  max_players   INTEGER NOT NULL DEFAULT 0,     -- 0 = no limit
  cost_cents    INTEGER NOT NULL DEFAULT 0,     -- what a game costs altogether (the court, the pitch)
  currency      TEXT NOT NULL DEFAULT 'EUR',
  win_points    INTEGER NOT NULL DEFAULT 3,
  draw_points   INTEGER NOT NULL DEFAULT 1,
  loss_points   INTEGER NOT NULL DEFAULT 0,
  email_players INTEGER NOT NULL DEFAULT 1,     -- email players about plans and reminders
  archived      INTEGER NOT NULL DEFAULT 0,
  import_key    TEXT NOT NULL DEFAULT '',       -- where an imported group came from (importing twice = once)
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX groups_user ON groups(user_id);
CREATE UNIQUE INDEX groups_import ON groups(user_id, import_key) WHERE import_key <> '';

CREATE TABLE members (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id     INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role         TEXT NOT NULL DEFAULT 'player',  -- organiser | player
  nickname     TEXT NOT NULL DEFAULT '',
  notify       TEXT NOT NULL DEFAULT 'all',     -- all | games | off
  last_read_id INTEGER NOT NULL DEFAULT 0,
  feed_token   TEXT NOT NULL UNIQUE,            -- the person's calendar feed of this group's games
  added_by     INTEGER REFERENCES users(id) ON DELETE SET NULL,
  joined_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (group_id, user_id)
);
CREATE INDEX members_user ON members(user_id, group_id);

-- A game is one meeting: when, where, who is coming, the talk about it.
-- What happened on the court is in matches (one or more per game).
CREATE TABLE games (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id      INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id       INTEGER REFERENCES users(id) ON DELETE SET NULL, -- who planned it
  status        TEXT NOT NULL DEFAULT 'planned',  -- voting | planned | cancelled
  starts_at     TEXT NOT NULL DEFAULT '',         -- local time YYYY-MM-DDTHH:MM ('' while voting)
  duration_min  INTEGER NOT NULL DEFAULT 90,
  title         TEXT NOT NULL DEFAULT '',
  notes         TEXT NOT NULL DEFAULT '',
  venue_name    TEXT NOT NULL DEFAULT '',
  venue_address TEXT NOT NULL DEFAULT '',
  venue_lat     REAL,
  venue_lon     REAL,
  min_players   INTEGER NOT NULL DEFAULT 2,
  max_players   INTEGER NOT NULL DEFAULT 0,
  cost_cents    INTEGER NOT NULL DEFAULT 0,
  currency      TEXT NOT NULL DEFAULT 'EUR',
  vote_until    TEXT NOT NULL DEFAULT '',         -- voting: answer by (local YYYY-MM-DDTHH:MM)
  cancel_reason TEXT NOT NULL DEFAULT '',
  auto          INTEGER NOT NULL DEFAULT 0,       -- planned from the group's usual day
  sent          TEXT NOT NULL DEFAULT '',         -- the reminders already sent: "short day hour"
  import_key    TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX games_group ON games(group_id, starts_at);
CREATE UNIQUE INDEX games_import ON games(group_id, import_key) WHERE import_key <> '';

-- Who is coming. waiting = said yes when the game was full; they move up
-- in the order they answered when someone drops out.
CREATE TABLE answers (
  game_id     INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  answer      TEXT NOT NULL,                     -- in | maybe | out | waiting
  side        TEXT NOT NULL DEFAULT '',          -- the team picked for the day: a | b | ''
  note        TEXT NOT NULL DEFAULT '',          -- "10 min late"
  answered_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (game_id, user_id)
);
CREATE INDEX answers_user ON answers(user_id);

-- Finding a time: the times offered and everyone's yes / maybe / no.
CREATE TABLE options (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  game_id   INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  starts_at TEXT NOT NULL,
  UNIQUE (game_id, starts_at)
);
CREATE TABLE votes (
  option_id INTEGER NOT NULL REFERENCES options(id) ON DELETE CASCADE,
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  answer    TEXT NOT NULL,                       -- yes | maybe | no
  PRIMARY KEY (option_id, user_id)
);

-- A result. side a against side b (singles, doubles, teams), or everyone
-- on their own (ranking: each player's score and place).
CREATE TABLE matches (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id     INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  game_id      INTEGER REFERENCES games(id) ON DELETE SET NULL,
  user_id      INTEGER REFERENCES users(id) ON DELETE SET NULL, -- who entered it
  played_at    TEXT NOT NULL,                    -- local YYYY-MM-DDTHH:MM
  score        TEXT NOT NULL DEFAULT '{}',       -- {"sets":[[6,4],[3,6]]} | {"a":3,"b":2} | {}
  winner       TEXT NOT NULL DEFAULT '',         -- a | b | draw | '' (ranking)
  note         TEXT NOT NULL DEFAULT '',
  confirmed_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  confirmed_at TEXT,
  disputed_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
  dispute      TEXT NOT NULL DEFAULT '',
  import_key   TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX matches_group ON matches(group_id, played_at);
CREATE INDEX matches_game ON matches(game_id);
CREATE UNIQUE INDEX matches_import ON matches(group_id, import_key) WHERE import_key <> '';

CREATE TABLE match_players (
  match_id INTEGER NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
  user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  side     TEXT NOT NULL DEFAULT '',             -- a | b | '' (ranking)
  score    REAL,                                 -- their goals / points, or their score in a ranking
  place    INTEGER NOT NULL DEFAULT 0,           -- ranking: 1 = won
  PRIMARY KEY (match_id, user_id)
);
CREATE INDEX match_players_user ON match_players(user_id);

-- Player of the game: one vote per person who was there.
CREATE TABLE mvp_votes (
  game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  for_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (game_id, user_id)
);

-- The group's chat, and the comments under a game (game_id set).
CREATE TABLE messages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id   INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  game_id    INTEGER REFERENCES games(id) ON DELETE CASCADE,
  user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL, -- NULL = the app (a game planned, a result)
  kind       TEXT NOT NULL DEFAULT 'text',       -- text | system
  body       TEXT NOT NULL DEFAULT '',
  data       TEXT NOT NULL DEFAULT '{}',         -- system: {"what":"planned","game_id":…}
  reply_to   INTEGER,
  client_id  TEXT NOT NULL DEFAULT '',
  edited_at  TEXT,
  deleted_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX messages_group ON messages(group_id, id);
CREATE INDEX messages_game ON messages(game_id, id);

CREATE TABLE reactions (
  message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  emoji      TEXT NOT NULL,
  PRIMARY KEY (message_id, user_id, emoji)
);

-- Sign-in links for players: only the hash is kept; one use each.
CREATE TABLE sign_in_links (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  group_id   INTEGER REFERENCES groups(id) ON DELETE SET NULL,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  used_at    TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX sign_in_links_user ON sign_in_links(user_id);

-- Things made in the other Pocket apps for one person: the game in their
-- calendar, the match in their health history, their share in Money …
CREATE TABLE made (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  group_id   INTEGER REFERENCES groups(id) ON DELETE CASCADE,
  game_id    INTEGER REFERENCES games(id) ON DELETE CASCADE,
  match_id   INTEGER REFERENCES matches(id) ON DELETE CASCADE,
  app        TEXT NOT NULL,                      -- calendar | health | habits | split | money | tasks | places
  ref        TEXT NOT NULL DEFAULT '',
  url        TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  detail     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX made_user ON made(user_id, game_id);
CREATE UNIQUE INDEX made_once ON made(user_id, app, COALESCE(game_id, 0), COALESCE(match_id, 0));
