package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- notifications on the phone (Web Push) ----
//
// Players are rarely on this box's notification service, so League sends
// to their phone itself: they allow notifications once (League added to
// the home screen on an iPhone) and the browser keeps a subscription that
// League signs with its own key (VAPID, made on first start).

var vapidMu sync.Mutex

type vapidKeys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

func (s *Server) vapid(ctx context.Context) (vapidKeys, error) {
	vapidMu.Lock()
	defer vapidMu.Unlock()
	var k vapidKeys
	if v, _, ok := s.store.CacheGet(ctx, "vapid"); ok && json.Unmarshal([]byte(v), &k) == nil && k.Public != "" {
		return k, nil
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return k, err
	}
	k = vapidKeys{Public: pub, Private: priv}
	b, _ := json.Marshal(k)
	return k, s.store.CachePut(ctx, "vapid", string(b))
}

func (s *Server) handlePushKey(w http.ResponseWriter, r *http.Request) {
	k, err := s.vapid(r.Context())
	if err != nil {
		s.fail(w, err, "push key")
		return
	}
	subs, _ := s.store.PushSubs(r.Context(), actorID(r.Context()))
	writeJSON(w, http.StatusOK, map[string]any{"public_key": k.Public, "phones": subs})
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := decode(r, &in); err != nil || !strings.HasPrefix(in.Endpoint, "https://") || in.Keys.P256dh == "" || in.Keys.Auth == "" {
		writeErr(w, http.StatusBadRequest, "not a push subscription")
		return
	}
	p := &store.PushSub{UserID: actorID(ctx), Endpoint: in.Endpoint, P256dh: in.Keys.P256dh, Auth: in.Keys.Auth, Agent: clip(r.UserAgent(), 200)}
	if err := s.store.SavePushSub(ctx, p); err != nil {
		s.fail(w, err, "subscribe")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Endpoint string `json:"endpoint"`
	}
	_ = decode(r, &in)
	_ = s.store.DeletePushSub(r.Context(), actorID(r.Context()), in.Endpoint)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePushTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n := s.webPush(ctx, actorID(ctx), note{title: "Pocket League", body: "Notifications on this phone work. You'll hear about new games, reminders, results and the chat.", link: s.publicBase(r) + "/"})
	if n == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "no phone took it — allow notifications again"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Sent to " + itoa(int64(n)) + " " + plural(n, "phone", "phones")})
}

// webPush sends a note to all of a person's phones; it returns how many took it.
func (s *Server) webPush(ctx context.Context, uid int64, n note) int {
	subs, err := s.store.PushSubs(ctx, uid)
	if err != nil || len(subs) == 0 {
		return 0
	}
	k, err := s.vapid(ctx)
	if err != nil {
		return 0
	}
	payload, _ := json.Marshal(map[string]any{"title": n.title, "body": n.body, "url": n.link, "tag": n.kind})
	subscriber := "mailto:league@pocket.local"
	if u, err := s.store.GetUser(ctx, s.store.OwnerID(ctx)); err == nil && u.Email != "" {
		subscriber = "mailto:" + u.Email
	}
	sent := 0
	for _, p := range subs {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		res, err := webpush.SendNotificationWithContext(cctx, payload, &webpush.Subscription{Endpoint: p.Endpoint, Keys: webpush.Keys{P256dh: p.P256dh, Auth: p.Auth}},
			&webpush.Options{Subscriber: subscriber, VAPIDPublicKey: k.Public, VAPIDPrivateKey: k.Private, TTL: 3600 * 12, Urgency: webpush.UrgencyNormal})
		cancel()
		if err != nil {
			s.store.PushResult(ctx, p.ID, false, false)
			continue
		}
		res.Body.Close()
		switch {
		case res.StatusCode == http.StatusGone || res.StatusCode == http.StatusNotFound:
			s.store.PushResult(ctx, p.ID, false, true)
		case res.StatusCode >= 200 && res.StatusCode < 300:
			s.store.PushResult(ctx, p.ID, true, false)
			sent++
		default:
			s.store.PushResult(ctx, p.ID, false, false)
		}
	}
	return sent
}
