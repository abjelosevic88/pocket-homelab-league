package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"
)

// uploads a file as the person, returns status and body
func (a *app) upload(auth, path string, data []byte) (int, []byte) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "court.png")
	_, _ = fw.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", a.url+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (a *app) raw(auth, path string) (int, []byte) {
	req, _ := http.NewRequest("GET", a.url+path, nil)
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func testPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestPhotosInTheChat(t *testing.T) {
	a := newApp(t, nil)
	gid, _ := tennis(a)
	g := "/api/v1/groups/" + gid
	// Bob is not in the group: no uploads
	if c, _ := a.upload(a.bob, g+"/photos", testPNG(40, 30)); c != 404 {
		t.Fatalf("outsider upload: %d", c)
	}
	if c, _ := a.upload(a.alice, g+"/photos", []byte("not a photo")); c != 400 {
		t.Fatalf("junk upload: %d", c)
	}
	c, b := a.upload(a.alice, g+"/photos", testPNG(2400, 1200))
	if c != 201 {
		t.Fatalf("upload: %d %s", c, b)
	}
	p := decodeMap(t, b)
	if num(p, "w") != 1920 || num(p, "h") != 960 {
		t.Fatalf("not shrunk: %v", p)
	}
	pid := id(p)
	// not sent yet: only Alice sees it
	a.ok(a.alice, "POST", g+"/members", map[string]any{"name": "Bob", "email": "bob@example.com"})
	if c, _ := a.raw(a.bob, "/api/v1/photos/"+pid); c != 404 {
		t.Fatalf("loose photo shown to Bob: %d", c)
	}
	// Bob can't send Alice's photo as his
	a.expect(400, a.bob, "POST", g+"/messages", map[string]any{"photos": []int64{int64(num(p, "id"))}})
	m := a.ok(a.alice, "POST", g+"/messages", map[string]any{"body": "", "photos": []int64{int64(num(p, "id"))}})
	if ph := m["data"].(map[string]any)["photos"].([]any); len(ph) != 1 {
		t.Fatalf("message photos: %v", m)
	}
	if c, img := a.raw(a.bob, "/api/v1/photos/"+pid+"?size=small"); c != 200 || !bytes.HasPrefix(img, []byte{0xff, 0xd8}) {
		t.Fatalf("thumb: %d", c)
	}
	// sent once only
	a.expect(400, a.alice, "POST", g+"/messages", map[string]any{"photos": []int64{int64(num(p, "id"))}})
	// deleting the message deletes the photo
	a.expect(204, a.alice, "DELETE", "/api/v1/messages/"+id(m), nil)
	if c, _ := a.raw(a.alice, "/api/v1/photos/"+pid); c != 404 {
		t.Fatalf("photo after delete: %d", c)
	}
}

func TestPushSubscriptions(t *testing.T) {
	a := newApp(t, nil)
	k := a.ok(a.alice, "GET", "/api/v1/push/key", nil)
	if len(k["public_key"].(string)) < 80 {
		t.Fatalf("vapid key: %v", k)
	}
	if k2 := a.ok(a.bob, "GET", "/api/v1/push/key", nil); k2["public_key"] != k["public_key"] {
		t.Fatal("the key changes")
	}
	a.expect(400, a.alice, "POST", "/api/v1/push/subscribe", map[string]any{"endpoint": "http://x"})
	a.ok(a.alice, "POST", "/api/v1/push/subscribe", map[string]any{"endpoint": "https://push.example/abc", "keys": map[string]any{"p256dh": "BNc", "auth": "x"}})
	if l := a.ok(a.alice, "GET", "/api/v1/push/key", nil)["phones"].([]any); len(l) != 1 {
		t.Fatalf("phones: %v", l)
	}
	if l := a.ok(a.bob, "GET", "/api/v1/push/key", nil)["phones"].([]any); len(l) != 0 {
		t.Fatalf("Bob sees Alice's phones: %v", l)
	}
	a.ok(a.alice, "POST", "/api/v1/push/unsubscribe", map[string]any{"endpoint": "https://push.example/abc"})
	if l := a.ok(a.alice, "GET", "/api/v1/push/key", nil)["phones"].([]any); len(l) != 0 {
		t.Fatalf("phone not removed: %v", l)
	}
}

func decodeMap(t *testing.T, b []byte) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json: %v %s", err, b)
	}
	return m
}

func soon(h int) string {
	return time.Now().Add(time.Duration(h) * time.Hour).Format("2006-01-02T15:04")
}

func TestGuestsTakeAPlace(t *testing.T) {
	a := newApp(t, nil)
	gid, _ := tennis(a)
	g := "/api/v1/groups/" + gid
	a.ok(a.alice, "POST", g+"/members", map[string]any{"name": "Bob", "email": "bob@example.com"})
	gm := a.ok(a.alice, "POST", g+"/games", map[string]any{"starts_at": soon(72), "im_in": true, "max_players": 2})
	game := "/api/v1/games/" + id(gm["game"].(map[string]any))
	a.ok(a.bob, "POST", game+"/answer", map[string]any{"answer": "in"})
	d := a.ok(a.alice, "POST", game+"/guests", map[string]any{"name": "Mia"})
	gs := d["guests"].([]any)
	if len(gs) != 1 || gs[0].(map[string]any)["answer"] != "waiting" {
		t.Fatalf("full game: guest should wait: %v", gs)
	}
	// Bob can't come: the guest moves up
	d = a.ok(a.bob, "POST", game+"/answer", map[string]any{"answer": "out"})
	if d["guests"].([]any)[0].(map[string]any)["answer"] != "in" || num(d["game"].(map[string]any), "in") != 2 {
		t.Fatalf("guest moved up: %v", d["guests"])
	}
	// Bob can't take Alice's guest off
	gidS := id(gs[0].(map[string]any))
	a.expect(403, a.bob, "DELETE", game+"/guests/"+gidS, nil)
	a.ok(a.alice, "DELETE", game+"/guests/"+gidS, nil)
}

func TestReliabilityAwayAndChores(t *testing.T) {
	a := newApp(t, nil)
	gid, _ := tennis(a)
	g := "/api/v1/groups/" + gid
	a.ok(a.alice, "POST", g+"/members", map[string]any{"name": "Bob", "email": "bob@example.com"})
	// a late drop-out
	gm := a.ok(a.alice, "POST", g+"/games", map[string]any{"starts_at": soon(5), "im_in": true})
	game := "/api/v1/games/" + id(gm["game"].(map[string]any))
	a.ok(a.bob, "POST", game+"/answer", map[string]any{"answer": "in"})
	a.ok(a.bob, "POST", game+"/answer", map[string]any{"answer": "out"})
	_, _, l := a.call(a.alice, "GET", g+"/reliability", nil)
	var bob map[string]any
	for _, x := range l {
		if int64(num(x.(map[string]any), "user_id")) == a.ids["Bob"] {
			bob = x.(map[string]any)
		}
	}
	if bob == nil || num(bob, "late_outs") != 1 || len(l) != 2 {
		t.Fatalf("reliability: %v", l)
	}
	if _, _, mine := a.call(a.bob, "GET", g+"/reliability", nil); len(mine) != 1 {
		t.Fatalf("Bob sees others' reliability: %v", mine)
	}
	// away: a game on those days is "out" for Bob, and he isn't asked
	day := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	a.ok(a.bob, "POST", "/api/v1/away", map[string]any{"from": day, "to": day, "note": "Holidays"})
	if l := a.list(a.alice, "/api/v1/away"); len(l) != 0 {
		t.Fatal("away days are per person")
	}
	gm2 := a.ok(a.alice, "POST", g+"/games", map[string]any{"starts_at": day + "T19:00", "im_in": true})
	for _, p := range gm2["people"].([]any) {
		pm := p.(map[string]any)
		if int64(num(pm, "user_id")) == a.ids["Bob"] && (pm["answer"] != "out" || pm["note"] != "Holidays") {
			t.Fatalf("away answer: %v", pm)
		}
	}
	// chores: given to someone in, handed over only by them or an organiser
	a.ok(a.alice, "PUT", g+"/duties", map[string]any{"names": []string{"Balls", "Book the court", "balls"}})
	ds := a.list(a.alice, g+"/duties")
	if len(ds) != 2 {
		t.Fatalf("duties: %v", ds)
	}
	gm3, _ := a.st.GameByID(context.Background(), int64(num(gm2["game"].(map[string]any), "id")))
	grp, _ := a.st.GroupByID(context.Background(), n(gid))
	a.s.assignDuties(context.Background(), grp, gm3, false)
	d := a.ok(a.alice, "GET", "/api/v1/games/"+itoa(gm3.ID), nil)
	for _, x := range d["duties"].([]any) {
		if int64(num(x.(map[string]any), "user_id")) != a.ids["Alice"] {
			t.Fatalf("chore not given to the one who is in: %v", d["duties"])
		}
	}
	duty := itoa(int64(num(ds[0].(map[string]any), "id")))
	a.expect(403, a.bob, "POST", "/api/v1/games/"+itoa(gm3.ID)+"/duties/"+duty, map[string]any{"done": true})
	a.ok(a.alice, "POST", "/api/v1/games/"+itoa(gm3.ID)+"/duties/"+duty, map[string]any{"user_id": a.ids["Bob"]})
	a.ok(a.bob, "POST", "/api/v1/games/"+itoa(gm3.ID)+"/duties/"+duty, map[string]any{"done": true})
}

func TestFindingASub(t *testing.T) {
	a := newApp(t, nil)
	gid, _ := tennis(a)
	// Bob plays in Alice's other tennis group, not this one
	other := a.ok(a.alice, "POST", "/api/v1/groups", map[string]any{"name": "Weekend Tennis", "sport": "tennis"})
	a.ok(a.alice, "POST", "/api/v1/groups/"+id(other)+"/members", map[string]any{"name": "Bob", "email": "bob@example.com"})
	gm := a.ok(a.alice, "POST", "/api/v1/groups/"+gid+"/games", map[string]any{"starts_at": soon(30), "im_in": true, "min_players": 2, "max_players": 2})
	game := "/api/v1/games/" + id(gm["game"].(map[string]any))
	dry := a.ok(a.alice, "POST", game+"/sub", map[string]any{"others": true, "dry_run": true})
	if num(dry, "others") != 1 {
		t.Fatalf("dry run: %v", dry)
	}
	a.expect(404, a.bob, "POST", game+"/sub", nil)
	r := a.ok(a.alice, "POST", game+"/sub", map[string]any{"others": true})
	if num(r, "asked") != 1 {
		t.Fatalf("asked: %v", r)
	}
	subs := a.list(a.bob, "/api/v1/subs")
	if len(subs) != 1 {
		t.Fatalf("Bob's offers: %v", subs)
	}
	take := a.ok(a.bob, "POST", "/api/v1/subs/"+id(subs[0].(map[string]any)), nil)
	if take["answer"] != "in" {
		t.Fatalf("take: %v", take)
	}
	// Bob is in the group now and in the game; the offer is gone
	d := a.ok(a.bob, "GET", game, nil)
	if num(d["game"].(map[string]any), "in") != 2 {
		t.Fatalf("not in: %v", d["game"])
	}
	if l := a.list(a.bob, "/api/v1/subs"); len(l) != 0 {
		t.Fatalf("offer still open: %v", l)
	}
}

func TestWhoOwesWhat(t *testing.T) {
	a := newApp(t, nil)
	gid, _ := tennis(a) // the court costs 40 BAM
	g := "/api/v1/groups/" + gid
	a.ok(a.alice, "PUT", g, map[string]any{"charge_games": true, "pay_note": "revolut.me/alice"})
	a.ok(a.alice, "POST", g+"/members", map[string]any{"name": "Bob", "email": "bob@example.com"})
	gm := a.ok(a.alice, "POST", g+"/games", map[string]any{"starts_at": soon(-3), "im_in": true, "duration_min": 60})
	gameID := int64(num(gm["game"].(map[string]any), "id"))
	_ = a.st.SetAnswer(context.Background(), gameID, a.ids["Bob"], "in", "")
	a.ok(a.alice, "POST", "/api/v1/games/"+itoa(gameID)+"/guests", map[string]any{"name": "Mia"})
	grp, _ := a.st.GroupByID(context.Background(), n(gid))
	game, _ := a.st.GameByID(context.Background(), gameID)
	a.s.chargeGame(context.Background(), grp, game)
	a.s.chargeGame(context.Background(), grp, game) // once only
	m := a.ok(a.bob, "GET", g+"/money", nil)
	owes := map[string]float64{}
	for _, b := range m["balances"].([]any) {
		bm := b.(map[string]any)
		owes[bm["name"].(string)] = num(bm, "owes")
	}
	// 40 between three (Alice, Bob, Mia): 13.34 each, Mia's on Alice
	if owes["Bob"] != 1334 || owes["Alice"] != 2668 {
		t.Fatalf("balances: %v", owes)
	}
	if len(m["entries"].([]any)) != 1 || m["pay_note"] != "revolut.me/alice" {
		t.Fatalf("Bob sees only his own entries: %v", m["entries"])
	}
	// Bob says he paid: pending until Alice confirms
	a.expect(403, a.bob, "POST", g+"/money", map[string]any{"kind": "charge", "user_id": a.ids["Alice"], "amount_cents": 100})
	r := a.ok(a.bob, "POST", g+"/money", map[string]any{"kind": "payment", "amount_cents": 1334, "note": "cash"})
	var pid string
	for _, e := range r["entries"].([]any) {
		em := e.(map[string]any)
		if em["kind"] == "payment" {
			pid = id(em)
		}
	}
	a.expect(403, a.bob, "POST", "/api/v1/money/"+pid+"/confirm", nil)
	r = a.ok(a.alice, "POST", "/api/v1/money/"+pid+"/confirm", nil)
	for _, b := range r["balances"].([]any) {
		bm := b.(map[string]any)
		if bm["name"] == "Bob" && num(bm, "owes") != 0 {
			t.Fatalf("after paying: %v", bm)
		}
	}
	a.expect(404, a.bob, "DELETE", "/api/v1/money/999", nil)
}
