package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// a tennis group of Alice's with a planned game
func tennis(a *app) (gid, gameID string) {
	g := a.ok(a.alice, "POST", "/api/v1/groups", map[string]any{"name": "Tuesday Tennis", "sport": "tennis", "venue_name": "TK Koševo", "cost_cents": 4000, "currency": "BAM"})
	if g["scoring"] != "sets" || g["format"] != "singles" || num(g, "win_points") != 1 {
		a.t.Fatalf("sport defaults: %v", g)
	}
	gid = id(g)
	gm := a.ok(a.alice, "POST", "/api/v1/groups/"+gid+"/games", map[string]any{"starts_at": "2030-05-07T20:00", "im_in": true})
	return gid, id(gm["game"].(map[string]any))
}

func TestGroupsBelongToTheirPeople(t *testing.T) {
	a := newApp(t, nil)
	gid, gameID := tennis(a)
	g := "/api/v1/groups/" + gid
	// Bob is not in it: 404 everywhere, also as… nobody special (Alice is admin, Bob member)
	for _, c := range []struct{ m, p string }{
		{"GET", g}, {"PUT", g}, {"DELETE", g}, {"GET", g + "/members"}, {"GET", g + "/table"}, {"GET", g + "/messages"},
		{"POST", g + "/messages"}, {"POST", g + "/games"}, {"POST", g + "/matches"}, {"POST", g + "/members"},
		{"GET", "/api/v1/games/" + gameID}, {"POST", "/api/v1/games/" + gameID + "/answer"}, {"GET", "/api/v1/games/" + gameID + "/game.ics"},
	} {
		a.expect(404, a.bob, c.m, c.p, map[string]any{"body": "x", "answer": "in", "name": "x", "email": "x@example.com"})
	}
	if l := a.list(a.bob, "/api/v1/groups"); len(l) != 0 {
		t.Fatalf("Bob sees Alice's groups: %v", l)
	}
	if l := a.list(a.bob, "/api/v1/upcoming"); len(l) != 0 {
		t.Fatalf("Bob sees Alice's games: %v", l)
	}
	// Alice adds Bob: now he sees it, as a player (not an organiser)
	a.ok(a.alice, "POST", g+"/members", map[string]any{"name": "Bob", "email": "bob@example.com"})
	a.ok(a.bob, "GET", g, nil)
	a.expect(403, a.bob, "PUT", g, map[string]any{"name": "mine"})
	a.ok(a.bob, "POST", "/api/v1/games/"+gameID+"/answer", map[string]any{"answer": "in"})
	// Bob (a member account) gets no sign-in link: he signs in himself
	a.expect(400, a.alice, "POST", g+"/members/"+itoa(a.ids["Bob"])+"/link", nil)
	// a result between them, in the table for both
	a.ok(a.bob, "POST", g+"/matches", map[string]any{"game_id": n(gameID), "played_at": "2026-05-07T20:00",
		"players": []map[string]any{{"user_id": a.ids["Alice"], "side": "a"}, {"user_id": a.ids["Bob"], "side": "b"}}, "score": map[string]any{"sets": [][]int{{6, 4}, {6, 3}}}})
	tbl := a.ok(a.alice, "GET", g+"/table", nil)
	rows := tbl["rows"].([]any)
	top := rows[0].(map[string]any)
	if top["name"] != "Alice" || num(top, "won") != 1 || num(top, "rating") <= 1000 {
		t.Fatalf("table: %v", rows)
	}
	// Bob leaves: 404 again
	a.ok(a.bob, "POST", g+"/leave", nil)
	a.expect(404, a.bob, "GET", g, nil)
}

func TestPlayersGetAnAccountAndAOneTimeLink(t *testing.T) {
	a := newApp(t, nil)
	gid, gameID := tennis(a)
	g := "/api/v1/groups/" + gid
	r := a.ok(a.alice, "POST", g+"/members", map[string]any{"name": "Marko Petrović", "phone": "+387 61 111 222"})
	m := r["member"].(map[string]any)
	if r["created"] != true || m["can_link"] != true || m["account"] != "player" {
		t.Fatalf("player: %v", r)
	}
	uid := itoa(int64(num(m, "user_id")))
	link := a.ok(a.alice, "POST", g+"/members/"+uid+"/link", nil)
	url := link["url"].(string)
	if !strings.Contains(url, "/join/") || !strings.Contains(link["whatsapp"].(string), "wa.me/38761111222") {
		t.Fatalf("link: %v", link)
	}
	tok := url[strings.LastIndex(url, "/")+1:]
	// a preview does not use it up
	peek := a.ok("", "GET", "/api/v1/auth/link/"+tok, nil)
	if peek["name"] != "Marko Petrović" || peek["invited_by"] != "Alice" {
		t.Fatalf("peek: %v", peek)
	}
	cookie := useLink(a, tok, 200)
	useLink(a, tok, 404) // once only
	// signed in as Marko: sees the group and the game, answers, cannot start a group
	req, _ := http.NewRequest("GET", a.url+"/api/v1/groups", nil)
	req.AddCookie(cookie)
	res, _ := http.DefaultClient.Do(req)
	var gs []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&gs)
	res.Body.Close()
	if len(gs) != 1 || gs[0]["name"] != "Tuesday Tennis" {
		t.Fatalf("Marko's groups: %v", gs)
	}
	body, _ := json.Marshal(map[string]any{"name": "Mine", "sport": "padel"})
	req, _ = http.NewRequest("POST", a.url+"/api/v1/groups", bytes.NewReader(body))
	req.AddCookie(cookie)
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != 403 {
		t.Fatalf("a player started a group: %d", res.StatusCode)
	}
	res.Body.Close()
	// Bob, an organiser of nothing Marko is in, may not send Marko a link
	a.expect(404, a.bob, "POST", g+"/members/"+uid+"/link", nil)
	// a new link stops the old unused one
	l1 := a.ok(a.alice, "POST", g+"/members/"+uid+"/link", nil)["url"].(string)
	a.ok(a.alice, "POST", g+"/members/"+uid+"/link", nil)
	useLink(a, l1[strings.LastIndex(l1, "/")+1:], 404)
	// adding the same phone again finds the same account
	a.expect(409, a.alice, "POST", g+"/members", map[string]any{"name": "Marko", "phone": "00387 61 111 222"})
	_ = gameID
}

func useLink(a *app, tok string, want int) *http.Cookie {
	a.t.Helper()
	body, _ := json.Marshal(map[string]string{"token": tok})
	res, err := http.Post(a.url+"/api/v1/auth/link", "application/json", bytes.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode != want {
		a.t.Fatalf("use link → %d, want %d", res.StatusCode, want)
	}
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	return nil
}

func TestWaitingListMovesUp(t *testing.T) {
	a := newApp(t, nil)
	gid, gameID := tennis(a)
	g := "/api/v1/groups/" + gid
	a.ok(a.alice, "POST", g+"/members", map[string]any{"name": "Bob", "email": "bob@example.com"})
	a.ok(a.alice, "PUT", "/api/v1/games/"+gameID, map[string]any{"max_players": 1, "min_players": 1})
	r := a.ok(a.bob, "POST", "/api/v1/games/"+gameID+"/answer", map[string]any{"answer": "in"})
	if r["result"] != "waiting" {
		t.Fatalf("a full game: %v", r["result"])
	}
	a.ok(a.alice, "POST", "/api/v1/games/"+gameID+"/answer", map[string]any{"answer": "out"})
	if got := a.st.GetAnswer(context.Background(), n(gameID), a.ids["Bob"]); got != store.AnswerIn {
		t.Fatalf("Bob did not move up: %q", got)
	}
}

func TestLinkScopes(t *testing.T) {
	for _, c := range []struct {
		m, p   string
		scopes []string
		ok     bool
	}{
		{"GET", "/api/v1/upcoming", []string{"league:read"}, true},
		{"GET", "/api/v1/groups/3/table", []string{"league:read"}, true},
		{"GET", "/api/v1/day/2026-11-07", []string{"league:read"}, true},
		{"POST", "/api/v1/games/3/answer", []string{"league:read"}, false},
		{"POST", "/api/v1/games/3/answer", []string{"league:write"}, true},
		{"POST", "/api/v1/groups/3/messages", []string{"league:read", "league:write"}, false},
		{"POST", "/api/v1/groups/3/matches", []string{"league:write"}, false},
		{"GET", "/api/v1/settings", []string{"league:read"}, false},
		{"GET", "/api/v1/auth/me", nil, true},
		{"GET", "/api/v1/tournaments/4", []string{"league:read"}, true},
		{"POST", "/api/v1/tmatches/4/result", []string{"league:read", "league:write"}, false},
		{"GET", "/api/v1/groups/3/seasons", []string{"league:read"}, true},
		{"GET", "/api/v1/groups/3/money", []string{"league:read"}, true},
		{"GET", "/api/v1/subs", []string{"league:read"}, true},
		{"POST", "/api/v1/subs/3", []string{"league:read", "league:write"}, false},
		{"POST", "/api/v1/live/3/point", []string{"league:read", "league:write"}, false},
		{"POST", "/api/v1/groups/3/money", []string{"league:read", "league:write"}, false},
		{"GET", "/api/v1/photos/3", []string{"league:read"}, false},
	} {
		r, _ := http.NewRequest(c.m, c.p, nil)
		if got := linkAllows(c.scopes, r); got != c.ok {
			t.Errorf("%s %s with %v: %v, want %v", c.m, c.p, c.scopes, got, c.ok)
		}
	}
}

func TestExportImportTwiceIsOnce(t *testing.T) {
	a := newApp(t, nil)
	gid, gameID := tennis(a)
	g := "/api/v1/groups/" + gid
	a.ok(a.alice, "POST", g+"/members", map[string]any{"name": "Marko", "email": "marko@example.com"})
	marko, _ := a.st.GetUserByEmail(context.Background(), "marko@example.com")
	a.ok(a.alice, "POST", g+"/matches", map[string]any{"game_id": n(gameID), "played_at": "2026-05-07T20:00",
		"players": []map[string]any{{"user_id": a.ids["Alice"], "side": "a"}, {"user_id": marko.ID, "side": "b"}}, "score": map[string]any{"sets": [][]int{{4, 6}, {6, 3}, {10, 8}}}})
	a.ok(a.alice, "POST", g+"/messages", map[string]any{"body": "Good game!"})
	code, doc, _ := a.call(a.alice, "GET", "/api/v1/export.json", nil)
	if code != 200 || len(doc["groups"].([]any)) != 1 {
		t.Fatalf("export: %d %v", code, doc)
	}
	for i := 0; i < 2; i++ {
		r := a.ok(a.bob, "POST", "/api/v1/import", doc)
		im := r["imported"].(map[string]any)
		want := map[bool]float64{true: 1, false: 0}[i == 0]
		if num(im, "groups") != want || num(im, "matches") != want || num(im, "messages") != want {
			t.Fatalf("import %d: %v", i, im)
		}
	}
	if l := a.list(a.bob, "/api/v1/groups"); len(l) != 1 {
		t.Fatalf("Bob's groups after importing twice: %d", len(l))
	}
}

func TestSettingsArePersonal(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"remind_hours": 5, "calendar_auto": false, "notify_url": "https://ntfy.sh/alice"})
	bob := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if num(bob, "remind_hours") != 3 || bob["calendar_auto"] != true || bob["notify_url"] != "" {
		t.Fatalf("Bob got Alice's settings: %v", bob)
	}
	al := a.ok(a.alice, "GET", "/api/v1/settings", nil)
	if num(al, "remind_hours") != 5 || al["calendar_auto"] != false {
		t.Fatalf("Alice's settings: %v", al)
	}
}

func TestPublishesOnlyLeagueEvents(t *testing.T) {
	if len(eventTopics) != 10 {
		t.Fatalf("event topics: %v", eventTopics)
	}
	for _, e := range eventTopics {
		if !strings.HasPrefix(e, "league.") {
			t.Fatalf("not a League event: %s", e)
		}
	}
}

func TestVotingOnTimesAndPlaces(t *testing.T) {
	a := newApp(t, nil)
	gid, _ := tennis(a)
	g := "/api/v1/groups/" + gid
	a.ok(a.alice, "POST", g+"/members", map[string]any{"name": "Bob", "email": "bob@example.com"})
	// a place used once is remembered; the group's own one is there from the start
	vs := a.list(a.alice, g+"/venues")
	if len(vs) != 1 || vs[0].(map[string]any)["name"] != "TK Koševo" {
		t.Fatalf("venues: %v", vs)
	}
	a.expect(403, a.bob, "POST", g+"/venues", map[string]any{"name": "Mine"})
	a.ok(a.alice, "POST", g+"/venues", map[string]any{"name": "TK Zetra", "address": "Alipašina"})
	// the same evening at two places, and another evening: three options (a bare time still works)
	d := a.ok(a.bob, "POST", g+"/games", map[string]any{"options": []any{
		map[string]any{"starts_at": "2030-05-07T20:00", "venue_name": "TK Koševo"},
		map[string]any{"starts_at": "2030-05-07T20:00", "venue_name": "TK Zetra"},
		"2030-05-08T19:00",
	}})
	game := d["game"].(map[string]any)
	opts := d["options"].([]any)
	if game["status"] != "voting" || len(opts) != 3 {
		t.Fatalf("vote: %v %v", game["status"], opts)
	}
	var zetra string
	for _, o := range opts {
		if o.(map[string]any)["venue_name"] == "TK Zetra" {
			zetra = id(o.(map[string]any))
		}
	}
	a.ok(a.alice, "POST", "/api/v1/games/"+id(game)+"/votes", map[string]any{"votes": map[string]string{zetra: "yes"}})
	p := a.ok(a.bob, "POST", "/api/v1/games/"+id(game)+"/pick", map[string]any{"option_id": n(zetra)})
	pg := p["game"].(map[string]any)
	if pg["status"] != "planned" || pg["starts_at"] != "2030-05-07T20:00" || pg["venue_name"] != "TK Zetra" || pg["venue_address"] != "Alipašina" {
		t.Fatalf("picked: %v", pg)
	}
	if got := a.st.GetAnswer(context.Background(), n(id(pg)), a.ids["Alice"]); got != store.AnswerIn {
		t.Fatalf("a yes for the picked option is in: %q", got)
	}
	// the default place can't be removed; another can
	for _, v := range a.list(a.alice, g+"/venues") {
		m := v.(map[string]any)
		want := 204
		if m["name"] == "TK Koševo" {
			want = 400
		}
		a.expect(want, a.alice, "DELETE", g+"/venues/"+id(m), nil)
	}
}
