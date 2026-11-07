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

## Tournaments

| | |
|---|---|
| `GET /tournaments/formats?group=` | The formats, the ones that suit the group's sport first, and their default settings |
| `GET /tournaments` · `GET /groups/{id}/tournaments` · `POST /groups/{id}/tournaments` | Tournaments · make one `{"name","format","entry_kind":"player|pair|team","settings":{…},"courts":["Court 1"],"add_members":true}` |
| `GET /tournaments/{id}` · `PUT` · `DELETE` | Everything: entries, stages with tables or a leaderboard, matches, courts, challenges |
| `POST /tournaments/{id}/join` · `/leave` | Sign up (`{"partner":id}` for pairs, `{"team_name":…}` for teams) · drop out (walkovers once playing) |
| `POST /tournaments/{id}/entries` · `PUT /tournaments/{id}/entries/{eid}` · `DELETE` | Organisers: add (people or a guest's name), seed, handicap, paid, check-in, withdraw, disqualify |
| `POST /tournaments/{id}/teams` | Make `{"teams":n}` teams of close ratings from the single sign-ups |
| `POST /tournaments/{id}/start` · `/redraw` · `/regenerate` · `/finish` · `/next-round` | Make the draw · back to sign-up · draw the rounds not played again · finish (or `{"cancel":true}`) · a box league's next round |
| `POST /tmatches/{id}/result` | `{"score":{…}}`, `{"a_total","b_total"}` (Americano), `{"winner"}`, `{"outcome":"walkover|retired|no_show|double_walkover"}` |
| `POST /tmatches/{id}/confirm` · `/reopen` · `/call` | Confirm (`{"wrong":true,"reason"}` disputes) · undo (organisers) · call to a court `{"court_id"}` |
| `PUT /tournaments/{id}/scorecards` | A score card `{"stage_id","round","entry_id","holes":[…]}` or `{"gross"}` |
| `POST /tournaments/{id}/challenges` · `/challenges/{cid}` | Ladder: challenge `{"defender"}` · answer `{"accept":true}` |
| `GET /public/t/{token}` | The read-only tournament for anyone with the link (no token needed) |

## Guests, subs, chores, away days

| | |
|---|---|
| `POST /games/{id}/guests` · `DELETE /games/{id}/guests/{guest}` | Bring a friend for one game `{"name"}` (up to 3 each; on the waiting list when full; members waiting go first) |
| `POST /games/{id}/no-show` | Organisers, after the start: `{"user_id","no_show":true}` |
| `GET /groups/{id}/reliability` | Came / late drop-outs / no-shows / no answer per person (organisers: everyone; others: their own) |
| `POST /games/{id}/sub` | Ask who could fill in `{"others":true,"dry_run":true}` — maybe, no answer, then (with `others`) people from your other groups of the sport |
| `GET /subs` · `POST /subs/{id}` | Places offered to you · take one `{}` (joins the group as a player when needed) or `{"no":true}` |
| `GET /groups/{id}/duties` · `PUT` | The chores `{"names":["Bring the balls", …]}` (organisers) |
| `POST /games/{id}/duties/{duty}` | Hand a chore over `{"user_id"}` or tick it `{"done":true}` (whoever has it, or an organiser) |
| `GET /away` · `POST` · `DELETE /away/{id}` | Your away days `{"from","to","note"}` — games then are "out" and you aren't asked |

## Money

| | |
|---|---|
| `GET /groups/{id}/money` | Balances for everyone, the entries (organisers: all; others: their own), how to pay |
| `POST /groups/{id}/money` | Organisers: `{"kind":"charge|payment","user_id","amount_cents","note"}`; anyone: `{"kind":"payment","amount_cents"}` for themselves (waits for an organiser) |
| `POST /money/{id}/confirm` · `DELETE /money/{id}` | Confirm a payment · remove an entry |

A group with `charge_games` books everyone's share of the court after each game (a guest's share to whoever brought them); a tournament's entry fee is booked when it starts.

## Seasons and stats

| | |
|---|---|
| `GET /groups/{id}/seasons` · `POST` | Seasons (and the one on now) · start one `{"name","starts_on","ends_on"}` |
| `PUT /seasons/{id}` · `DELETE` | Change · delete (the results stay) |
| `POST /seasons/{id}/close` | Keep the table and hand out the awards `{"next":{"name","starts_on"}}`; `{"reopen":true}` opens it again |
| `GET /groups/{id}/table?season={id}` | A season's table: worked out while it's on, the kept one (with `awards`) once closed |

Results take per-player stats the group counts (`group.stats`): `"players":[{"user_id","side","stats":{"assists":2,"yellow":1}}]`; the table and player pages add them up. With `rating_scope: "season"` ratings start at 1000 each season.

## Live scoring

| | |
|---|---|
| `GET /groups/{id}/live` · `POST` | Live scores · start one `{"game_id","a_ids","b_ids","rules"}` or `{"tmatch_id"}` |
| `GET /live/{id}` | One, with `view` (sets, games, 15-30-40, tie-break, set/match point) and `line` ("6–4 3–2 (30–15)") |
| `POST /live/{id}/point` | `{"side":"a","n":1,"at":<points seen>}` · `{"undo":true}` · `{"take":true}` to take over from someone else |
| `POST /live/{id}/finish` · `DELETE` | Over (the score then goes in as the result) · drop it |

Rules: `{"kind":"tennis","sets":3,"games":6,"tiebreak_at":6,"tiebreak_to":7,"super_final":false,"golden":false}`, `{"kind":"rally","sets":3,"points":21,"last_points":15,"win_by":2,"cap":30}` or `{"kind":"count"}`. Tournament pages (also the public one) carry `live` by match.

## Photos and phones

| | |
|---|---|
| `POST /groups/{id}/photos` | Upload a photo (multipart `file`, up to 15 MB) → `{id, url, thumb}`; send it with a message `{"body","photos":[id]}` |
| `GET /photos/{id}?size=small` | A photo of a group you're in (re-encoded: no camera location) |
| `GET /push/key` · `POST /push/subscribe` · `/unsubscribe` · `/test` | Notifications on this phone (Web Push, League's own key) |
