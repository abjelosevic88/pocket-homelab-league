-- The places a group plays at: picked from when planning a game, and
-- offered next to the times in a vote ("Tuesday at Koševo or Wednesday
-- at Mojmilo?"). The group's own venue stays its default.
CREATE TABLE venues (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id   INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  address    TEXT NOT NULL DEFAULT '',
  lat        REAL,
  lon        REAL,
  ref        TEXT NOT NULL DEFAULT '',            -- the place in Pocket Places
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX venues_name ON venues(group_id, name COLLATE NOCASE);

INSERT INTO venues (group_id, name, address, lat, lon, ref)
  SELECT id, venue_name, venue_address, venue_lat, venue_lon, venue_ref FROM groups WHERE venue_name <> '';

-- A time to vote on can come with its own place; the same time at two
-- places is two options. The table is rebuilt for the new uniqueness,
-- with the votes kept.
CREATE TABLE options_new (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  game_id       INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  starts_at     TEXT NOT NULL,
  venue_name    TEXT NOT NULL DEFAULT '',
  venue_address TEXT NOT NULL DEFAULT '',
  venue_lat     REAL,
  venue_lon     REAL,
  UNIQUE (game_id, starts_at, venue_name)
);
CREATE TABLE votes_new (
  option_id INTEGER NOT NULL REFERENCES options_new(id) ON DELETE CASCADE,
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  answer    TEXT NOT NULL,
  PRIMARY KEY (option_id, user_id)
);
INSERT INTO options_new (id, game_id, starts_at) SELECT id, game_id, starts_at FROM options;
INSERT INTO votes_new (option_id, user_id, answer) SELECT option_id, user_id, answer FROM votes;
DROP TABLE votes;
DROP TABLE options;
ALTER TABLE options_new RENAME TO options;
ALTER TABLE votes_new RENAME TO votes;
