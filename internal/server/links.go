package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), League as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "league:read", Name: "See your games and results", Description: "Read your groups, the coming games (when, where, who is in), the results, the tables and seasons, the tournaments, live scores, what you owe, your away days and the chat headlines — for your journal, your day, search, dashboards and Pocket AI."},
	{Scope: "league:write", Name: "Say whether you are coming", Description: "Answer for you whether you are in, maybe or out for a game (\"I'm in for Tuesday\"). Nothing else: no messages, no results."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "league.json", Kind: core.KindPocket, Title: "Your groups as you see them: the people, games with answers, results, places and the chat"},
		{Path: "/api/v1/export.csv", File: "results.csv", Kind: core.KindCSV, Title: "Every result of your groups as a table"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, Form: "file", MaxBytes: 64 << 20, Title: "Brings the groups, games, results and chat back with you as organiser; what is already here is left alone"},
	},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(sports|groups|games|matches|today|summary|search|lookup|upcoming|day/\d{4}-\d{2}-\d{2})$`), "league:read"},
	{"GET", regexp.MustCompile(`^/api/v1/groups/\d+(/table|/members|/players/\d+)?$`), "league:read"},
	{"GET", regexp.MustCompile(`^/api/v1/games/\d+$`), "league:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(tournaments|groups/\d+/tournaments|tournaments/\d+)$`), "league:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(subs|away|groups/\d+/(seasons|live|money|duties))$`), "league:read"},
	{"POST", regexp.MustCompile(`^/api/v1/games/\d+/answer$`), "league:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "league" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
