package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "image/gif"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/abjelosevic88/pocket-homelab-league/internal/store"
)

// ---- photos in the chat ----
//
// A photo is sent in two steps: uploaded (the phone already shrinks it),
// then sent with the message. League decodes it again and saves a clean
// JPEG — which also drops what the camera wrote into it (where it was
// taken) — plus a small one for the chat. The files live next to the
// database, under files/<group>/, and only the group's people can open them.

const (
	maxPhotoBytes = 15 << 20
	photoSide     = 1920
	thumbSide     = 480
	maxPhotos     = 6
)

func (s *Server) photoDir(gid int64) string {
	return filepath.Join(s.cfg.DataDir, "files", itoa(gid))
}

func (s *Server) handleUploadPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, ok := s.myGroup(w, r, "id")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+1<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "pick a photo (up to 15 MB)")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxPhotoBytes+1))
	if err != nil || len(raw) > maxPhotoBytes {
		writeErr(w, http.StatusBadRequest, "a photo is up to 15 MB")
		return
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width*cfg.Height > 60_000_000 {
		writeErr(w, http.StatusBadRequest, "that isn't a photo League can show (JPEG, PNG, WebP or GIF)")
		return
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "that photo can't be read")
		return
	}
	big := fit(img, photoSide)
	small := fit(img, thumbSide)
	dir := s.photoDir(g.ID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.fail(w, err, "photo")
		return
	}
	key := randHex(12)
	path := filepath.Join(itoa(g.ID), key+".jpg")
	thumb := filepath.Join(itoa(g.ID), key+"_t.jpg")
	n, err := writeJPEG(filepath.Join(s.cfg.DataDir, "files", path), big, 84)
	if err == nil {
		_, err = writeJPEG(filepath.Join(s.cfg.DataDir, "files", thumb), small, 78)
	}
	if err != nil {
		s.fail(w, err, "photo")
		return
	}
	b := big.Bounds()
	a := &store.Attachment{GroupID: g.ID, UserID: actorID(ctx), Name: clip(filepath.Base(hdr.Filename), 120), Mime: "image/jpeg", Bytes: n, Width: b.Dx(), Height: b.Dy(), Path: path, Thumb: thumb}
	if err := s.store.AddAttachment(ctx, a); err != nil {
		s.removeFiles([]string{path, thumb})
		s.fail(w, err, "photo")
		return
	}
	writeJSON(w, http.StatusCreated, photoJSON(a))
}

// photoJSON is how a photo is shown in a message's data.
func photoJSON(a *store.Attachment) map[string]any {
	return map[string]any{"id": a.ID, "w": a.Width, "h": a.Height, "url": "/api/v1/photos/" + itoa(a.ID), "thumb": "/api/v1/photos/" + itoa(a.ID) + "?size=small"}
}

func (s *Server) handlePhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, err := s.store.GetAttachment(ctx, idParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if _, err := s.store.GetGroup(ctx, actorID(ctx), a.GroupID); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if a.MessageID == 0 && a.UserID != actorID(ctx) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	p := a.Path
	if r.URL.Query().Get("size") == "small" && a.Thumb != "" {
		p = a.Thumb
	}
	full := filepath.Join(s.cfg.DataDir, "files", filepath.Clean("/"+p))
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="league-photo-`+itoa(a.ID)+`.jpg"`)
	}
	http.ServeFile(w, r, full)
}

// attachPhotos puts uploaded photos into a new message.
func (s *Server) attachPhotos(ctx context.Context, m *store.Message, ids []int64) []map[string]any {
	if len(ids) == 0 {
		return nil
	}
	as, err := s.store.ClaimAttachments(ctx, m.GroupID, m.UserID, m.ID, ids)
	if err != nil || len(as) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(as))
	for _, a := range as {
		out = append(out, photoJSON(a))
	}
	return out
}

// dropPhotos deletes a message's photo files (messageID 0: loose uploads a day old).
func (s *Server) dropPhotos(ctx context.Context, messageID int64) {
	files, err := s.store.DropAttachments(ctx, messageID)
	if err == nil {
		s.removeFiles(files)
	}
}

func (s *Server) removeFiles(files []string) {
	root := filepath.Join(s.cfg.DataDir, "files")
	for _, f := range files {
		if f == "" || strings.Contains(f, "..") {
			continue
		}
		_ = os.Remove(filepath.Join(root, f))
	}
}

func fit(img image.Image, side int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= side && h <= side {
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)
		return dst
	}
	if w >= h {
		h, w = h*side/w, side
	} else {
		w, h = w*side/h, side
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

func writeJPEG(path string, img image.Image, q int) (int64, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		return 0, err
	}
	return int64(buf.Len()), os.WriteFile(path, buf.Bytes(), 0o640)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
