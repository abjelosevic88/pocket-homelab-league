// Package league holds what does not need a database: the sports and how
// each one is scored, the league table with its ratings, and fair teams.
package league

import "strings"

// Ways of scoring.
const (
	ScoringSets    = "sets"    // tennis: sets with games in each (6–4 3–6 7–5)
	ScoringGoals   = "goals"   // football: one number per side, a draw is possible
	ScoringPoints  = "points"  // basketball, darts legs: one number per side
	ScoringResult  = "result"  // chess: only who won (or a draw)
	ScoringRanking = "ranking" // golf, bowling: everyone on their own, a score each
)

// Formats.
const (
	FormatSingles  = "singles"  // one against one
	FormatDoubles  = "doubles"  // two against two
	FormatTeams    = "teams"    // two teams picked on the day
	FormatEveryone = "everyone" // everyone on their own (a ranking)
)

// Sport is one sport and how it is usually played.
type Sport struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Emoji       string   `json:"emoji"`
	Scoring     string   `json:"scoring"`
	Format      string   `json:"format"`
	Formats     []string `json:"formats"`
	Min         int      `json:"min_players"`
	Max         int      `json:"max_players"`
	Outdoor     bool     `json:"outdoor"`
	LowerBetter bool     `json:"lower_better"`
	Minutes     int      `json:"minutes"`
	Unit        string   `json:"unit"`   // what one point is called: "game", "goal", "point", "stroke"
	Health      string   `json:"health"` // the kind of workout in Pocket Health
	Draws       bool     `json:"draws"`
	Color       string   `json:"color"`
	Stats       []string `json:"stats"` // what is counted per player by default
}

// Stat is something counted per player in a match.
type Stat struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// Stats are the things a group can count per player.
var Stats = []Stat{
	{"assists", "Assists", "🅰️"}, {"yellow", "Yellow cards", "🟨"}, {"red", "Red cards", "🟥"}, {"saves", "Saves", "🧤"},
	{"aces", "Aces", "🎯"}, {"blocks", "Blocks", "🧱"}, {"rebounds", "Rebounds", "🏀"}, {"steals", "Steals", "🖐️"},
	{"threes", "Three-pointers", "3️⃣"}, {"double_faults", "Double faults", "❌"}, {"winners", "Winners", "⚡"},
	{"checkouts", "Checkouts", "✅"}, {"180s", "180s", "💯"}, {"breaks", "Breaks", "🎱"}, {"strikes", "Strikes", "🎳"}, {"birdies", "Birdies", "🐦"},
}

// ValidStat reports whether a stat exists.
func ValidStat(k string) bool {
	for _, s := range Stats {
		if s.Key == k {
			return true
		}
	}
	return false
}

// DefaultStats per sport.
var defaultStats = map[string][]string{
	"football": {"assists", "yellow", "red", "saves"}, "futsal": {"assists", "yellow", "saves"}, "handball": {"assists", "saves"}, "hockey": {"assists", "saves"},
	"basketball": {"rebounds", "assists", "threes", "steals"}, "volleyball": {"aces", "blocks"}, "beach_volleyball": {"aces", "blocks"},
	"tennis": {"aces", "double_faults"}, "padel": {"winners"}, "darts": {"180s", "checkouts"}, "pool": {"breaks"}, "bowling": {"strikes"}, "golf": {"birdies"},
}

func init() {
	for i := range Sports {
		Sports[i].Stats = defaultStats[Sports[i].Key]
		if Sports[i].Stats == nil {
			Sports[i].Stats = []string{}
		}
	}
}

var vs = []string{FormatSingles, FormatDoubles}

// Sports is the list people pick from; "other" lets them name their own.
var Sports = []Sport{
	{Key: "tennis", Name: "Tennis", Emoji: "🎾", Scoring: ScoringSets, Format: FormatSingles, Formats: vs, Min: 2, Max: 4, Outdoor: true, Minutes: 90, Unit: "game", Health: "tennis", Color: "#c2d500"},
	{Key: "padel", Name: "Padel", Emoji: "🎾", Scoring: ScoringSets, Format: FormatDoubles, Formats: vs, Min: 4, Max: 4, Minutes: 90, Unit: "game", Health: "tennis", Color: "#1c7ed6"},
	{Key: "pickleball", Name: "Pickleball", Emoji: "🏓", Scoring: ScoringSets, Format: FormatDoubles, Formats: vs, Min: 2, Max: 4, Minutes: 60, Unit: "point", Health: "tennis", Color: "#12b886"},
	{Key: "badminton", Name: "Badminton", Emoji: "🏸", Scoring: ScoringSets, Format: FormatSingles, Formats: vs, Min: 2, Max: 4, Minutes: 60, Unit: "point", Health: "tennis", Color: "#7950f2"},
	{Key: "squash", Name: "Squash", Emoji: "🎾", Scoring: ScoringSets, Format: FormatSingles, Formats: []string{FormatSingles}, Min: 2, Max: 2, Minutes: 45, Unit: "point", Health: "tennis", Color: "#e8590c"},
	{Key: "table_tennis", Name: "Table tennis", Emoji: "🏓", Scoring: ScoringSets, Format: FormatSingles, Formats: vs, Min: 2, Max: 4, Minutes: 60, Unit: "point", Health: "table_tennis", Color: "#e03131"},
	{Key: "volleyball", Name: "Volleyball", Emoji: "🏐", Scoring: ScoringSets, Format: FormatTeams, Formats: []string{FormatTeams}, Min: 8, Max: 14, Minutes: 90, Unit: "point", Health: "volleyball", Color: "#f59f00"},
	{Key: "beach_volleyball", Name: "Beach volleyball", Emoji: "🏐", Scoring: ScoringSets, Format: FormatDoubles, Formats: []string{FormatDoubles, FormatTeams}, Min: 4, Max: 8, Outdoor: true, Minutes: 90, Unit: "point", Health: "volleyball", Color: "#fab005"},
	{Key: "football", Name: "Football", Emoji: "⚽", Scoring: ScoringGoals, Format: FormatTeams, Formats: []string{FormatTeams}, Min: 10, Max: 14, Outdoor: true, Minutes: 90, Unit: "goal", Health: "football", Draws: true, Color: "#2f9e44"},
	{Key: "futsal", Name: "Futsal / 5-a-side", Emoji: "⚽", Scoring: ScoringGoals, Format: FormatTeams, Formats: []string{FormatTeams}, Min: 8, Max: 12, Minutes: 60, Unit: "goal", Health: "football", Draws: true, Color: "#37b24d"},
	{Key: "basketball", Name: "Basketball", Emoji: "🏀", Scoring: ScoringPoints, Format: FormatTeams, Formats: []string{FormatTeams}, Min: 6, Max: 12, Outdoor: true, Minutes: 60, Unit: "point", Health: "basketball", Color: "#f76707"},
	{Key: "handball", Name: "Handball", Emoji: "🤾", Scoring: ScoringGoals, Format: FormatTeams, Formats: []string{FormatTeams}, Min: 10, Max: 16, Minutes: 60, Unit: "goal", Health: "other", Draws: true, Color: "#4263eb"},
	{Key: "hockey", Name: "Hockey", Emoji: "🏒", Scoring: ScoringGoals, Format: FormatTeams, Formats: []string{FormatTeams}, Min: 8, Max: 16, Minutes: 60, Unit: "goal", Health: "other", Draws: true, Color: "#1098ad"},
	{Key: "darts", Name: "Darts", Emoji: "🎯", Scoring: ScoringPoints, Format: FormatSingles, Formats: vs, Min: 2, Max: 8, Minutes: 120, Unit: "leg", Health: "other", Color: "#c92a2a"},
	{Key: "pool", Name: "Pool / snooker", Emoji: "🎱", Scoring: ScoringPoints, Format: FormatSingles, Formats: vs, Min: 2, Max: 8, Minutes: 120, Unit: "frame", Health: "other", Color: "#212529"},
	{Key: "chess", Name: "Chess", Emoji: "♟️", Scoring: ScoringResult, Format: FormatSingles, Formats: []string{FormatSingles}, Min: 2, Max: 20, Minutes: 60, Unit: "", Health: "", Draws: true, Color: "#495057"},
	{Key: "bowling", Name: "Bowling", Emoji: "🎳", Scoring: ScoringRanking, Format: FormatEveryone, Formats: []string{FormatEveryone}, Min: 2, Max: 12, Minutes: 90, Unit: "pin", Health: "other", Color: "#ae3ec9"},
	{Key: "golf", Name: "Golf", Emoji: "⛳", Scoring: ScoringRanking, Format: FormatEveryone, Formats: []string{FormatEveryone}, Min: 2, Max: 8, Outdoor: true, LowerBetter: true, Minutes: 240, Unit: "stroke", Health: "golf", Color: "#2b8a3e"},
	{Key: "other", Name: "Something else", Emoji: "🏅", Scoring: ScoringPoints, Format: FormatSingles, Formats: []string{FormatSingles, FormatDoubles, FormatTeams, FormatEveryone}, Min: 2, Max: 0, Minutes: 60, Unit: "point", Health: "other", Draws: true, Color: "#868e96"},
}

// SportByKey finds a sport ("other" when unknown).
func SportByKey(key string) Sport {
	for _, s := range Sports {
		if s.Key == key {
			return s
		}
	}
	return Sports[len(Sports)-1]
}

// ValidSport reports whether a key is in the list.
func ValidSport(key string) bool {
	for _, s := range Sports {
		if s.Key == key {
			return true
		}
	}
	return false
}

// ValidScoring reports whether a way of scoring exists.
func ValidScoring(v string) bool {
	switch v {
	case ScoringSets, ScoringGoals, ScoringPoints, ScoringResult, ScoringRanking:
		return true
	}
	return false
}

// ValidFormat reports whether a format exists.
func ValidFormat(v string) bool {
	switch v {
	case FormatSingles, FormatDoubles, FormatTeams, FormatEveryone:
		return true
	}
	return false
}

// SideSize is how many play on a side ("teams" and "everyone": any number).
func SideSize(format string) int {
	switch format {
	case FormatSingles:
		return 1
	case FormatDoubles:
		return 2
	}
	return 0
}

// DisplayName is the sport's name, or the group's own for "other".
func DisplayName(key, own string) string {
	if key == "other" && strings.TrimSpace(own) != "" {
		return strings.TrimSpace(own)
	}
	return SportByKey(key).Name
}
