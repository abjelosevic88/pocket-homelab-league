# API

JSON over HTTP at `/api/v1`. Authenticate with the session cookie, or `Authorization: Bearer <token>` (Settings → API & shortcuts). Everything is for the person the token belongs to: groups are visible only to the people in them; anything else answers `404`.

## Groups and people

| | |
|---|---|
| `GET /sports` | The sports and how each is scored |
| `GET /groups` · `POST /groups` | Your groups (with the next game) · make one `{"name","sport", …}` |
| `GET /groups/{id}` · `PUT` · `DELETE` | A group with its people and coming games · change it (organisers) · delete it (its maker) |
| `PUT /groups/{id}/me` | What you hear about it: `{"notify":"all|games|off"}` |
| `POST /groups/{id}/leave` | Leave it |
| `POST /groups/{id}/members` | Add someone `{"name","email","phone","role","send"}` — a player account is made when nobody here has that email or phone |
| `POST /groups/{id}/members/{uid}/link` | A new one-time sign-in link for a player `{"email":true}` → `{url, text, whatsapp, sms, mailto, emailed}` |
| `POST /groups/{id}/invite-all` | Email a link to every player who hasn't come in |
| `POST /groups/{id}/import-players` | A CSV (name, email, phone) |
| `GET /groups/{id}/venues` · `POST` · `PUT /groups/{id}/venues/{vid}` · `DELETE` | The places a group plays at (organisers change them; `{"default":true}` makes one the group's own); a place used for a game is added by itself |
| `GET /groups/{id}/table?period=all|year|season|month` | The table with ratings and form |
| `GET /groups/{id}/players/{uid}` | A player's line, rating over time, head-to-head and last matches |

## Games

| | |
|---|---|
| `GET /upcoming` · `GET /games?group=&upcoming=1|past=1&from=&to=` | Games |
| `POST /groups/{id}/games` | Plan one `{"starts_at":"2026-11-10T20:00", "venue_name":…, "im_in":true}` or offer times and places `{"options":["2026-11-10T20:00", {"starts_at":…, "venue_name":"Mojmilo"}], "vote_until":…}` — a place given by name takes its saved address and position |
| `GET /games/{id}` · `PUT` · `DELETE` | A game with who's in, the times, results, player of the game |
| `POST /games/{id}/answer` | `{"answer":"in|maybe|out|"}` (organisers: `"user_id"`) |
| `POST /games/{id}/votes` · `/pick` | Vote `{"votes":{"<option id>":"yes|maybe|no"}}` · pick a time (and its place) `{"option_id"}` |
| `POST /games/{id}/teams` | `{"mode":"fair|shuffle|clear|set", "sides":{…}}` |
| `POST /games/{id}/mvp` | `{"for": <user id>}` |
| `POST /games/{id}/cancel` | `{"reason"}` or `{"undo":true}` |
| `GET /games/{id}/game.ics` · `GET /feed/{token}.ics` | One game · a group's games to subscribe to |
| `GET /games/{id}/weather` · `/busy` | The forecast at the venue · your calendar at those times |
| `POST /games/{id}/make` | In your other apps: `{"what":"calendar|split|money|health|tasks"}` |

## Results and chat

| | |
|---|---|
| `GET /matches?group=&game=&player=` | Results |
| `POST /groups/{id}/matches` | `{"game_id", "players":[{"user_id","side":"a|b","score"}], "score":{"sets":[[6,4],[6,3]]} | {"a":3,"b":2}, "winner"}` |
| `PUT /matches/{id}` · `DELETE` · `POST /matches/{id}/confirm` | Fix · delete · confirm (`{"wrong":true,"reason"}` disputes) |
| `GET /groups/{id}/messages?before=&after=&game=` · `POST` | The chat (or a game's comments) · say something `{"body","game_id","client_id"}` |
| `GET /stream` | Live updates (Server-Sent Events) |

## Overviews (also for Pocket Day, Search and Pocket AI)

`GET /today`, `GET /summary`, `GET /day/YYYY-MM-DD`, `GET /search?q=`, `GET /lookup?email=`.

## Sign-in links (no token)

`GET /auth/link/{token}` shows who invited whom to what; `POST /auth/link {"token"}` uses it (once) and signs in; `POST /auth/link/request {"email"}` sends a player a new one.
