package server

import (
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- the places a group plays at ----
//
// Everyone in a group picks from its places when planning a game, and a
// place used once is remembered by itself. Organisers add, rename and
// remove places, and choose the default one (the group's own venue: the
// usual game is played there).

func (s *Server) handleVenues(w http.ResponseWriter, r *http.Request) {
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	vs, err := s.store.Venues(r.Context(), g.ID)
	if err != nil {
		s.fail(w, err, "places")
		return
	}
	writeJSON(w, http.StatusOK, vs)
}

type venueInput struct {
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Lat     *float64 `json:"lat"`
	Lon     *float64 `json:"lon"`
	Ref     string   `json:"ref"`
	Default bool     `json:"default"` // make it the group's own venue
}

func (in *venueInput) clean() string {
	in.Name, in.Address, in.Ref = clip(strings.TrimSpace(in.Name), 120), clip(strings.TrimSpace(in.Address), 300), clip(strings.TrimSpace(in.Ref), 60)
	if in.Name == "" {
		return "give the place a name"
	}
	if (in.Lat == nil) != (in.Lon == nil) || (in.Lat != nil && (*in.Lat < -90 || *in.Lat > 90 || *in.Lon < -180 || *in.Lon > 180)) {
		return "the position is a latitude and a longitude"
	}
	return ""
}

func (s *Server) saveVenue(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	var in venueInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.clean(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	v := &store.Venue{ID: id, GroupID: g.ID, Name: in.Name, Address: in.Address, Lat: in.Lat, Lon: in.Lon, Ref: in.Ref}
	var old *store.Venue
	if id != 0 {
		var err error
		if old, err = s.store.GetVenue(ctx, g.ID, id); err != nil {
			s.fail(w, err, "place")
			return
		}
	} else if vs, _ := s.store.Venues(ctx, g.ID); len(vs) >= store.MaxVenues {
		writeErr(w, http.StatusBadRequest, "a group keeps up to 30 places")
		return
	}
	if err := s.store.SaveVenue(ctx, v); err != nil {
		s.fail(w, err, "save place")
		return
	}
	// the group's own venue follows when it is this one, or when asked
	if in.Default || (old != nil && strings.EqualFold(old.Name, g.VenueName)) {
		g.VenueName, g.VenueAddress, g.VenueLat, g.VenueLon, g.VenueRef = v.Name, v.Address, v.Lat, v.Lon, v.Ref
		if err := s.store.UpdateGroup(ctx, g); err != nil {
			s.fail(w, err, "save group")
			return
		}
	}
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID})
	vs, _ := s.store.Venues(ctx, g.ID)
	writeJSON(w, http.StatusOK, vs)
}

func (s *Server) handleAddVenue(w http.ResponseWriter, r *http.Request) { s.saveVenue(w, r, 0) }
func (s *Server) handleUpdateVenue(w http.ResponseWriter, r *http.Request) {
	s.saveVenue(w, r, idParam(r, "vid"))
}

func (s *Server) handleDeleteVenue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok || !s.needOrganiser(w, g) {
		return
	}
	v, err := s.store.GetVenue(ctx, g.ID, idParam(r, "vid"))
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	if strings.EqualFold(v.Name, g.VenueName) {
		writeErr(w, http.StatusBadRequest, "this is the group's own place: make another one the default first")
		return
	}
	if err := s.store.DeleteVenue(ctx, g.ID, v.ID); err != nil {
		s.fail(w, err, "delete place")
		return
	}
	s.toGroup(ctx, g.ID, "group", map[string]any{"id": g.ID})
	w.WriteHeader(http.StatusNoContent)
}
