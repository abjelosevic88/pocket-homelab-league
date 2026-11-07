package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
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
