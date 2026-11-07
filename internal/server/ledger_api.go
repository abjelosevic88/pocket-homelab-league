package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- who paid, who owes ----
//
// A group can keep accounts: after each game everyone who was in owes their
// share of the court (a guest's share goes to whoever brought them), and a
// tournament's entry fee is owed when it starts. Organisers book payments;
// a player can say "I paid", which counts once an organiser confirms it.
// Everyone in the group sees the balances; the entries behind them are
// the organisers' and each person's own.

type balanceView struct {
	*store.Balance
	Name string `json:"name"`
}

type ledgerView struct {
	*store.LedgerEntry
	Name       string `json:"name"`
	ByName     string `json:"by_name"`
	GameAt     string `json:"game_at,omitempty"`
	Tournament string `json:"tournament,omitempty"`
}

func (s *Server) handleMoney(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.moneyView(ctx, g))
}

func (s *Server) moneyView(ctx context.Context, g *store.Group) map[string]any {
	uid := actorID(ctx)
	names := s.names(ctx, g.ID)
	bal, _ := s.store.Balances(ctx, g.ID)
	list := []balanceView{}
	for id := range names {
		b := bal[id]
		if b == nil {
			b = &store.Balance{UserID: id}
		}
		list = append(list, balanceView{Balance: b, Name: names[id]})
	}
	for id, b := range bal {
		if _, ok := names[id]; !ok && (b.Owes != 0 || b.Pending != 0) {
			list = append(list, balanceView{Balance: b, Name: "Former player"})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Owes != list[j].Owes {
			return list[i].Owes > list[j].Owes
		}
		return list[i].Name < list[j].Name
	})
	who := uid
	if organiser(g) {
		who = 0
	}
	entries, _ := s.store.Ledger(ctx, g.ID, who, 300)
	ev := make([]ledgerView, 0, len(entries))
	gameAt := map[int64]string{}
	tname := map[int64]string{}
	var total, open int64
	for _, e := range entries {
		v := ledgerView{LedgerEntry: e, Name: firstNonEmpty(names[e.UserID], "Former player"), ByName: names[e.CreatedBy]}
		if e.GameID != 0 {
			if _, ok := gameAt[e.GameID]; !ok {
				if gm, err := s.store.GameByID(ctx, e.GameID); err == nil {
					gameAt[e.GameID] = gm.StartsAt
				}
			}
			v.GameAt = gameAt[e.GameID]
		}
		if e.TournamentID != 0 {
			if _, ok := tname[e.TournamentID]; !ok {
				if t, err := s.store.GetTournament(ctx, e.TournamentID); err == nil {
					tname[e.TournamentID] = t.Name
				}
			}
			v.Tournament = tname[e.TournamentID]
		}
		ev = append(ev, v)
	}
	for _, b := range bal {
		total += b.Charged
		if b.Owes > 0 {
			open += b.Owes
		}
	}
	return map[string]any{"currency": firstNonEmpty(g.Currency, "EUR"), "pay_note": g.PayNote, "charge_games": g.ChargeGames, "balances": list,
		"entries": ev, "total_charged": total, "outstanding": open, "can_edit": organiser(g)}
}

func (s *Server) handleAddMoney(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	uid := actorID(ctx)
	var in struct {
		Kind        string `json:"kind"`
		UserID      int64  `json:"user_id"`
		AmountCents int64  `json:"amount_cents"`
		Note        string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !oneOf(in.Kind, []string{"charge", "payment"}) {
		writeErr(w, http.StatusBadRequest, "a charge or a payment")
		return
	}
	if in.AmountCents <= 0 || in.AmountCents > 100_000_00 {
		writeErr(w, http.StatusBadRequest, "how much?")
		return
	}
	if in.UserID == 0 {
		in.UserID = uid
	}
	org := organiser(g)
	if !org && (in.Kind != "payment" || in.UserID != uid) {
		writeErr(w, http.StatusForbidden, "only organisers book charges and others' payments")
		return
	}
	if _, err := s.store.GetMember(ctx, g.ID, in.UserID); err != nil {
		writeErr(w, http.StatusBadRequest, "they aren't in the group")
		return
	}
	e := &store.LedgerEntry{GroupID: g.ID, UserID: in.UserID, Kind: in.Kind, AmountCents: in.AmountCents, Currency: firstNonEmpty(g.Currency, "EUR"),
		Note: clip(strings.TrimSpace(in.Note), 200), CreatedBy: uid}
	if org {
		e.ConfirmedBy = uid
	}
	if _, err := s.store.AddLedger(ctx, e); err != nil {
		s.fail(w, err, "money")
		return
	}
	if !org {
		ms, _ := s.store.Members(ctx, g.ID)
		var orgs []int64
		for _, m := range ms {
			if m.Role == "organiser" {
				orgs = append(orgs, m.UserID)
			}
		}
		s.tell(ctx, g, orgs, uid, note{kind: "short", title: g.Name + ": " + s.nameIn(ctx, g.ID, uid) + " paid " + fmtMoney(e.AmountCents, e.Currency),
			body: "Confirm it under Money when it has arrived.", link: s.groupLink(g.ID) + "?tab=money", button: "Confirm"})
	} else if in.Kind == "charge" && in.UserID != uid {
		s.tell(ctx, g, []int64{in.UserID}, 0, note{kind: "results", title: g.Name + ": " + fmtMoney(e.AmountCents, e.Currency) + " to pay", body: firstNonEmpty(e.Note, "Booked by "+s.nameIn(ctx, g.ID, uid)) + payLine(g), link: s.groupLink(g.ID) + "?tab=money"})
	}
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID, "money": true})
	writeJSON(w, http.StatusCreated, s.moneyView(ctx, g))
}

func (s *Server) myLedger(w http.ResponseWriter, r *http.Request) (*store.LedgerEntry, *store.Group, bool) {
	ctx := r.Context()
	e, err := s.store.GetLedger(ctx, idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	g, err := s.store.GetGroup(ctx, actorID(ctx), e.GroupID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil, false
	}
	return e, g, true
}

func (s *Server) handleConfirmMoney(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e, g, ok := s.myLedger(w, r)
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	if err := s.store.ConfirmLedger(ctx, e.ID, actorID(ctx)); err != nil {
		s.fail(w, err, "confirm")
		return
	}
	if e.UserID != actorID(ctx) {
		s.tell(ctx, g, []int64{e.UserID}, 0, note{kind: "results", title: g.Name + ": your " + fmtMoney(e.AmountCents, e.Currency) + " arrived", body: "Thanks — it's in the books.", link: s.groupLink(g.ID) + "?tab=money"})
	}
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID, "money": true})
	writeJSON(w, http.StatusOK, s.moneyView(ctx, g))
}

func (s *Server) handleDeleteMoney(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e, g, ok := s.myLedger(w, r)
	if !ok {
		return
	}
	own := e.UserID == actorID(ctx) && e.Kind == "payment" && e.ConfirmedAt == nil
	if !organiser(g) && !own {
		writeErr(w, http.StatusForbidden, "only organisers change the books")
		return
	}
	if err := s.store.DeleteLedger(ctx, e.ID); err != nil {
		s.fail(w, err, "delete")
		return
	}
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID, "money": true})
	writeJSON(w, http.StatusOK, s.moneyView(ctx, g))
}

// chargeGame books everyone's share once a game is over (groups keeping accounts).
func (s *Server) chargeGame(ctx context.Context, g *store.Group, gm *store.Game) {
	share := shareCost(gm)
	if !g.ChargeGames || share <= 0 {
		return
	}
	answers, _ := s.store.Answers(ctx, gm.ID)
	owe := map[int64]int64{}
	for _, a := range answers {
		if a.Answer == store.AnswerIn {
			owe[a.UserID] += share
		}
	}
	guests, _ := s.store.Guests(ctx, gm.ID)
	extra := map[int64]int{}
	for _, x := range guests {
		if x.Answer == store.AnswerIn && x.AddedBy != 0 {
			owe[x.AddedBy] += share
			extra[x.AddedBy]++
		}
	}
	day := parseTS(gm.StartsAt).Format("Mon 2 Jan")
	for uid, amt := range owe {
		n := fmt.Sprintf("Game %s: %s of %s", day, fmtMoney(share, gm.Currency), fmtMoney(gm.CostCents, gm.Currency))
		if extra[uid] > 0 {
			n += fmt.Sprintf(" (+%d %s)", extra[uid], plural(extra[uid], "guest", "guests"))
		}
		e := &store.LedgerEntry{GroupID: g.ID, UserID: uid, Kind: "charge", AmountCents: amt, Currency: firstNonEmpty(gm.Currency, g.Currency, "EUR"), GameID: gm.ID, Note: n}
		if fresh, err := s.store.AddLedger(ctx, e); err == nil && fresh {
			bal, _ := s.store.Balances(ctx, g.ID)
			total := amt
			if b := bal[uid]; b != nil {
				total = b.Owes
			}
			if total > 0 {
				s.tell(ctx, g, []int64{uid}, 0, note{kind: "results", title: g.Name + ": " + fmtMoney(amt, e.Currency) + " for " + day,
					body: "Your share of the court. You owe " + fmtMoney(total, e.Currency) + " in all." + payLine(g), link: s.groupLink(g.ID) + "?tab=money"})
			}
		}
	}
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID, "money": true})
}

// chargeTournament books the entry fee for everyone in a tournament when it starts.
func (s *Server) chargeTournament(ctx context.Context, t *store.Tournament, g *store.Group) {
	if t.FeeCents <= 0 {
		return
	}
	es, _ := s.store.Entries(ctx, t.ID)
	for _, e := range es {
		if e.Status == "withdrawn" {
			continue
		}
		for _, m := range e.Members {
			if m.UserID == 0 {
				continue
			}
			l := &store.LedgerEntry{GroupID: g.ID, UserID: m.UserID, Kind: "charge", AmountCents: t.FeeCents, Currency: firstNonEmpty(t.Currency, g.Currency, "EUR"),
				TournamentID: t.ID, Note: "Entry: " + t.Name}
			if e.Paid {
				l.Note += " (paid at the desk)"
			}
			if fresh, _ := s.store.AddLedger(ctx, l); fresh && e.Paid {
				_, _ = s.store.AddLedger(ctx, &store.LedgerEntry{GroupID: g.ID, UserID: m.UserID, Kind: "payment", AmountCents: t.FeeCents, Currency: l.Currency,
					TournamentID: t.ID, Note: "Entry paid: " + t.Name, ConfirmedBy: firstID(t.UserID, g.UserID)})
			}
		}
	}
}

func payLine(g *store.Group) string {
	if g.PayNote == "" {
		return ""
	}
	return "\nHow to pay: " + g.PayNote
}
