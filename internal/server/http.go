package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Get("/pocket-ask.js", s.core.AskWidget("league"))
	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	// a group's games as a calendar to subscribe to (the token is the key)
	r.Get("/feed/{token}", s.handleFeed)

	r.Route("/api/v1", func(r chi.Router) {
		// the live stream stays open; it has no timeout
		r.With(s.requireAuth).Get("/stream", s.handleStream)

		// imports, AI and the other apps can take longer than a request may
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/import", s.handleImport)
			r.Post("/groups/{id}/import-players", s.handleImportPlayers)
			r.Post("/groups/{id}/catchup", s.handleCatchUp)
			r.Post("/matches/{id}/report", s.handleMatchReport)
			r.Post("/games/{id}/make", s.handleMake)
			r.Post("/groups/{id}/invite-all", s.handleInviteAll)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(120 * time.Second))
			r.Post("/auth/login", s.handleLogin)
			r.Post("/auth/setup", s.handleSetup)
			r.Get("/auth/status", s.handleAuthStatus)
			r.Get("/auth/oidc/start", s.handleOIDCStart)
			r.Get("/auth/oidc/callback", s.handleOIDCCallback)
			// players: a sign-in link, and a new one by email
			r.Get("/auth/link/{token}", s.handlePeekLink)
			r.Post("/auth/link", s.handleUseLink)
			r.Post("/auth/link/request", s.handleRequestLink)
			// a tournament's read-only page, for anyone with its link
			r.Get("/public/t/{token}", s.handlePublicTournament)

			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Post("/auth/logout", s.handleLogout)
				r.Get("/auth/me", s.handleMe)
				r.Put("/auth/me", s.handleUpdateMe)

				r.Get("/sports", s.handleSports)
				r.Post("/groups/{id}/photos", s.handleUploadPhoto)
				r.Get("/photos/{id}", s.handlePhoto)
				r.Get("/push/key", s.handlePushKey)
				r.Post("/push/subscribe", s.handlePushSubscribe)
				r.Post("/push/unsubscribe", s.handlePushUnsubscribe)
				r.Post("/push/test", s.handlePushTest)

				// groups and their people
				r.Get("/groups", s.handleListGroups)
				r.Post("/groups", s.handleCreateGroup)
				r.Get("/groups/{id}", s.handleGetGroup)
				r.Put("/groups/{id}", s.handleUpdateGroup)
				r.Delete("/groups/{id}", s.handleDeleteGroup)
				r.Put("/groups/{id}/me", s.handleMyGroupSettings)
				r.Post("/groups/{id}/leave", s.handleLeaveGroup)
				r.Get("/groups/{id}/members", s.handleMembers)
				r.Post("/groups/{id}/members", s.handleAddMember)
				r.Put("/groups/{id}/members/{uid}", s.handleUpdateMember)
				r.Delete("/groups/{id}/members/{uid}", s.handleRemoveMember)
				r.Post("/groups/{id}/members/{uid}/link", s.handleMemberLink)
				r.Post("/groups/{id}/venue/save", s.handleSaveVenue)
				r.Get("/groups/{id}/venues", s.handleVenues)
				r.Post("/groups/{id}/venues", s.handleAddVenue)
				r.Put("/groups/{id}/venues/{vid}", s.handleUpdateVenue)
				r.Delete("/groups/{id}/venues/{vid}", s.handleDeleteVenue)

				// games
				r.Get("/games", s.handleListGames)
				r.Get("/upcoming", s.handleUpcoming)
				r.Post("/groups/{id}/games", s.handleCreateGame)
				r.Get("/games/{id}", s.handleGetGame)
				r.Put("/games/{id}", s.handleUpdateGame)
				r.Delete("/games/{id}", s.handleDeleteGame)
				r.Post("/games/{id}/cancel", s.handleCancelGame)
				r.Post("/games/{id}/answer", s.handleAnswer)
				r.Post("/games/{id}/votes", s.handleVote)
				r.Post("/games/{id}/pick", s.handlePick)
				r.Post("/games/{id}/teams", s.handleTeams)
				r.Post("/games/{id}/mvp", s.handleMVP)
				r.Get("/games/{id}/game.ics", s.handleGameICS)
				r.Get("/games/{id}/weather", s.handleGameWeather)
				r.Get("/games/{id}/busy", s.handleGameBusy)
				r.Delete("/made/{id}", s.handleDeleteMade)

				// tournaments
				r.Get("/tournaments", s.handleListTournaments)
				r.Get("/tournaments/formats", s.handleTournamentFormats)
				r.Get("/groups/{id}/tournaments", s.handleListTournaments)
				r.Post("/groups/{id}/tournaments", s.handleCreateTournament)
				r.Get("/tournaments/{id}", s.handleGetTournament)
				r.Put("/tournaments/{id}", s.handleUpdateTournament)
				r.Delete("/tournaments/{id}", s.handleDeleteTournament)
				r.Post("/tournaments/{id}/join", s.handleJoinTournament)
				r.Post("/tournaments/{id}/leave", s.handleLeaveTournament)
				r.Post("/tournaments/{id}/entries", s.handleAddEntry)
				r.Put("/tournaments/{id}/entries/{eid}", s.handleUpdateEntry)
				r.Delete("/tournaments/{id}/entries/{eid}", s.handleDeleteEntry)
				r.Post("/tournaments/{id}/from-game", s.handleEntriesFromGame)
				r.Post("/tournaments/{id}/teams", s.handleFairTeams)
				r.Post("/tournaments/{id}/start", s.handleStartTournament)
				r.Post("/tournaments/{id}/redraw", s.handleRedraw)
				r.Post("/tournaments/{id}/regenerate", s.handleRegenerate)
				r.Post("/tournaments/{id}/finish", s.handleFinishTournament)
				r.Post("/tournaments/{id}/next-round", s.handleNextSeason)
				r.Put("/tournaments/{id}/scorecards", s.handleScorecard)
				r.Post("/tournaments/{id}/challenges", s.handleChallenge)
				r.Post("/tournaments/{id}/challenges/{cid}", s.handleChallengeAnswer)
				r.Post("/tmatches/{id}/result", s.handleTMatchResult)
				r.Post("/tmatches/{id}/confirm", s.handleTMatchConfirm)
				r.Post("/tmatches/{id}/reopen", s.handleTMatchReopen)
				r.Post("/tmatches/{id}/call", s.handleTMatchCall)

				// results and the table
				r.Get("/matches", s.handleListMatches)
				r.Post("/groups/{id}/matches", s.handleCreateMatch)
				r.Put("/matches/{id}", s.handleUpdateMatch)
				r.Delete("/matches/{id}", s.handleDeleteMatch)
				r.Post("/matches/{id}/confirm", s.handleConfirmMatch)
				r.Get("/groups/{id}/table", s.handleTable)
				r.Get("/groups/{id}/players/{uid}", s.handlePlayer)

				// the chat
				r.Get("/groups/{id}/messages", s.handleListMessages)
				r.Post("/groups/{id}/messages", s.handleSendMessage)
				r.Post("/groups/{id}/read", s.handleRead)
				r.Post("/groups/{id}/typing", s.handleTyping)
				r.Put("/messages/{id}", s.handleEditMessage)
				r.Delete("/messages/{id}", s.handleDeleteMessage)
				r.Post("/messages/{id}/react", s.handleReact)

				// the other Pocket apps
				r.Get("/apps", s.handleApps)
				r.Get("/apps/options/{what}", s.handleAppOptions)
				r.Get("/apps/contacts", s.handleContactSearch)
				r.Get("/apps/places", s.handlePlaceSearch)

				// overviews, and for the rest of the box: Pocket Day, Search, AI
				r.Get("/today", s.handleToday)
				r.Get("/summary", s.handleSummary)
				r.Get("/day/{date}", s.handleDay)
				r.Get("/search", s.handleSearch)
				r.Get("/lookup", s.handleLookup)

				// AI
				r.Get("/ai/status", s.handleAIStatus)
				r.Get("/ai/models", s.handleAIModels)

				r.Post("/integrations/test", s.handleIntegrationTest)
				r.Post("/notify/test", s.handleNotifyTest)
				r.Post("/email/test", s.handleEmailTest)

				r.Get("/users", s.handleListUsers)
				r.Post("/users", s.handleCreateUser)
				r.Put("/users/{id}", s.handleUpdateUser)
				r.Delete("/users/{id}", s.handleDeleteUser)

				r.Get("/settings", s.handleGetSettings)
				r.Put("/settings", s.handlePutSettings)
				r.Get("/settings/system", s.handleSystemInfo)

				r.Get("/tokens", s.handleListTokens)
				r.Post("/tokens", s.handleCreateToken)
				r.Delete("/tokens/{id}", s.handleDeleteToken)

				r.Get("/backup", s.handleBackup)
				r.Get("/export.json", s.handleExportJSON)
				r.Get("/export.csv", s.handleExportCSV)
			})
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict), errors.Is(err, core.ErrOperationConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(nil, r.Body, s.cfg.MaxBodyBytes))
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_league_info Build info.\n# TYPE pocket_league_info gauge\npocket_league_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_league_uptime_seconds Process uptime.\n# TYPE pocket_league_uptime_seconds gauge\npocket_league_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_league_http_requests_total HTTP requests served.\n# TYPE pocket_league_http_requests_total counter\npocket_league_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_league_http_errors_total HTTP 5xx responses.\n# TYPE pocket_league_http_errors_total counter\npocket_league_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_league_rows Rows per table.\n# TYPE pocket_league_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_league_rows{table=%q} %d\n", k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if path == "sw.js" {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Service-Worker-Allowed", "/")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}
