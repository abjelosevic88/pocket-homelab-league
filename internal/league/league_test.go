package league

import (
	"testing"

	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

func f(v float64) *float64 { return &v }

func TestCheckWorksOutTheWinner(t *testing.T) {
	tennis := Rules{Scoring: ScoringSets, Format: FormatSingles}
	m := &store.Match{Players: []store.MatchPlayer{{UserID: 1, Side: "a"}, {UserID: 2, Side: "b"}}, Score: store.Score{Sets: [][2]int{{4, 6}, {7, 5}, {6, 2}}}}
	if err := Check(tennis, m); err != nil || m.Winner != "a" || ScoreText(tennis, m) != "4–6 7–5 6–2" {
		t.Fatalf("tennis: %v %q", err, m.Winner)
	}
	bad := &store.Match{Players: []store.MatchPlayer{{UserID: 1, Side: "a"}, {UserID: 2, Side: "a"}, {UserID: 3, Side: "b"}}, Score: store.Score{Sets: [][2]int{{6, 0}}}}
	if Check(tennis, bad) == nil {
		t.Fatal("two against one in singles")
	}
	foot := Rules{Scoring: ScoringGoals, Format: FormatTeams}
	d := &store.Match{Players: []store.MatchPlayer{{UserID: 1, Side: "a"}, {UserID: 2, Side: "b"}}, Score: store.Score{A: f(2), B: f(2)}}
	if err := Check(foot, d); err != nil || d.Winner != "draw" {
		t.Fatalf("draw: %v %q", err, d.Winner)
	}
	golf := Rules{Scoring: ScoringRanking, Format: FormatEveryone, LowerBetter: true}
	g := &store.Match{Players: []store.MatchPlayer{{UserID: 1, Score: f(82)}, {UserID: 2, Score: f(79)}, {UserID: 3, Score: f(82)}}}
	if err := Check(golf, g); err != nil || g.Players[1].Place != 1 || g.Players[0].Place != 2 || g.Players[2].Place != 2 {
		t.Fatalf("golf places: %v %+v", err, g.Players)
	}
}

func TestTableAndRatings(t *testing.T) {
	r := Rules{Scoring: ScoringGoals, Format: FormatSingles, Win: 3, Draw: 1}
	ms := []*store.Match{
		{ID: 1, Players: []store.MatchPlayer{{UserID: 1, Side: "a"}, {UserID: 2, Side: "b"}}, Score: store.Score{A: f(3), B: f(1)}, Winner: "a"},
		{ID: 2, Players: []store.MatchPlayer{{UserID: 1, Side: "a"}, {UserID: 3, Side: "b"}}, Score: store.Score{A: f(1), B: f(1)}, Winner: "draw"},
		{ID: 3, Players: []store.MatchPlayer{{UserID: 2, Side: "a"}, {UserID: 3, Side: "b"}}, Score: store.Score{A: f(0), B: f(5)}, Winner: "b", DisputedBy: 9},
	}
	tb := Compute(r, ms, []int64{1, 2, 3, 4})
	top := tb.Rows[0]
	if top.UserID != 1 || top.Points != 4 || top.Won != 1 || top.Drawn != 1 || top.For != 4 || top.Rating <= StartRating {
		t.Fatalf("top: %+v", top)
	}
	for _, row := range tb.Rows {
		if row.UserID == 3 && row.Played != 1 {
			t.Fatalf("a disputed result counted: %+v", row)
		}
		if row.UserID == 4 && (row.Played != 0 || row.Position != 0) {
			t.Fatalf("someone who never played: %+v", row)
		}
	}
	h := HeadToHead(1, ms)
	if h[0].Played != 1 || (h[0].Won+h[0].Drawn) != 1 {
		t.Fatalf("head to head: %+v", h[0])
	}
}

func TestFairTeamsAreFair(t *testing.T) {
	ratings := map[int64]float64{1: 1200, 2: 1180, 3: 1000, 4: 990, 5: 820, 6: 800}
	a, b, diff := FairTeams([]int64{1, 2, 3, 4, 5, 6}, ratings, 7)
	if len(a) != 3 || len(b) != 3 || diff > 20 {
		t.Fatalf("teams %v %v differ by %v", a, b, diff)
	}
	// the two best never together
	together := func(x []int64) bool {
		n := 0
		for _, v := range x {
			if v == 1 || v == 2 {
				n++
			}
		}
		return n == 2
	}
	if together(a) || together(b) {
		t.Fatalf("the two best on one side: %v %v", a, b)
	}
}
