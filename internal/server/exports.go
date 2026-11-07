package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-league/internal/league"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- your groups, out and in ----
//
// The export is one person's groups as they see them: the people (with
// email and phone only where they organise), the games with answers, the
// results and the chat. Importing it — on this box or another — brings the
// groups back for the importer as organiser; importing twice is the same
// as once (every group, game and result remembers where it came from).

type exportDoc struct {
	App      string        `json:"app"`
	Version  string        `json:"version"`
	Exported string        `json:"exported_at"`
	Groups   []exportGroup `json:"groups"`
}

type exportPerson struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Role     string `json:"role"`
	Nickname string `json:"nickname,omitempty"`
}

type exportGame struct {
	Key     string            `json:"key"`
	Game    *store.Game       `json:"game"`
	Answers map[string]string `json:"answers"` // person key → answer
}

type exportMatch struct {
	Key      string            `json:"key"`
	GameKey  string            `json:"game_key,omitempty"`
	PlayedAt string            `json:"played_at"`
	Score    store.Score       `json:"score"`
	Winner   string            `json:"winner"`
	Note     string            `json:"note,omitempty"`
	Text     string            `json:"text"`
	Players  []exportMatchSide `json:"players"`
}

type exportMatchSide struct {
	Person string   `json:"person"`
	Side   string   `json:"side"`
	Score  *float64 `json:"score,omitempty"`
}

type exportMessage struct {
	Key    string `json:"key"`
	Person string `json:"person"`
	Body   string `json:"body"`
	At     string `json:"at"`
}

type exportGroup struct {
	Key      string          `json:"key"`
	Group    *store.Group    `json:"group"`
	People   []exportPerson  `json:"people"`
	Games    []exportGame    `json:"games"`
	Matches  []exportMatch   `json:"matches"`
	Messages []exportMessage `json:"messages"`
	Venues   []store.Venue   `json:"venues"`
}

func personKey(m *store.Member) string {
	switch {
	case m.Email != "":
		return "email:" + strings.ToLower(m.Email)
	case m.Phone != "":
		return "phone:" + m.Phone
	}
	return fmt.Sprintf("user:%d", m.UserID)
}

func (s *Server) export(ctx context.Context, uid int64) exportDoc {
	doc := exportDoc{App: "pocket-league", Version: Version, Exported: store.Now(), Groups: []exportGroup{}}
	gs, _ := s.store.ListGroups(ctx, uid, true)
	for _, g := range gs {
		eg := exportGroup{Key: firstNonEmpty(s.importKeyOf(ctx, g), fmt.Sprintf("league:group:%d", g.ID)), Group: g,
			People: []exportPerson{}, Games: []exportGame{}, Matches: []exportMatch{}, Messages: []exportMessage{}}
		eg.Venues, _ = s.store.Venues(ctx, g.ID)
		ms, _ := s.store.Members(ctx, g.ID)
		keys := map[int64]string{}
		for _, m := range ms {
			k := personKey(m)
			keys[m.UserID] = k
			p := exportPerson{Key: k, Name: m.Name, Role: m.Role, Nickname: m.Nickname}
			if organiser(g) || m.UserID == uid {
				p.Email, p.Phone = m.Email, m.Phone
			} else if strings.HasPrefix(k, "email:") || strings.HasPrefix(k, "phone:") {
				p.Key = fmt.Sprintf("user:%d", m.UserID)
				keys[m.UserID] = p.Key
			}
			eg.People = append(eg.People, p)
		}
		games, _ := s.store.ListGames(ctx, uid, store.GameFilter{GroupID: g.ID})
		gameKeys := map[int64]string{}
		for _, gm := range games {
			k := fmt.Sprintf("game:%d", gm.ID)
			gameKeys[gm.ID] = k
			ans, _ := s.store.Answers(ctx, gm.ID)
			am := map[string]string{}
			for _, a := range ans {
				if pk := keys[a.UserID]; pk != "" {
					am[pk] = a.Answer
				}
			}
			eg.Games = append(eg.Games, exportGame{Key: k, Game: gm, Answers: am})
		}
		matches, _ := s.store.ListMatches(ctx, uid, store.MatchFilter{GroupID: g.ID, Oldest: true})
		rules := league.RulesOf(g)
		names := s.names(ctx, g.ID)
		for _, m := range matches {
			em := exportMatch{Key: fmt.Sprintf("match:%d", m.ID), GameKey: gameKeys[m.GameID], PlayedAt: m.PlayedAt, Score: m.Score, Winner: m.Winner,
				Note: m.Note, Text: s.resultLine(g, m, names), Players: []exportMatchSide{}}
			_ = rules
			for _, p := range m.Players {
				em.Players = append(em.Players, exportMatchSide{Person: firstNonEmpty(keys[p.UserID], fmt.Sprintf("user:%d", p.UserID)), Side: p.Side, Score: p.Score})
			}
			eg.Matches = append(eg.Matches, em)
		}
		before := int64(0)
		for {
			page, _ := s.store.ListMessages(ctx, store.MessageFilter{GroupID: g.ID, Before: before, Limit: 200})
			if len(page) == 0 {
				break
			}
			chunk := []exportMessage{}
			for _, m := range page {
				if m.Kind != "text" || m.DeletedAt != nil {
					continue
				}
				chunk = append(chunk, exportMessage{Key: fmt.Sprintf("msg:%d", m.ID), Person: keys[m.UserID], Body: m.Body, At: m.CreatedAt})
			}
			eg.Messages = append(chunk, eg.Messages...)
			before = page[0].ID
			if len(page) < 200 {
				break
			}
		}
		doc.Groups = append(doc.Groups, eg)
	}
	return doc
}

func (s *Server) importKeyOf(ctx context.Context, g *store.Group) string {
	var k string
	_ = s.store.DB.QueryRowContext(ctx, `SELECT import_key FROM groups WHERE id = ?`, g.ID).Scan(&k)
	return k
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-league-export.json"`)
	writeJSON(w, http.StatusOK, s.export(r.Context(), actorID(r.Context())))
}

// handleExportCSV is every result of the person's groups, one per line.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-league-results.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Played", "Group", "Sport", "Side A", "Side B", "Score", "Winner", "Result", "Note"})
	gs, _ := s.store.ListGroups(ctx, uid, true)
	for _, g := range gs {
		names := s.names(ctx, g.ID)
		rules := league.RulesOf(g)
		ms, _ := s.store.ListMatches(ctx, uid, store.MatchFilter{GroupID: g.ID, Oldest: true})
		for _, m := range ms {
			side := func(sd string) string {
				var p []string
				for _, x := range m.Players {
					if x.Side == sd {
						p = append(p, names[x.UserID])
					}
				}
				return strings.Join(p, " & ")
			}
			_ = cw.Write([]string{m.PlayedAt, g.Name, league.DisplayName(g.Sport, g.SportName), side("a"), side("b"), league.ScoreText(rules, m), m.Winner, s.resultLine(g, m, names), m.Note})
		}
	}
	cw.Flush()
}

// handleImport takes a Pocket League export (JSON).
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	if isPlayer(u) {
		writeErr(w, http.StatusForbidden, "players can't import groups")
		return
	}
	data, err := s.readUpload(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var doc exportDoc
	if err := json.Unmarshal(data, &doc); err != nil || doc.App != "pocket-league" {
		writeErr(w, http.StatusBadRequest, "this is not a Pocket League export")
		return
	}
	writeJSON(w, http.StatusOK, s.importDoc(ctx, u, doc))
}

func (s *Server) readUpload(r *http.Request) ([]byte, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		f, _, err := r.FormFile("file")
		if err != nil {
			return nil, errors.New("send the file as 'file'")
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, 64<<20))
	}
	return s.readBody(r)
}

// person finds (or makes) the account behind an exported person.
func (s *Server) importPerson(ctx context.Context, me *store.User, p exportPerson) *store.User {
	email, phone := "", ""
	switch {
	case strings.HasPrefix(p.Key, "email:"):
		email = strings.TrimPrefix(p.Key, "email:")
	case strings.HasPrefix(p.Key, "phone:"):
		phone = strings.TrimPrefix(p.Key, "phone:")
	}
	email = firstNonEmpty(strings.ToLower(p.Email), email)
	phone = firstNonEmpty(p.Phone, phone)
	if email == "" && phone == "" {
		return nil // only a name: nobody to find again
	}
	if u, err := s.store.FindPlayer(ctx, email, phone); err == nil {
		return u
	}
	u := &store.User{Email: email, Phone: phone, Name: firstNonEmpty(p.Name, email, phone), Role: store.RolePlayer, InvitedBy: me.ID}
	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil
	}
	return u
}

func (s *Server) importDoc(ctx context.Context, me *store.User, doc exportDoc) map[string]any {
	res := map[string]int{"groups": 0, "people": 0, "games": 0, "matches": 0, "messages": 0, "skipped": 0}
	for _, eg := range doc.Groups {
		if eg.Group == nil || eg.Key == "" {
			continue
		}
		gid := s.store.ImportedID(ctx, "groups", me.ID, eg.Key)
		var g *store.Group
		if gid == 0 {
			g = eg.Group
			g.ID = 0
			if !league.ValidSport(g.Sport) {
				g.Sport = "other"
			}
			if err := s.store.CreateGroup(ctx, me.ID, g); err != nil {
				res["skipped"]++
				continue
			}
			_ = s.store.SetImportKey(ctx, "groups", g.ID, eg.Key)
			res["groups"]++
		} else {
			var err error
			if g, err = s.store.GetGroup(ctx, me.ID, gid); err != nil {
				res["skipped"]++
				continue
			}
		}
		for _, v := range eg.Venues {
			_ = s.store.EnsureVenue(ctx, g.ID, store.Venue{Name: v.Name, Address: v.Address, Lat: v.Lat, Lon: v.Lon})
		}
		people := map[string]int64{}
		for _, p := range eg.People {
			u := s.importPerson(ctx, me, p)
			if u == nil {
				continue
			}
			people[p.Key] = u.ID
			if added, _ := s.store.AddMember(ctx, g.ID, u.ID, me.ID, p.Role); added {
				res["people"]++
				if p.Nickname != "" {
					_ = s.store.UpdateMember(ctx, g.ID, u.ID, nil, &p.Nickname)
				}
			}
		}
		games := map[string]int64{}
		for _, eg2 := range eg.Games {
			if eg2.Game == nil {
				continue
			}
			if id := s.store.ImportedID(ctx, "games", g.ID, eg2.Key); id != 0 {
				games[eg2.Key] = id
				continue
			}
			gm := eg2.Game
			gm.ID, gm.GroupID = 0, g.ID
			if gm.Status == store.GameVoting {
				gm.Status = store.GameCancelled
			}
			if err := s.store.CreateGame(ctx, me.ID, gm); err != nil {
				res["skipped"]++
				continue
			}
			_ = s.store.SetImportKey(ctx, "games", gm.ID, eg2.Key)
			games[eg2.Key] = gm.ID
			for pk, a := range eg2.Answers {
				if uid := people[pk]; uid != 0 && oneOf(a, []string{store.AnswerIn, store.AnswerMaybe, store.AnswerOut, store.AnswerWaiting}) {
					_ = s.store.SetAnswer(ctx, gm.ID, uid, a, "")
				}
			}
			res["games"]++
		}
		for _, em := range eg.Matches {
			if s.store.ImportedID(ctx, "matches", g.ID, em.Key) != 0 {
				continue
			}
			m := &store.Match{GroupID: g.ID, GameID: games[em.GameKey], PlayedAt: em.PlayedAt, Score: em.Score, Winner: em.Winner, Note: em.Note}
			ok := true
			for _, p := range em.Players {
				uid := people[p.Person]
				if uid == 0 {
					ok = false
					break
				}
				m.Players = append(m.Players, store.MatchPlayer{UserID: uid, Side: p.Side, Score: p.Score})
			}
			if !ok || league.Check(league.RulesOf(g), m) != nil {
				res["skipped"]++
				continue
			}
			if err := s.store.SaveMatch(ctx, me.ID, m); err != nil {
				res["skipped"]++
				continue
			}
			_ = s.store.SetImportKey(ctx, "matches", m.ID, em.Key)
			_ = s.store.ConfirmMatch(ctx, m.ID, me.ID)
			res["matches"]++
		}
		for _, em := range eg.Messages {
			uid := people[em.Person]
			if uid == 0 || strings.TrimSpace(em.Body) == "" {
				continue
			}
			m := &store.Message{GroupID: g.ID, UserID: uid, Kind: "text", Body: clip(em.Body, 4000), ClientID: "imp:" + eg.Key + ":" + em.Key}
			if isNew, err := s.store.AddMessage(ctx, m); err == nil && isNew {
				res["messages"]++
			}
		}
	}
	return map[string]any{"imported": res}
}

// handleImportPlayers adds a list of people (a CSV from Spond, TeamSnap,
// Heja, a spreadsheet or a WhatsApp group's contacts: name, email, phone in
// any order, with a header line) to a group. Someone already in is skipped.
func (s *Server) handleImportPlayers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	me := userFrom(ctx)
	data, err := s.readUpload(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	people, err := parsePeopleCSV(data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	added, created, skipped := 0, 0, 0
	for _, p := range people {
		u, err := s.store.FindPlayer(ctx, p.Email, p.Phone)
		if err != nil {
			u = &store.User{Email: p.Email, Phone: p.Phone, Name: firstNonEmpty(p.Name, p.Email, p.Phone), Role: store.RolePlayer, InvitedBy: me.ID}
			if err := s.store.CreateUser(ctx, u); err != nil {
				skipped++
				continue
			}
			created++
		}
		if ok, _ := s.store.AddMember(ctx, g.ID, u.ID, me.ID, "player"); ok {
			added++
		} else {
			skipped++
		}
	}
	if added > 0 {
		s.systemMessage(ctx, g, 0, "joined", fmt.Sprintf("%d %s joined the group", added, plural(added, "person", "people")), nil)
		s.toGroup(ctx, g.ID, "members", map[string]any{"group_id": g.ID})
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "new_accounts": created, "skipped": skipped})
}

type csvPerson struct{ Name, Email, Phone string }

func parsePeopleCSV(data []byte) ([]csvPerson, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	sep := ','
	if first, _, _ := bytes.Cut(data, []byte("\n")); bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		sep = ';'
	}
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma, cr.FieldsPerRecord, cr.LazyQuotes = sep, -1, true
	recs, err := cr.ReadAll()
	if err != nil || len(recs) < 2 {
		return nil, errors.New("a CSV with a header line (name, email, phone) and one person per line")
	}
	col := map[string]int{"name": -1, "first": -1, "last": -1, "email": -1, "phone": -1}
	for i, h := range recs[0] {
		h = strings.ToLower(strings.TrimSpace(h))
		switch {
		case strings.Contains(h, "mail"):
			col["email"] = i
		case strings.Contains(h, "phone") || strings.Contains(h, "mobile") || strings.Contains(h, "tel"):
			col["phone"] = i
		case strings.Contains(h, "first") || h == "given name":
			col["first"] = i
		case strings.Contains(h, "last") || strings.Contains(h, "surname") || h == "family name":
			col["last"] = i
		case strings.Contains(h, "name"):
			col["name"] = i
		}
	}
	if col["email"] < 0 && col["phone"] < 0 {
		return nil, errors.New("the CSV needs an email or a phone column")
	}
	get := func(rec []string, k string) string {
		if i := col[k]; i >= 0 && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	out := []csvPerson{}
	for _, rec := range recs[1:] {
		p := csvPerson{Name: firstNonEmpty(get(rec, "name"), strings.TrimSpace(get(rec, "first")+" "+get(rec, "last"))),
			Email: strings.ToLower(get(rec, "email")), Phone: get(rec, "phone")}
		if p.Email != "" && !strings.Contains(p.Email, "@") {
			p.Email = ""
		}
		if p.Email == "" && p.Phone == "" {
			continue
		}
		out = append(out, p)
		if len(out) >= 500 {
			break
		}
	}
	return out, nil
}
