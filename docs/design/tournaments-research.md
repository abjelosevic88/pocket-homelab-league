# Pocket League: tournament module research

Sources checked: Challonge, Tournify, Toornament, start.gg, Battlefy, Playtomic (KOTC/Americano/Mexicano tools), Americano apps (americano-padel.com, Padel Americano, FenixPlay, padel.fyi), TeamSnap Tournaments, pickleball.com, LTA Local Leagues and ClubSpark box leagues and ladders, Global Tennis Network, FIDE C.04.3 (Dutch) and the FIDE tie-break regulations, the lichess arena rules, USTA/ITF round-robin tie-break rules, FIVB pool ranking, golf Stableford and countback policies, darts and bowling league rules, squash interclub rules, and brackets-manager.js / brackets-viewer.js. Spond and Heja don't really have tournaments. They do events with availability (RSVP), which Pocket League already has. Sources are listed at the end.

---

## 1. Formats

### 1.1 Single elimination (knockout)
- **How it works:** you lose once and you're out. The bracket size is the next power of 2 at or above N. That gives log2(size) rounds and N−1 matches.
- **Generation:** standard seed order puts 1 v 16, 8 v 9, 5 v 12, 4 v 13 and so on, so seeds 1 and 2 can only meet in the final. This is brackets-manager's `inner_outer`. Byes (size − N) go to the top seeds in round 1, spread so that no match is bye v bye (`balanceByes`).
- **Winner:** whoever wins the final.
- **Add-ons:**
  - **Third-place match** between the two semi-final losers.
  - **Consolation / plate:** first-round losers (sometimes round-1 and round-2 losers) play a second knockout.
  - **Feed-in consolation:** losers from later rounds join the plate.
  - **Compass draw** (tennis): every player gets about 4 matches.
  - **Full placement:** play-offs for 5th to 8th and so on.
- **Ties inside a match** are settled by sport rules: extra time or penalties (football, futsal, handball, hockey), overtime (basketball), a match tie-break (tennis, padel), a decider frame or leg. Knockouts don't need table tie-breakers.
- **Sports:** all of them. It's the default "cup" for tennis, padel, squash, table tennis, darts, pool, football and futsal, and golf match play.
- **Sizes:** 4 to 64. For friends, mostly 8 or 16.

### 1.2 Double elimination
- **How it works:** a winners bracket (WB) and a losers bracket (LB). You're out after two losses.
- **Generation:** WB round-1 losers drop into LB round 1. Losers from later WB rounds drop into alternate LB rounds, in a crossed order (reverse or half-shift) so that rematches come late.
- **Grand final:** WB winner v LB winner. With a **bracket reset**, a second match is played if the LB side wins the first. Options are none, simple or double.
- **Matches:** about 2N−2, or 2N−1 with a reset.
- **Sports:** pool (very common), darts, table tennis, table games, esports, and any casual night where people want at least two matches.
- **Sizes:** 4 to 32 (64 at most).

### 1.3 Round robin (league / pool)
- **How it works:** everyone plays everyone, once (single) or twice (double, home and away with sides swapped).
- **Generation:** the circle (Berger) method. Fix one entry and rotate the rest.
  - Even N: N−1 rounds.
  - Odd N: add a dummy. Whoever meets the dummy sits out that round, so N rounds.
  - Matches: N(N−1)/2.
  - Berger tables also balance white/black in chess and home/away.
- **Winner:** the table, sorted by points and then the tie-break chain.
- **Points systems:**
  - Football, futsal, hockey and handball: 3 / 1 / 0.
  - Basketball: no draws. Rank by wins or win %, or use 2/1 (FIBA: 2 for a win, 1 for a loss).
  - Volleyball (FIVB): 3-0 or 3-1 gives 3 / 0; 3-2 gives 2 / 1.
  - Beach volleyball: 2 for a win, 1 for a loss, 0 for a forfeit.
  - Table tennis (ITTF): 2 for a win, 1 for a played loss, 0 for an unplayed loss.
  - Racket sports: matches won.
  - Chess: 1 / ½ / 0.
  - Darts and squash leagues: often a point per leg or per game won, plus a bonus for winning the match.
- **Tie-breakers by sport:**
  - **Football and futsal:** FIFA-style is goal difference, then goals scored, then head-to-head. UEFA and EHF style (also handball) is head-to-head points, then head-to-head goal difference, then head-to-head goals, then overall goal difference and goals scored. After that come fair play and lots.
  - **Basketball:** head-to-head record, then head-to-head point difference, then overall point difference, then points scored.
  - **Volleyball:** match points, wins, set ratio (sets won ÷ sets lost), point ratio, head-to-head.
  - **Tennis and padel (USTA/ITF):**
    1. Matches won.
    2. Two tied: head-to-head.
    3. Three or more tied: % sets won, then % games won, then head-to-head once only two are still tied. Some sections use fewest sets lost or fewest games lost instead.
    4. Retired or walkover matches are often left out of game %.
  - **Badminton (BWF):** matches won. Two tied: head-to-head. Three or more tied: game difference, then point difference.
  - **Table tennis (ITTF):** match points. Then, using only the matches among the tied players: match points, game ratio, point ratio, lots.
  - **Squash:** matches won, then games won (or game difference), then points difference, then head-to-head.
  - **Darts:** match points, then leg difference, then legs won, then head-to-head, then 3-dart average.
  - **Pool and snooker:** frame difference, then frames won, then head-to-head.
  - **Chess round robin:** Sonneborn-Berger, then direct encounter, then number of wins, then Koya.
- **Sports:** all of them. It's the best format for friends with 3 to 8 entries.
- **Sizes:** 3 to 10 per pool. Double round robin works well for 3 to 6.

### 1.4 Groups then knockout (pool play to playoffs)
- **How it works:** a round robin in groups of 3 to 6. The top k from each group (sometimes the best third-placed teams too) go into a knockout.
- **Generation:**
  - **Groups:** snake seeding (1 to A, 2 to B, 3 to C, 4 to C, 5 to B, 6 to A...) or by pots.
  - **Knockout:** cross-over pairings (A1 v B2, B1 v A2). Same-group entries are placed so they meet as late as possible (brackets-manager `groups.bracket_optimized`).
- **Optional extras:** placement brackets for non-qualifiers ("gold" and "silver" cups), so nobody goes home after the groups.
- **Sports:** football and futsal tournaments (Tournify's core use), volleyball, handball, hockey, basketball, padel and tennis events, table tennis.
- **Sizes:** 8 to 48 entries.

### 1.5 Swiss system
- **How it works:** a fixed number of rounds. Each round pairs entries with the same or similar score. Nobody is knocked out and nobody plays the same opponent twice.
- **Rounds:** about ⌈log2 N⌉, plus 1 or 2 for a clear winner. For example, 16 players need 4 to 5 rounds and 30 players need 5 to 7.
- **Generation (FIDE Dutch, simplified):**
  1. Make score groups of players on the same points, each sorted by rating or initial rank.
  2. Split each group into a top half S1 and a bottom half S2. Pair S1[i] with S2[i], so round 1 is 1 v N/2+1.
  3. If the pairing would give a rematch or a bad colour, swap within S2 (transpositions), then exchange players between S1 and S2.
  4. If a group has an odd count or can't be paired, move a player into the next group ("floaters"). Avoid floating the same player twice in a row.
  5. Colours: alternate, aim for a balance of 0, and never give anyone the same colour 3 times in a row.
  6. Odd N: the lowest-ranked player who hasn't had a bye gets it. A bye is worth 1 point (some events give ½), at most once per player.
- **Simpler version** (fine for non-chess): sort by points, then tie-breaks, then rating. Pair greedily from the top, backtracking to avoid rematches. Tennis and padel Swiss and esports Swiss (Toornament, Challengermode) work this way.
- **Winner:** most points.
- **Tie-breaks:**
  - **Buchholz:** sum of opponents' scores. Variants drop the worst opponent (Cut-1) or the best and worst (Median-Buchholz, Challonge's option).
  - **Sonneborn-Berger:** sum of the scores of opponents you beat, plus half the scores of those you drew.
  - **Others:** direct encounter, number of wins, wins with black, progressive (cumulative) score, average rating of opponents, performance rating.
  - **Unplayed games:** FIDE's 2023 tie-break rules count them against a "virtual opponent" so byes don't inflate Buchholz.
- **Sports:** chess (the standard), bridge, esports, and increasingly padel, tennis and table-tennis social events with 12 or more entries and not enough time for a round robin.
- **Sizes:** 8 to 100 or more.

### 1.6 Arena (lichess-style, timed)
- **How it works:** a fixed time window, for example 60 minutes. As soon as you finish a game you're paired again with someone close to you in the current ranking, avoiding the opponent you just played.
- **Scoring:** win 2, draw 1, loss 0.
  - Two wins in a row starts a streak (shown as a flame). While it lasts, wins score 4 and draws score 2.
  - "Berserk" (half your clock) adds +1 for a win.
- **Winner:** the top score when time runs out. Ties go to performance rating.
- **Sports:** chess, or any quick 1v1 game (darts 301, table tennis to 11) on a club night where people come and go.
- **Sizes:** 6 to 1,000s. It works with any number and late arrivals.

### 1.7 Ladder / challenge ladder, and box leagues
- **Challenge ladder:**
  - A ranked list. You may challenge someone up to K places above you (usually 3; some clubs run "5-up weeks").
  - The defender must accept within X days (3 to 7) and play within Y days (7 to 21), or lose by default.
  - If the challenger wins, they take the defender's place (or go just above them) and the players in between move down one. If the challenger loses, nothing changes.
  - Usual extras: one open challenge at a time, a protection period after a match, an inactivity drop, and a holiday or freeze status (GTN "leave").
- **Points ladder (GTN style):** points for playing, winning, and sets and games won. Some give a bonus for beating someone ranked higher (3 instead of 1). There are penalties for accepted challenges not played (−1 after 20 days) and for forfeits (−5 / +5).
- **Box league** (LTA ClubSpark, squash clubs):
  - Groups of 4 to 6 by level play a round robin over a period (2 to 6 weeks). Players arrange their own matches and court time.
  - After the period, the top 1 or 2 go up a box and the bottom 1 or 2 go down.
  - Points vary. For example: 2 for a win; or sets won; or 1 per game plus 4 for turning up.
- **LTA Local Leagues:** groups of 6 to 8 players of a similar standard. Players book their own courts and send in results online. Rounds last about 2 months.
- **Sports:** tennis, squash, badminton, table tennis, pickleball, padel, pool, darts, chess. This is the main long-running "season" format, and it overlaps with Pocket League's Elo table.
- **Sizes:** 8 to 60.

### 1.8 Americano (padel, also pickleball, tennis doubles, badminton doubles, beach volley)
- **How it works:** each player enters alone. Partners change every round, and the schedule tries to pair everyone with everyone once and face everyone about equally. Every match is a fixed number of rally points (16, 21, 24 or 32; 32 is popular because each player serves 4 times). Each player banks their team's points (for example, 18–14 gives 18 to each winner and 14 to each loser).
- **Generation:** a pre-computed rotation ("whist" schedule).
  - 8 players: 7 rounds on 2 courts, with every partnership exactly once.
  - 12 players: 11 rounds on 3 courts.
  - 16 players: 15 rounds on 4 courts.
  - Perfect schedules exist for N ≡ 0 or 1 mod 4. Otherwise generate a near-optimal one (greedy or optimiser minimising repeated partners and opponents).
  - When N isn't a multiple of 4, players sit out in turn. Spread sit-outs evenly. A sitting-out player gets either nothing (rank by average) or the round's average.
  - Uneven counts and repeated partners are the most common complaint about Americano apps (for example, "6 players randomises badly").
- **Winner:** most total points. Then most matches won, point difference, and head-to-head or lots.
- **Variants:**
  - **Team Americano:** fixed pairs, each match a timed or points-capped game. It's really a round robin of pairs with points scoring.
  - **Mixed Americano:** always one man and one woman per side.
  - **Club / Team Americano:** teams of players, with points summed per club.
- **Sizes:** 8, 12 or 16 (up to 32). About 2 hours for 8 players at 32 points.

### 1.9 Mexicano
- **How it works:** Americano scoring with ranking-driven pairings.
  - Round 1 is random or by level.
  - From round 2, sort the live ranking. Court 1: 1st + 3rd v 2nd + 4th. Court 2: 5th + 7th v 6th + 8th, and so on. Some apps use 1+4 v 2+3, or 1+2 v 3+4.
  - Optional rule: avoid repeating last round's partner.
  - Courts are ordered, so the top court is where the leaders play.
- **Winner:** total points after a fixed number of rounds (usually 5 to 8). Ties as in Americano.
- **Variants:** Super Mexicano (court bonuses), Mixicano (mixed), Team Mexicano.
- **Key constraint:** rounds must be generated one at a time.

### 1.10 King / Queen of the Court (and pozo, "up and down the river")
- **How it works:**
  - Courts are ranked 1 (top) to C. Each round is a timed or short match (for example, 10 to 15 minutes, first to 4 no-ad games, or to 21 points).
  - Winners move up a court and losers move down. Winners on court 1 stay ("kings"). Losers on the bottom court stay and face new opponents.
  - Doubles variant: split partners after each round. One winner pairs with one loser on the new court, so it's individual. Some clubs keep fixed teams.
- **Winner:**
  - Whoever is on court 1 at the end.
  - Or individual counts (rounds won on court 1, total wins, games won).
  - Or a points-per-court multiplier.
- **Beach volleyball "King of the Beach":** different rules. 3 or more teams share a court. Winner stays, challengers rotate in, and points only count on the "king" side.
- **Sports:** padel, pickleball, tennis doubles, beach and indoor volleyball, badminton, table tennis.
- **Sizes:** 8 to 32 (2 to 8 courts).
- **Key constraint:** round by round, and needs a court order.

### 1.11 Stroke play / Stableford rounds (golf, and "everyone-on-their-own" generally)
- **Stroke play:** total strokes over 1 or more rounds.
  - Gross is raw strokes. Net = gross − playing handicap.
  - WHS: course handicap = handicap index × slope / 113 + (course rating − par). Playing handicap is about 95% of that for individual stroke play.
  - Strokes are given per hole by stroke index (SI 1 gets the first stroke, and so on).
- **Stableford (net), points per hole:**

  | Net result on the hole | Points |
  |---|---|
  | Double bogey or worse | 0 (pick up) |
  | Bogey | 1 |
  | Par | 2 |
  | Birdie | 3 |
  | Eagle | 4 |
  | Albatross | 5 |

  Modified Stableford scales also exist (for example, PGA Barracuda: +8 / +5 / +2 / 0 / −1 / −3).
- **Winner:** lowest net (stroke play) or most points (Stableford).
- **Countback:** compare the last 9, then the last 6, then the last 3, then the last hole. For net stroke play, use ½, ⅓, ⅙ and 1/18 of the handicap. Some societies then use lowest handicap, or a play-off.
- **Golf match play:** decided hole by hole. The result is written "3&2" (3 up with 2 to play) or "1 up", and a match can be halved. Team formats: four-ball (better ball), foursomes (alternate shot), singles (Ryder Cup style).
- **Society "order of merit":** season points by finishing position each round.
- **The same "ranking rounds" engine fits:**
  - **Bowling:** total pins over N games (a series). Handicap = (basis − average) × % (commonly 90% of 220, rounded down), added per game. Team leagues award points per game plus points for the series (for example, 2 / 2 / 2 + 1). Ties go to scratch pins. Last-week "position rounds" pair 1 v 2, 3 v 4.
  - **Darts** high-score or "around the clock" ranking nights.
  - Golf-like **disc golf** and **mini-golf**.
  - Pocket League's existing **ranking** scoring.

### 1.12 Team tournaments (club v club ties with rubbers)
- **How it works:** a tie between two teams is a set of individual matches (rubbers or strings). The team that wins the most rubbers wins the tie.
- **Common shapes:**
  - **Tennis Davis Cup:** 4 singles + 1 doubles, first to 3. The modern Finals use 2 singles + 1 doubles.
  - **Padel and tennis club leagues:** 3 pairs ranked by strength.
  - **Squash:** 5 strings in order of merit. Captains declare the order, and a wrong order loses by default.
  - **Table tennis:** 3 v 3, 9 singles (ABC v XYZ), or 4 singles + 1 doubles (Corbillon).
  - **Darts league nights:** for example, 6 singles + 3 pairs, best of 3 legs each, 501 straight in and double out.
  - **Badminton:** 3 men's doubles + 3 women's doubles + mixed, or 5 rubbers.
  - **Golf:** Ryder Cup sessions.
  - **Bowling:** team points per game.
- **Scoring:** a point per rubber, sometimes plus a point per game won and a bonus for winning the tie. Remaining "dead rubbers" may be shortened.
- **Tie-break chain:** tie points, then rubbers won, then sets or games, then points.
- **Rules to model:** a team roster, a lineup per tie (who plays which rubber, an order of strength), substitutes, a player can't appear twice in the same rubber type, and a defaulted rubber when someone is short.
- **For friends:** "teams picked on the day". Split the group into balanced teams (an Elo snake), then play a round-robin or knockout of ties, or one big tie.

---

## 2. Seeding and byes

| Method | Use |
|---|---|
| **Random draw** | Casual events, and the round 1 draw in Americano/Mexicano. Show an animated or "shuffle" draw so it feels fair, and allow a redraw before start. |
| **By rating** | Use Pocket League's Elo (snapshot it at draw time). It's the natural default for knockouts, Swiss and Mexicano round 1. |
| **Manual** | Drag to reorder seeds, or pin seeds 1 to 4 and draw the rest at random. Challonge's drag-and-drop seed list is the model. |
| **Snake into groups** | Seeds 1..G go to groups A..G, then G..1 back, and so on (`groups.seed_optimized`). Other options are pots (one from each pot per group) and "avoid same club or same pair" separation. |
| **Bracket-aware groups** | Place group stage entries so that group-mates are in opposite halves of the knockout. |
| **Balanced teams on the day** | Sort players by Elo and snake them into T teams. Optionally balance by gender, keepers or position. Allow manual swaps afterwards. |

**Byes:**
- **Knockout:** size − N byes go to seeds 1, 2, 3 and so on. Put them in the standard seed slots so every bye gets a real opponent and no match is bye v bye. A bye is an automatic win that doesn't count for Elo or stats.
- **Round robin:** with odd N, one entry sits out each round. The dummy entry rotates, so everyone sits once.
- **Swiss:** the lowest-ranked player without a previous bye gets it, worth a full or half point. Count it as "unplayed" in tie-breaks.
- **Americano / Mexicano / KOTC:** spread sit-outs (N mod 4 players per round) as evenly as possible. Never sit the same person twice before everyone has sat once. Choose a compensation rule: none, average points, or fixed points.
- **Double elimination:** losers bracket byes come from WB byes. Make sure the bracket doesn't end up with empty LB matches; brackets-manager propagates BYE automatically.

---

## 3. Scheduling at the venue

- **Resources:** a list of courts, pitches, tables, boards or lanes, with names and an order. The order matters for Mexicano and KOTC.
- **Timing inputs** (Tournify's model): start time, match duration, buffer between matches, minimum rest per entry (do not schedule back-to-back), breaks such as lunch, and an end-time check ("this won't fit in 3 hours").
- **Two modes:**
  1. **All at once:** generate the full time-and-court grid up front. Use it for round robin, groups and fixed brackets. The planner warns about rest conflicts and doubles booked on two courts.
  2. **Round by round, or queue:** a match is "ready" when both sides are known and rested, and it's called to the next free court. This mode is mandatory for Mexicano, KOTC, Swiss, Arena and knockouts, and better for any event where matches vary in length. Times then become estimates that move with real progress.
- **Phone views at the venue** (the most-loved feature):
  - **"Now playing":** one card per court with the players, score and elapsed time.
  - **"Up next":** the queue in order. Highlight "You're on Court 2 next" and the "Your matches" timeline.
  - **Push or chat notifications:** match called, opponent ready, result confirmed. Playtomic posts the final summary into the tournament chat.
  - **TV / slideshow mode:** a big-screen rotation of schedule, live scores, tables and bracket (Tournify slideshow, Challonge Station Tracker "airport board").
  - **Organizer console:** drag a match to another court, swap the order, delay a round, mark a court out of use.
- **Time-boxed matches** (Americano, KOTC, Arena): a shared round timer with a horn or alert. When it ends, enter scores and the next round is generated.
- **Multi-day or self-scheduled events** (ladders, box leagues): no court grid. Players propose times and use a play-by deadline, ideally linked to Pocket League "games" with in/maybe/out answers, so a tournament match can become a normal game invite.

---

## 4. Entries

- **Entry kinds:**
  - **Individual.**
  - **Pair:** fixed doubles. Register with a partner, or join a "looking for partner" pool that the organizer or an auto-match pairs up.
  - **Team:** a roster with a captain, subs and a lineup per tie.
  - **Individual into rotating pairs:** Americano, Mexicano, KOTC.
  - **Individual into teams picked on the day:** balanced draw.
- **Sign-up:**
  - Open, close and deadline times, a capacity, and level or gender restrictions (mixed events).
  - **Waiting list** with automatic promotion when someone withdraws. Notify them, and optionally require acceptance within X hours.
  - Battlefy and OpenSports offer manual "offer spot" from the waitlist.
  - **For friends:** sign up straight from a group game's "in" answers. The tournament is attached to a game, and the "in" list becomes entries ("maybe" means the waitlist).
- **Fees:** most rec apps take payments (Playtomic, TeamSnap, LeagueApps). Self-hosted only needs:
  - a fee amount and note;
  - a paid / unpaid flag per entry;
  - a "who owes" list;
  - optionally, a link to Pocket Money.
- **Check-in:**
  - The organizer ticks names, or players self-check-in (button or venue QR) in a window before start.
  - start.gg auto-DQs entrants who don't check in within 10 to 15 minutes.
  - **Rule:** no check-in, no draw. The draw happens after check-in closes, and waitlisted players present fill the gaps.
- **Withdrawals:**
  - **Before the draw:** remove the entry and promote the waitlist.
  - **After the draw, knockout:** the opponent gets a walkover ("w/o").
  - **After the draw, round robin:** common rule: if the withdrawing entry played under 50% of its matches, void all its results; otherwise the remaining matches count as walkover losses. Make this a setting.
  - **Americano / Mexicano:** replace the player with a substitute who inherits the slot (score kept with the slot or reset), or rebalance the remaining schedule.
- **Retirement:** the match ends mid-way. The score so far is recorded with "ret." and the opponent wins. For tie-breaks, either count the remaining sets as won by the winner or exclude the match from game %.
- **No-show:** after a grace time, the organizer (or the waiting player, via a "claim walkover" button) marks a walkover.
  - **Double no-show:** both lose, a "double walkover".
  - **Walkover score conventions:** football 3–0; FIVB 25–0 per set; ITTF 0 match points for the absentee; a tennis w/o counts no games.
- **Disqualification:** removes the entry. Choose whether its future matches become walkovers and whether its past results stay.
- **Substitutes** for team events: roster changes allowed until the tie lineup is locked.

---

## 5. Results, confirmation, live views, sharing, printing

- **Who enters:** the organizer always. The participants of the match optionally. Scorekeepers optionally.
- **Confirmation:**
  - One side submits and the other side gets a confirm prompt.
  - It auto-confirms after T minutes (pickleball.com defaults to 3 minutes and recommends instant for round robin and a short delay for knockouts).
  - **Dispute** goes to the organizer ("report to desk"). start.gg makes both sides confirm and offers "call a moderator".
  - **For friends:** default to "either player enters, opponent can reject within 10 minutes; organizer can always fix".
- **Score entry must fit the sport's scoring type:**
  - **Sets:** tennis and padel 6–4 7–6(5) with a 10-point match tie-break; volleyball 25 / 15 win by 2; badminton 21 (cap 30); table tennis 11 win by 2; squash 11 PAR.
  - **Goals and points:** football, basketball and so on, plus extra time and penalties.
  - **Legs and frames:** darts and pool.
  - **Winner only:** chess (with a draw option).
  - **Strokes per hole:** golf, with a gross/net/points auto-calc.
  - **Pins per frame or per game:** bowling.
  - **Points total:** Americano.
  - Validate impossible scores, but allow an override, because casual rules vary.
- **Corrections after later matches were generated:** the top pain point.
  - **Knockout:** if the winner changes, roll back the downstream matches that aren't played yet. Warn and offer re-generation if they are played.
  - **Mexicano / Swiss:** allow editing a past round and offer to regenerate unplayed rounds.
  - Keep an audit log with undo.
- **Live:**
  - Standings recalculate instantly.
  - Show **why** someone ranks where they do: the tie-break column that decided it, for example "ahead on head-to-head". People complain when ranking is opaque.
  - The bracket updates live, with in-progress markers (Challonge "underway").
- **Public share page:** a read-only link with a token (no login) and a QR code to scan at the venue. It shows schedule, now/next, tables, bracket and results. Players hate needing an account just to see the bracket.
- **Printing:**
  - Bracket and group tables as PDF or print CSS.
  - Blank score sheets per match or per court ("court cards").
  - Golf scorecards with handicap strokes marked.
  - Darts and bowling sheets.
  - A final results and podium sheet.
  - CSV export.
- **After the event:**
  - Summary posted to the group chat (Playtomic does this).
  - Podium and awards (champion, runner-up, most improved, best Elo gain).
  - Matches optionally feed the Elo table, with a "rated" toggle per tournament.
  - History of past tournaments per group, with titles shown on member profiles.

---

## 6. What the best apps do well, and common complaints

**Done well:**
- **Tournify:** very fast setup for football-style events; drag-and-drop schedule with automatic time slots; one shareable live site plus app with push notifications; big-screen slideshow; a "fields" concept with any naming (pitch, court, table, lane).
- **Challonge:** a bracket in a minute, drag-to-reseed, many formats (single elimination, double elimination, round robin, Swiss, two-stage), clear tie-break choices (wins, game wins, points, point difference, custom points, head-to-head, Median-Buchholz), embeddable brackets, stations and an "underway" status.
- **start.gg / Battlefy:** check-in with auto-DQ, player self-reporting with both sides confirming, moderator calls, waitlists, "bracket feedback" from players.
- **Toornament:** 11 stage types chained together (groups, league, Swiss, brackets, gauntlet, free-for-all), manual or automatic placement between stages, match format per round (best of 1 in groups, best of 3 in knockouts), configurable points per match and game.
- **Playtomic and padel apps:** format templates (Americano, Mexicano, KOTC) that generate rounds automatically; one-tap score entry per court; live leaderboard; summary posted to the event chat. The best-rated Americano apps win on "2 minutes from names to round 1".
- **pickleball.com:** the winner enters the score, the loser confirms or it auto-confirms, the next match is assigned and court notifications go out automatically. Directors call it their best feature.
- **TeamSnap Tournaments:** pools plus brackets plus consolation per division; scores updated from a phone.
- **brackets-viewer.js:** clean bracket rendering with "from match X" placeholders before opponents are known.

**Common complaints:**
1. Changes are hard once the event has started: edits don't carry through (TourneyEngine, LeagueLobster: "needs constant refreshes"), and correcting a result breaks the bracket.
2. Odd player counts, unfair sit-outs and repeated partners in Americano apps.
3. Multi-stage support is incomplete (Challonge's groups-to-knockout and API).
4. Too many settings up front for a casual night; no sensible per-sport presets.
5. No-show, walkover and retirement handling is missing or inconsistent.
6. Players need accounts or app installs just to see results.
7. Unclear tie-breakers ("why am I third?").
8. Schedules drift and the planned times become fiction; there's no "next up" queue.
9. Unreliable apps on venue Wi-Fi or Android (Playtomic stability reviews).
10. Many organizers still fall back to Excel for Americano nights.
11. Reschedule rules for multi-week formats are missing (ladders, box leagues).

---

## 7. Recommendation for Pocket League

### 7.1 Principles
- **Presets per sport and format** fill in scoring, match rules and the tie-break chain. Advanced settings stay folded away.
- **One engine:** a tournament is one or more **stages**. A stage generates **matches** inside **groups** and **rounds**. Results feed **standings**, and standings seed the next stage.
- **Two scheduling modes:** a full timetable, or a round-by-round queue with courts.
- **Reuse what the app has:**
  - group members as entrants;
  - a game's in/maybe/out answers as sign-up and waitlist;
  - the group's existing scoring types for score entry;
  - Elo for seeding and balanced teams, plus an optional "rated" toggle so matches feed the table;
  - chat for calls and the summary.
- Every destructive action (redraw, regenerate, withdraw, correct a result) shows a preview of its effect and can be undone.

### 7.2 Minimal-but-complete features (v1)
1. **Create from template:**
   - formats: Knockout, League (round robin), Groups + Knockout, Americano, Mexicano, Ranking rounds;
   - entry type: singles, pairs, teams-on-the-day, individual;
   - match rules: best of N sets, points to X, time limit, legs or frames, holes;
   - tie-break chain: preset per sport, can be reordered.
2. **Entries:**
   - from game "in" answers or invite;
   - capacity and waitlist with auto-promotion;
   - fee and paid flag;
   - check-in;
   - withdraw, substitute and DQ;
   - partner pairing (pick a partner, or random or balanced draw);
   - teams on the day (Elo snake plus manual swaps).
3. **Draw / seeding:**
   - random, by Elo, or manual drag;
   - snake into groups;
   - byes to top seeds;
   - fair sit-out rotation.
4. **Courts and schedule:**
   - courts list;
   - start time, duration, buffer and rest;
   - full timetable or "call next match to free court";
   - round timer for timed formats.
5. **Results:**
   - sport-aware score entry;
   - player entry plus opponent confirm, auto-confirm after N minutes, organizer override;
   - walkover, retired and no-show outcomes;
   - corrections that roll forward safely.
6. **Live views:**
   - My next match, Now / Next per court;
   - standings with a "decided by" note;
   - bracket;
   - TV mode;
   - public read-only share link and QR code.
7. **Close:**
   - final ranking and podium;
   - chat summary;
   - optional Elo update;
   - print bracket, tables and score sheets;
   - CSV export;
   - tournament history.

### 7.3 Order to ship the formats

| Phase | Formats | Why |
|---|---|---|
| **1** | Round robin (single/double, plus groups), Single elimination (+3rd place, + plate for first-round losers), Groups → Knockout, Americano (individual, team, mixed), Mexicano, Ranking rounds (stroke play/Stableford/bowling/darts "everyone on their own", sum or best-of over rounds) | Covers every sport and all four group formats. Americano and Mexicano are what friend groups in racket sports run most. |
| **2** | King of the Court, Double elimination (with optional grand final reset), Swiss (simple pairing plus Buchholz and Sonneborn-Berger; FIDE Dutch-lite with colours for chess), Team ties with rubbers (club v club, or teams-on-the-day) | Same engine, more pairing rules. |
| **3** | Challenge ladder and box leagues (season-long, self-scheduled, with deadlines and promotion/relegation), Arena (timed, continuous pairing), compass and feed-in consolation draws, golf match play and handicap allowances, bowling handicap | Long-running or niche formats. They overlap with the Elo table and need deadline and notification machinery. |

**Default format per sport (suggested):**

| Sport | Default formats |
|---|---|
| Padel, pickleball | Americano / Mexicano / KOTC (individual), groups → knockout (pairs) |
| Tennis, badminton, squash, table tennis | Round robin or groups → knockout. Box league or ladder for seasons. |
| Volleyball, beach volleyball | Pools → knockout (FIVB points). KOTC for beach. |
| Football, futsal, handball, hockey, basketball | Groups → knockout, with teams on the day |
| Darts, pool, snooker | Double elimination or round robin. Team league night (rubbers). |
| Chess | Swiss (4 to 7 rounds), round robin with ≤ 8 players, Arena |
| Golf | Stableford or stroke play rounds (net, with countback). Match play knockout. |
| Bowling | Ranking rounds (series, optional handicap) |
| Other | Round robin / knockout / ranking |

### 7.4 Data model (SQLite)

```
tournament
  id, group_id, sport, name, description, status(draft|signup|ready|live|finished|cancelled),
  entry_kind(player|pair|team|rotating_pairs|picked_teams), rated(bool), game_id(nullable, link to group game),
  starts_at, ends_at, venue, capacity, waitlist(bool), signup_opens_at, signup_closes_at,
  fee_amount, fee_note, checkin_required(bool), checkin_opens_at,
  confirm_mode(organizer|opponent|auto), auto_confirm_minutes, share_token, settings_json,
  created_by, created_at, updated_at

court            id, tournament_id, name, sort_order, active
organizer        tournament_id, member_id, role(owner|scorer)

entry            id, tournament_id, name, kind, status(registered|waitlist|confirmed|checked_in|withdrawn|disqualified),
                 waitlist_pos, seed, rating_snapshot, paid(bool), registered_at, checked_in_at, withdrawn_at, withdraw_reason
entry_member     entry_id, member_id, role(player|captain|sub), lineup_order

stage            id, tournament_id, seq, name, type(round_robin|single_elim|double_elim|swiss|americano|mexicano|kotc|ranking|ladder|team_tie|arena),
                 status(pending|live|done), seeding(random|rating|manual|snake|from_previous),
                 settings_json   -- e.g. {cycles:2, groups:4, advance:2, third_place:true, consolation:"first_round",
                                 --  grand_final:"reset", rounds:5, points_per_match:32, partner_rule:"1+3v2+4",
                                 --  sitout_comp:"average", pts:{win:3,draw:1,loss:0}, tiebreaks:["h2h","set_ratio","game_ratio"],
                                 --  match_rules:{best_of:3, set_to:6, final_set:"mtb10", time_limit_min:15}}
stage_group      id, stage_id, number, name            -- pool "A", or bracket WB/LB/Final/Plate
stage_entry      stage_id, entry_id, group_id, seed, final_rank, qualified(bool)   -- placement and stage result
round            id, stage_id, group_id, number, name, status, starts_at, timer_ends_at

match            id, stage_id, group_id, round_id, number, parent_match_id(nullable; rubbers in a team tie),
                 side_a_entry_id, side_b_entry_id,           -- null = TBD
                 side_a_source, side_b_source,               -- "W:12" / "L:12" / "A1" / "bye", for placeholders
                 next_match_id, next_slot, loser_next_match_id, loser_next_slot,
                 court_id, scheduled_at, called_at, started_at, finished_at,
                 status(tbd|ready|called|live|reported|confirmed|cancelled),
                 outcome(normal|walkover|retired|no_show|double_walkover|bye|draw),
                 winner_entry_id, score_json,                -- sets/legs/periods/penalties as per scoring type
                 a_total, b_total,                           -- aggregate goals/points/games for fast standings
                 reported_by, reported_at, confirmed_by, confirmed_at, disputed(bool), dispute_note,
                 league_game_id(nullable; the rated result written to the Elo table)
match_player     match_id, side(a|b), member_id              -- actual players (rotating pairs, team lineups, subs)

scorecard        id, round_id, entry_id, member_id, gross, handicap, net, points, position, detail_json
                 -- ranking formats: golf holes, bowling frames/games, darts ranking; one row per player per round

ladder_position  stage_id, entry_id, position, points, protected_until, frozen_until
challenge        id, stage_id, challenger_entry_id, defender_entry_id, status(open|accepted|declined|expired|played|defaulted),
                 created_at, respond_by, play_by, match_id

standing_cache   stage_id, group_id, entry_id, rank, played, won, drawn, lost, pts, for, against,
                 sets_for, sets_against, games_for, games_against, tb_json, decided_by  -- rebuilt on every result
event_log        id, tournament_id, at, actor_id, action, payload_json, undo_json   -- audit and undo
```

**Notes on the model:**
- **Team ties:** a parent `match` for the tie between two teams, with child `match` rows for the rubbers. The tie result is rolled up from the rubbers.
- **Americano / Mexicano / KOTC:** each `entry` is one player. The match's `side_a/b_entry_id` stay null, and `match_player` holds the 2 + 2 players. Standings sum player points from `match_player` and the totals. KOTC uses `court.sort_order` for up and down moves.
- **Double elimination:** `stage_group` WB / LB / Final, with `loser_next_match_id` wiring.
- **Swiss / Mexicano / Arena:** create the next `round` only when the previous one is confirmed, which keeps corrections safe. Editing a past round flags later unplayed rounds for regeneration.
- **Tie-break chain:** an ordered list of keys computed in one standings function. Keys:

  `points, wins, h2h_points, h2h_diff, diff, for, set_ratio, set_pct, game_ratio, game_pct, point_ratio, leg_diff, frame_diff, buchholz, buchholz_cut1, median_buchholz, sonneborn_berger, progressive, wins_black, countback, scratch_pins, rating, lots`

  "Head-to-head" is applied to the subset that is tied, so a mini-table for 3 or more tied, as UEFA, ITTF and BWF do.
- **Open-source reference:** brackets-manager.js (MIT) has a matching stage → group → round → match → match_game model, seeding and ordering methods (`inner_outer`, `half_shift`, `reverse`, `pair_flip`, `groups.seed_optimized` snake, `groups.bracket_optimized`), BYE propagation, consolation final and grand-final reset. brackets-viewer.js renders it. Both are good references for bracket wiring even if the engine is written in Go.

---

## Sources
- Mexicano: [americano-padel.com](https://americano-padel.com/mexicano-americano/en), [FenixPlay Mexicano](https://fenixplay-web.pages.dev/en/blog/mexicano-padel-tournament/), [padelmix Mexicano](https://padelmix-web.pages.dev/mexicano-padel), [visu.network generator](https://visu.network/tools/padel-americano-generator/)
- Americano: [rnkd guide](https://rnkd.pages.dev/guides/padel-americano-format-explained), [padel.fyi](https://www.padel.fyi/terms/americano/), [Skedge](https://skedge-web.web.app/resources/americano-format), [ligaspel docs](https://docs.ligaspel.se/administrator/quickstart-guide/americano)
- King of the Court: [FenixPlay KOTC](https://fenixplay-web.pages.dev/en/blog/king-of-the-court-padel/), [Up and Down rules](https://sites.google.com/view/COTCP), [Playtomic KOTC listing](https://playtomic.com/de/activities/tournaments/cf234813-8a33-4ee4-883b-05be1c147134)
- Playtomic: [Playtomic Manager: new tournament tools](https://helpmanager.playtomic.com/hc/en-gb/articles/44129657203985), [Adding participants](https://helpmanager.playtomic.com/hc/en-gb/articles/48343795791505-Adding-Participants-to-a-Tournament-from-Playtomic-Manager)
- Challonge: [Rank & tie-break statistics](https://challonge.uservoice.com/knowledgebase/articles/448540-rank-tie-break-statistics), [SSB wiki on Challonge](https://ssbwiki.com/Challonge), [Station Tracker](https://esportsinsider.com/?p=21183), [XenForo thread on multi-stage](https://www.XenForo.com/community/threads/xf2-8wr-xentorneo-2-rankings-pro-paid.143644/page-3)
- Tournify: [Getting started](https://help.tournifyapp.com/en/articles/15069302-getting-started-with-tournify), [Slideshow](https://tournifyapp.com/en/features/slideshow), [App](https://tournifyapp.com/en/features/android-ios-app)
- Toornament: [Stages (developer docs)](https://developer.toornament.com/v2/core-concepts/stages), [Structures](https://help.toornament.com/structures/which-format-for-my-tournament), [Swiss guide](https://blog.toornament.com/?p=9928)
- start.gg: [Online tournaments](https://help.start.gg/en/articles/13766288-how-do-online-tournaments-work-on-start-gg), [Reporting results](https://intercom.help/startgg-e6e3339622f9/en/articles/13766247-reporting-match-results), [Bracket feedback](https://help.start.gg/article/bracket-feedback); Battlefy [waitlist](https://help.battlefy.com/en/articles/14454010-how-does-the-tournament-waitlist-work), [match page](https://help.battlefy.com/en/articles/4662216-game-day-guide-match-page)
- Player score entry: [pickleball.com](https://pickleball.com/blogs/player-score-entry-and-why-its-beneficial-1)
- TeamSnap: [Event brackets](https://helpme.teamsnap.com/article/206-creating-event-brackets), [Tournament schedule](https://helpme.teamsnap.com/article/494-create-tournament-schedule)
- brackets-manager.js / brackets-viewer.js: [manager repo](https://github.com/Drarig29/brackets-manager.js), [viewer repo](https://github.com/fluidpb/brackets-viewer.js), [structure docs](https://drarig29.github.io/brackets-docs/user-guide/structure/), [seeding docs](https://drarig29.github.io/brackets-docs/user-guide/seeding/), [ordering docs](https://drarig29.github.io/brackets-docs/user-guide/ordering/)
- Chess: [FIDE C.04.3 Dutch](https://handbook.fide.com/chapter/C0403202602), [FIDE tie-break regulations](https://handbook.fide.com/chapter/TieBreakRegulations082024), [WinTD Dutch explanation](https://estima.com/chess/wintdhelp/topics/pairingsystemfidedutchsystem.html), [lichess arena help](https://lichess.org/tournament/help), [Challengermode Swiss](https://support.challengermode.com/en/organizing-tournaments1/swiss-format)
- Tennis ladders and leagues: [LTA ClubSpark ladders](https://clubspark.lta.org.uk/WinchmoreHillTennisClub/Tennis/SinglesLadder), [BIREC challenge guidelines](https://birec.org/wp-content/uploads/2023/08/Singles-Ladder-Challenge-Guidelines_8.4.23.pdf), [Global Tennis Network](https://www.globaltennisnetwork.com/), [Metro Tennis ladder (GTN)](https://metrotennisgroup.com/mtg-summer-ladder), [LTA Local Leagues](https://clubspark.lta.org.uk/RoehamptonPlayingFields/LocalLeague), [Box league rules](https://clubspark.lta.org.uk/CothamParkTennisClub/Boxleaguerules)
- Round-robin tie-breaks: [USTA Georgia](https://ustageorgia.com/wp-content/uploads/2023/12/Adult_Tournament_Officials_breaking_round_robin_ties.pdf), [USTA SoCal](https://www.ustasocal.com/usta-adult-40-over-3-0-4-0/), [Tennis Canada](https://www.tenniscanada.com/wp-content/uploads/2021/08/Round-Robin-Tie-Break-Rules-bil.pdf), [LTA round robin](https://www.lta.org.uk/4ac71c/siteassets/about-lta/file/round-robin-events.pdf)
- Volleyball: [FIVB pool ranking criteria](https://en.volleyballworld.com/en/volleyball/worldcup/2019/competition/poolrankingcriteria), [Rio 2016 beach](https://rio2016.fivb.com/en/beachvolleyball/competition/pools-ranking-criteria)
- Golf: [Stableford (Wikipedia)](https://en.wikipedia.org/wiki/Stableford), [BGA countback policy](https://www.blindgolf.com.au/documents/bga-core-08-countback-policy.pdf), [Stableford and WHS](https://golfhandicapcalculator.co/stableford-scoring-points-table-whs/)
- Darts: [WDF](https://site.dartswdf.com/?p=50), [Tamworth league cups](https://sites.google.com/view/tamworthdartsleague/cup-competitions/handicap-cup), [VCDA format](https://vcda.org/format.php)
- Bowling: [gobowling handicap guide](https://gobowling.com/blog/guides-tips/bowling-handicap-system-guide/), [FXA 90% of 230](https://help.fxasports.com/bowling-rules-handicap)
- Team ties: [Davis Cup](https://simple.wikipedia.com/wiki/Davis_Cup), [Squash Auckland interclub](https://squashauckland.org.nz/Interclub-1/Information-Rules), [Northern Squash team squash](https://wazygoose.com/northern-squash-club/team-squash)
- Complaints: [Americano app reviews](https://marlvel.ai/api/llm/apps/sports/padel-americano-tournaments), [Product Hunt Americano Padel](https://www.producthunt.com/products/americano-padel/reviews), [TourneyEngine reviews](https://www.capterra.com/p/180235/Tourney-Engine/reviews/), [LeagueLobster reviews](https://www.capterra.com/reviews/134112/LeagueLobster)
