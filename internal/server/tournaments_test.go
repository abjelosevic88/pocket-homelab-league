package server

import (
	"fmt"
	"testing"
)

// groupWith makes a group of a sport with Alice and n−1 more players.
func groupWith(a *app, sport string, n int) string {
	g := a.ok(a.alice, "POST", "/api/v1/groups", map[string]any{"name": sport + " friends", "sport": sport})
	gid := id(g)
	for i := 1; i < n; i++ {
		a.ok(a.alice, "POST", "/api/v1/groups/"+gid+"/members", map[string]any{"name": fmt.Sprintf("Player %d", i), "email": fmt.Sprintf("p%d-%s@example.com", i, sport)})
	}
	return gid
}

// play enters results for every match that can be played until the
// tournament is over; body says what to send for a match.
func play(a *app, tid string, body func(m map[string]any) map[string]any) map[string]any {
	a.t.Helper()
	for i := 0; i < 400; i++ {
		d := a.ok(a.alice, "GET", "/api/v1/tournaments/"+tid, nil)
		t := d["tournament"].(map[string]any)
		if t["status"] == "finished" {
			return d
		}
		played := false
		for _, x := range d["matches"].([]any) {
			m := x.(map[string]any)
			st := m["status"].(string)
			if (st != "ready" && st != "called") || (m["label"] == "Grand final, reset" && m["a_entry"] == float64(0)) {
				continue
			}
			// a team tie is played through its matches
			if hasChildren(d, m) {
				continue
			}
			a.ok(a.alice, "POST", "/api/v1/tmatches/"+id(m)+"/result", body(m))
			played = true
			break
		}
		if !played {
			a.t.Fatalf("stuck: nothing to play and not finished (%v)", t["status"])
		}
	}
	a.t.Fatal("too many matches")
	return nil
}

func hasChildren(d map[string]any, m map[string]any) bool {
	for _, x := range d["matches"].([]any) {
		if x.(map[string]any)["parent_id"] == m["id"] {
			return true
		}
	}
	return false
}

func start(a *app, gid string, body map[string]any) string {
	a.t.Helper()
	body["add_members"] = true
	d := a.ok(a.alice, "POST", "/api/v1/groups/"+gid+"/tournaments", body)
	tid := id(d["tournament"].(map[string]any))
	a.ok(a.alice, "POST", "/api/v1/tournaments/"+tid+"/start", nil)
	return tid
}

var setsA = func(m map[string]any) map[string]any {
	return map[string]any{"score": map[string]any{"sets": [][]int{{6, 3}, {6, 4}}}}
}

func TestKnockoutAndGroupsToTheEnd(t *testing.T) {
	a := newApp(t, nil)
	gid := groupWith(a, "tennis", 6)
	tid := start(a, gid, map[string]any{"name": "Spring cup", "format": "knockout", "settings": map[string]any{"third_place": true, "plate": true}})
	d := play(a, tid, setsA)
	if d["tournament"].(map[string]any)["winner"] == "" {
		t.Fatalf("no winner: %v", d["tournament"])
	}
	// rated: the matches are in the group's table
	tbl := a.ok(a.alice, "GET", "/api/v1/groups/"+gid+"/table", nil)
	if num(tbl, "matches") < 5 {
		t.Fatalf("rated matches in the table: %v", tbl["matches"])
	}
	// groups then knockout
	gid2 := groupWith(a, "badminton", 8)
	tid2 := start(a, gid2, map[string]any{"name": "Club day", "format": "groups_knockout", "settings": map[string]any{"groups": 2, "advance": 2}})
	d2 := play(a, tid2, func(m map[string]any) map[string]any {
		return map[string]any{"score": map[string]any{"sets": [][]int{{21, 15}, {21, 18}}}}
	})
	if len(d2["stages"].([]any)) != 2 || d2["tournament"].(map[string]any)["winner"] == "" {
		t.Fatalf("groups → knockout: %d stages, winner %v", len(d2["stages"].([]any)), d2["tournament"].(map[string]any)["winner"])
	}
	// a knockout match needs a winner
	gid3 := groupWith(a, "football", 4)
	tid3 := start(a, gid3, map[string]any{"name": "Cup", "format": "knockout", "entry_kind": "player"})
	d3 := a.ok(a.alice, "GET", "/api/v1/tournaments/"+tid3, nil)
	for _, x := range d3["matches"].([]any) {
		m := x.(map[string]any)
		if m["status"] == "ready" || m["status"] == "called" {
			a.expect(400, a.alice, "POST", "/api/v1/tmatches/"+id(m)+"/result", map[string]any{"score": map[string]any{"a": 1, "b": 1}})
			break
		}
	}
}

func TestDoubleEliminationAndSwiss(t *testing.T) {
	a := newApp(t, nil)
	gid := groupWith(a, "darts", 5)
	tid := start(a, gid, map[string]any{"name": "Darts night", "format": "double_elim", "entry_kind": "player"})
	turn := 0
	d := play(a, tid, func(m map[string]any) map[string]any {
		turn++
		if turn%3 == 0 {
			return map[string]any{"score": map[string]any{"a": 1, "b": 3}}
		}
		return map[string]any{"score": map[string]any{"a": 3, "b": 1}}
	})
	if d["tournament"].(map[string]any)["winner"] == "" {
		t.Fatal("double elimination: no winner")
	}
	gid2 := groupWith(a, "chess", 5)
	tid2 := start(a, gid2, map[string]any{"name": "Rapid", "format": "swiss", "settings": map[string]any{"rounds": 3}})
	d2 := play(a, tid2, func(m map[string]any) map[string]any { return map[string]any{"winner": "a"} })
	rounds := map[float64]bool{}
	byes := 0
	for _, x := range d2["matches"].([]any) {
		m := x.(map[string]any)
		rounds[m["round"].(float64)] = true
		if m["outcome"] == "bye" {
			byes++
		}
	}
	if len(rounds) != 3 || byes != 3 {
		t.Fatalf("swiss: %d rounds, %d byes", len(rounds), byes)
	}
}

func TestAmericanoMexicanoKOTC(t *testing.T) {
	a := newApp(t, nil)
	gid := groupWith(a, "padel", 8)
	pts := func(m map[string]any) map[string]any { return map[string]any{"a_total": 14, "b_total": 10} }
	for _, f := range []string{"americano", "mexicano", "kotc"} {
		tid := start(a, gid, map[string]any{"name": f, "format": f, "settings": map[string]any{"rounds": 4}})
		d := play(a, tid, pts)
		st := d["stages"].([]any)[0].(map[string]any)
		var rows []any
		for _, v := range st["tables"].(map[string]any) {
			rows = v.([]any)
		}
		if len(rows) != 8 {
			t.Fatalf("%s: %d in the table", f, len(rows))
		}
		top := rows[0].(map[string]any)
		if num(top, "points") < 40 || d["tournament"].(map[string]any)["winner"] == "" {
			t.Fatalf("%s: top %v", f, top)
		}
	}
}

func TestScoredRoundsLadderAndTeamTies(t *testing.T) {
	a := newApp(t, nil)
	// golf, Stableford over 18 holes
	gid := groupWith(a, "golf", 3)
	tid := start(a, gid, map[string]any{"name": "Society day", "format": "ranking"})
	d := a.ok(a.alice, "GET", "/api/v1/tournaments/"+tid, nil)
	stage := id(d["stages"].([]any)[0].(map[string]any))
	for i, x := range d["entries"].([]any) {
		e := x.(map[string]any)
		holes := make([]float64, 18)
		for h := range holes {
			holes[h] = float64(4 + (h+i)%3)
		}
		a.ok(a.alice, "PUT", "/api/v1/tournaments/"+tid+"/scorecards", map[string]any{"stage_id": n(stage), "round": 1, "entry_id": num(e, "id"), "holes": holes})
	}
	d = a.ok(a.alice, "POST", "/api/v1/tournaments/"+tid+"/finish", nil)
	if d["tournament"].(map[string]any)["winner"] == "" {
		t.Fatal("golf: no winner")
	}
	// a ladder: the 3rd challenges the 1st and wins
	gid2 := groupWith(a, "squash", 4)
	tid2 := start(a, gid2, map[string]any{"name": "Ladder", "format": "ladder"})
	d2 := a.ok(a.alice, "GET", "/api/v1/tournaments/"+tid2, nil)
	pos := map[float64]float64{}
	for _, x := range d2["entries"].([]any) {
		e := x.(map[string]any)
		pos[num(e, "position")] = num(e, "id")
	}
	a.ok(a.alice, "POST", "/api/v1/tournaments/"+tid2+"/challenges", map[string]any{"challenger": pos[3], "defender": pos[1]})
	d2 = a.ok(a.alice, "GET", "/api/v1/tournaments/"+tid2, nil)
	for _, x := range d2["matches"].([]any) {
		m := x.(map[string]any)
		a.ok(a.alice, "POST", "/api/v1/tmatches/"+id(m)+"/result", map[string]any{"score": map[string]any{"sets": [][]int{{11, 5}, {11, 7}, {11, 9}}}})
	}
	d2 = a.ok(a.alice, "GET", "/api/v1/tournaments/"+tid2, nil)
	for _, x := range d2["entries"].([]any) {
		e := x.(map[string]any)
		if num(e, "id") == pos[3] && num(e, "position") != 1 {
			t.Fatalf("the challenger who won is at %v", num(e, "position"))
		}
		if num(e, "id") == pos[1] && num(e, "position") != 2 {
			t.Fatalf("the one beaten is at %v", num(e, "position"))
		}
	}
	// team ties: six players make two teams; each tie is two singles and a doubles
	gid3 := groupWith(a, "table_tennis", 6)
	d3 := a.ok(a.alice, "POST", "/api/v1/groups/"+gid3+"/tournaments", map[string]any{"name": "Club match", "format": "team_ties", "entry_kind": "team"})
	tid3 := id(d3["tournament"].(map[string]any))
	// the sign-ups come one by one, then become teams
	members := a.ok(a.alice, "GET", "/api/v1/groups/"+gid3, nil)["members"].([]any)
	for _, x := range members {
		mm := x.(map[string]any)
		a.ok(a.alice, "POST", "/api/v1/tournaments/"+tid3+"/entries", map[string]any{"members": []float64{num(mm, "user_id")}})
	}
	a.ok(a.alice, "POST", "/api/v1/tournaments/"+tid3+"/teams", map[string]any{"teams": 2})
	a.ok(a.alice, "POST", "/api/v1/tournaments/"+tid3+"/start", nil)
	d3 = play(a, tid3, func(m map[string]any) map[string]any {
		return map[string]any{"score": map[string]any{"sets": [][]int{{11, 6}, {11, 8}, {11, 4}}}}
	})
	if d3["tournament"].(map[string]any)["winner"] == "" {
		t.Fatal("team ties: no winner")
	}
}

func TestTournamentsBelongToTheGroup(t *testing.T) {
	a := newApp(t, nil)
	gid := groupWith(a, "tennis", 4)
	d := a.ok(a.alice, "POST", "/api/v1/groups/"+gid+"/tournaments", map[string]any{"name": "Cup", "format": "knockout"})
	tid := id(d["tournament"].(map[string]any))
	a.expect(404, a.bob, "GET", "/api/v1/tournaments/"+tid, nil)
	a.expect(404, a.bob, "POST", "/api/v1/tournaments/"+tid+"/join", nil)
	// the share link works without signing in, and shows no emails
	tok := d["share_url"].(string)
	tok = tok[len(tok)-24:]
	pub := a.ok("", "GET", "/api/v1/public/t/"+tok, nil)
	if pub["can_run"] != false || pub["log"] != nil {
		t.Fatalf("public view: %v", pub["can_run"])
	}
	// Bob joins the group: he can sign up but not start it
	a.ok(a.alice, "POST", "/api/v1/groups/"+gid+"/members", map[string]any{"name": "Bob", "email": "bob@example.com"})
	a.ok(a.bob, "POST", "/api/v1/tournaments/"+tid+"/join", nil)
	a.expect(409, a.bob, "POST", "/api/v1/tournaments/"+tid+"/join", nil)
	a.expect(403, a.bob, "POST", "/api/v1/tournaments/"+tid+"/start", nil)
}
