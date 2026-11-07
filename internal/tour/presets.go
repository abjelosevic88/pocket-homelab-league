package tour

// Format is a kind of tournament people pick from.
type Format struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Entries     []string `json:"entries"` // who enters: player | pair | team
	Rotating    bool     `json:"rotating"`
}

// Formats is every format, in the order they are offered.
var Formats = []Format{
	{Key: "groups_knockout", Name: "Groups, then knockout", Description: "Everyone plays everyone in small groups; the best of each go into a knockout. The classic one-day tournament.", Entries: []string{"player", "pair", "team"}},
	{Key: "knockout", Name: "Knockout", Description: "Lose once and you're out. Byes for the top seeds, a match for third place and a plate for first-round losers if you want.", Entries: []string{"player", "pair", "team"}},
	{Key: "round_robin", Name: "Round robin", Description: "Everyone plays everyone (once, or twice home and away). The table decides.", Entries: []string{"player", "pair", "team"}},
	{Key: "americano", Name: "Americano", Description: "Come alone: partners change every round so everyone plays with everyone. Each match is a set number of points; your points add up.", Entries: []string{"player"}, Rotating: true},
	{Key: "mexicano", Name: "Mexicano", Description: "Like Americano, but after the first round you play with and against people at your level, round by round.", Entries: []string{"player"}, Rotating: true},
	{Key: "kotc", Name: "King of the Court", Description: "Courts in order: winners move up a court, losers down, partners change. Whoever ends on court 1 wins.", Entries: []string{"player"}, Rotating: true},
	{Key: "double_elim", Name: "Double elimination", Description: "You're out after two losses: a winners' bracket, a losers' bracket and a grand final.", Entries: []string{"player", "pair", "team"}},
	{Key: "swiss", Name: "Swiss", Description: "A few rounds; each round you meet someone with the same score, never twice. Good for many players and little time (chess).", Entries: []string{"player", "pair", "team"}},
	{Key: "team_ties", Name: "Team ties", Description: "Teams meet in ties of several matches (two singles and a doubles, …); the team that wins more of them wins the tie.", Entries: []string{"team"}},
	{Key: "ranking", Name: "Scored rounds", Description: "Everyone plays the same rounds and gets a score: golf (stroke play or Stableford, with handicaps), bowling (with handicaps), a darts night.", Entries: []string{"player", "pair", "team"}},
	{Key: "ladder", Name: "Challenge ladder", Description: "A ranking anyone can climb: challenge someone up to a few places above you; win and you take their place. Runs for weeks.", Entries: []string{"player", "pair"}},
	{Key: "box_league", Name: "Box league", Description: "Divisions of four to six play everyone in their box over a few weeks; the best go up a box next round, the last go down.", Entries: []string{"player", "pair"}},
	{Key: "arena", Name: "Arena", Description: "Play as many games as you can in the time: when you finish you get the next opponent near your score. Wins in a row count double.", Entries: []string{"player"}},
}

// FormatByKey finds a format.
func FormatByKey(k string) (Format, bool) {
	for _, f := range Formats {
		if f.Key == k {
			return f, true
		}
	}
	return Format{}, false
}

// Suggested lists the formats that suit a sport, best first.
func Suggested(sport, scoring, format string) []string {
	switch {
	case sport == "padel" || sport == "pickleball":
		return []string{"americano", "mexicano", "kotc", "groups_knockout", "knockout", "round_robin", "box_league", "ladder", "double_elim", "swiss"}
	case sport == "beach_volleyball":
		return []string{"kotc", "groups_knockout", "knockout", "round_robin", "double_elim"}
	case sport == "chess":
		return []string{"swiss", "round_robin", "arena", "knockout", "ladder", "double_elim"}
	case sport == "golf":
		return []string{"ranking", "knockout", "round_robin"}
	case sport == "bowling":
		return []string{"ranking", "round_robin", "knockout"}
	case sport == "darts" || sport == "pool":
		return []string{"double_elim", "round_robin", "groups_knockout", "knockout", "ranking", "arena", "team_ties", "ladder"}
	case scoring == "ranking":
		return []string{"ranking", "knockout"}
	case format == "teams" || scoring == "goals" || sport == "basketball" || sport == "volleyball":
		return []string{"groups_knockout", "round_robin", "knockout", "double_elim", "team_ties"}
	}
	// tennis, badminton, squash, table tennis and the rest
	return []string{"groups_knockout", "round_robin", "knockout", "box_league", "ladder", "team_ties", "double_elim", "swiss", "americano", "mexicano"}
}

// RulesFor are a sport's points and tie-breakers for a kind of stage.
func RulesFor(sport, scoring, stage string) Rules {
	switch stage {
	case "americano", "mexicano", "kotc":
		return Rules{Win: 1, Individual: true, ByFor: true, Tiebreaks: []string{"points", "wins", "diff"}}
	case "swiss":
		r := Rules{Win: 1, Draw: 0.5, Bye: 1, Tiebreaks: []string{"points", "buchholz", "sb", "wins", "rating"}}
		if scoring != "result" {
			r.Tiebreaks = []string{"points", "buchholz", "diff", "for", "rating"}
		}
		return r
	case "arena":
		return Rules{Win: 2, Draw: 1, Tiebreaks: []string{"points", "wins", "rating"}}
	}
	switch {
	case sport == "volleyball":
		return Rules{Win: 3, Volley: true, Tiebreaks: []string{"points", "wins", "set_ratio", "point_ratio", "h2h"}}
	case sport == "beach_volleyball":
		return Rules{Win: 2, Loss: 1, Tiebreaks: []string{"points", "h2h", "set_ratio", "point_ratio"}}
	case sport == "table_tennis":
		return Rules{Win: 2, Loss: 1, PlayedLoss: true, Tiebreaks: []string{"points", "h2h", "set_ratio", "point_ratio"}}
	case sport == "badminton":
		return Rules{Win: 1, Tiebreaks: []string{"points", "h2h", "set_ratio", "game_ratio"}}
	case sport == "basketball":
		return Rules{Win: 2, Loss: 1, Tiebreaks: []string{"points", "h2h", "diff", "for"}}
	case sport == "chess" || scoring == "result":
		return Rules{Win: 1, Draw: 0.5, Tiebreaks: []string{"points", "sb", "h2h", "wins"}}
	case scoring == "goals":
		return Rules{Win: 3, Draw: 1, Tiebreaks: []string{"points", "diff", "for", "h2h"}}
	case scoring == "sets":
		return Rules{Win: 1, Tiebreaks: []string{"points", "h2h", "set_ratio", "game_ratio"}}
	case sport == "darts" || sport == "pool":
		return Rules{Win: 2, Tiebreaks: []string{"points", "diff", "h2h", "for"}}
	}
	return Rules{Win: 2, Draw: 1, Tiebreaks: []string{"points", "diff", "for", "h2h"}}
}
