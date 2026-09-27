package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SeriousBug/webp-go-pure/std"
	"github.com/gen2brain/heic"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/crypto/argon2"
)

type server struct {
	db                  *pgxpool.Pool
	registrationEnabled bool
	secureCookies       bool
	minio               *minio.Client
	mediaBucket         string
	maxUploadBytes      int64
}
type ctxKey string

const userKey ctxKey = "user"

type user struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

func NewServer(db *pgxpool.Pool, registrationEnabled bool) http.Handler {
	s := &server{db: db, registrationEnabled: registrationEnabled, secureCookies: os.Getenv("COOKIE_SECURE") != "false", mediaBucket: env("MINIO_BUCKET", "storere"), maxUploadBytes: envInt64("MAX_UPLOAD_BYTES", 1<<30)}
	if endpoint := os.Getenv("MINIO_ENDPOINT"); endpoint != "" {
		client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY"), ""), Secure: os.Getenv("MINIO_USE_SSL") == "true"})
		if err != nil {
			slog.Error("minio configuration", "error", err)
		} else {
			s.minio = client
		}
	}
	return s.router()
}

func (s *server) router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/v1/auth/register", s.register)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.Handle("GET /api/v1/auth/me", s.auth(http.HandlerFunc(s.me)))
	mux.Handle("GET /api/v1/spaces", s.auth(http.HandlerFunc(s.listSpaces)))
	mux.Handle("POST /api/v1/spaces", s.auth(http.HandlerFunc(s.createSpace)))
	mux.Handle("GET /api/v1/spaces/{spaceID}/locations", s.auth(http.HandlerFunc(s.locations)))
	mux.Handle("POST /api/v1/spaces/{spaceID}/locations", s.auth(http.HandlerFunc(s.createLocation)))
	mux.Handle("GET /api/v1/spaces/{spaceID}/boxes", s.auth(http.HandlerFunc(s.boxes)))
	mux.Handle("POST /api/v1/spaces/{spaceID}/boxes", s.auth(http.HandlerFunc(s.createBox)))
	mux.Handle("PATCH /api/v1/boxes/{boxID}/move", s.auth(http.HandlerFunc(s.moveBox)))
	mux.Handle("GET /api/v1/spaces/{spaceID}/items", s.auth(http.HandlerFunc(s.items)))
	mux.Handle("POST /api/v1/spaces/{spaceID}/items", s.auth(http.HandlerFunc(s.createItem)))
	mux.Handle("PATCH /api/v1/items/{itemID}", s.auth(http.HandlerFunc(s.patchItem)))
	mux.Handle("POST /api/v1/items/{itemID}/media", s.auth(http.HandlerFunc(s.addItemMedia)))
	mux.Handle("PATCH /api/v1/items/{itemID}/media", s.auth(http.HandlerFunc(s.reorderItemMedia)))
	mux.Handle("DELETE /api/v1/items/{itemID}/media/{mediaID}", s.auth(http.HandlerFunc(s.removeItemMedia)))
	mux.Handle("DELETE /api/v1/items/{itemID}", s.auth(http.HandlerFunc(s.deleteItem)))
	mux.Handle("PATCH /api/v1/boxes/{boxID}", s.auth(http.HandlerFunc(s.patchBox)))
	mux.Handle("DELETE /api/v1/boxes/{boxID}", s.auth(http.HandlerFunc(s.deleteBox)))
	mux.Handle("PATCH /api/v1/locations/{locationID}", s.auth(http.HandlerFunc(s.patchLocation)))
	mux.Handle("DELETE /api/v1/locations/{locationID}", s.auth(http.HandlerFunc(s.deleteLocation)))
	mux.Handle("GET /api/v1/spaces/{spaceID}/search", s.auth(http.HandlerFunc(s.search)))
	mux.Handle("GET /api/v1/{entity}/{id}/timeline", s.auth(http.HandlerFunc(s.timeline)))
	mux.Handle("POST /api/v1/media", s.auth(http.HandlerFunc(s.uploadMedia)))
	mux.Handle("GET /api/v1/media/{mediaID}", s.auth(http.HandlerFunc(s.getMedia)))
	return security(mux)
}
func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	var enabled bool
	if err := db.QueryRow(context.Background(), "select registration_enabled from app_settings where singleton=true").Scan(&enabled); err != nil {
		slog.Error("settings", "error", err)
		os.Exit(1)
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	slog.Info("listening", "addr", addr)
	if err := http.ListenAndServe(addr, NewServer(db, enabled)); err != nil {
		slog.Error("server", "error", err)
	}
}
func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (s *server) uploadMedia(w http.ResponseWriter, r *http.Request) {
	if s.minio == nil {
		fail(w, http.StatusServiceUnavailable, "media_unavailable", "Media storage is unavailable")
		return
	}
	const multipartOverhead = int64(1 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes+multipartOverhead)
	if err := r.ParseMultipartForm(s.maxUploadBytes + multipartOverhead); err != nil {
		fail(w, http.StatusRequestEntityTooLarge, "upload_too_large", "Photo is too large")
		return
	}
	spaceID := r.FormValue("spaceId")
	if !s.can(r, spaceID, "editor") {
		fail(w, http.StatusForbidden, "forbidden", "No edit access")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_input", "Photo is required")
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, s.maxUploadBytes+1))
	if err != nil || len(body) == 0 {
		fail(w, http.StatusBadRequest, "invalid_input", "Photo is required")
		return
	}
	if int64(len(body)) > s.maxUploadBytes {
		fail(w, http.StatusRequestEntityTooLarge, "upload_too_large", "Photo is too large")
		return
	}
	contentType := detectUploadImageType(body)
	if !allowedImageType(contentType) {
		fail(w, http.StatusBadRequest, "invalid_image", "Only JPEG, PNG, GIF, WebP and HEIC images are allowed")
		return
	}
	converted, err := convertImageToWebP(body)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_image", "Photo could not be decoded")
		return
	}
	if err := s.ensureBucket(r.Context()); err != nil {
		fail(w, http.StatusServiceUnavailable, "media_unavailable", "Media storage is unavailable")
		return
	}
	mediaID := uuid.NewString()
	originalKey, objectKey := mediaObjectKeys(spaceID, mediaID, contentType)
	if _, err := s.minio.PutObject(r.Context(), s.mediaBucket, originalKey, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: contentType}); err != nil {
		fail(w, http.StatusServiceUnavailable, "media_unavailable", "Could not store photo")
		return
	}
	if _, err := s.minio.PutObject(r.Context(), s.mediaBucket, objectKey, bytes.NewReader(converted), int64(len(converted)), minio.PutObjectOptions{ContentType: "image/webp"}); err != nil {
		_ = s.minio.RemoveObject(r.Context(), s.mediaBucket, originalKey, minio.RemoveObjectOptions{})
		fail(w, http.StatusServiceUnavailable, "media_unavailable", "Could not store photo")
		return
	}
	if _, err := s.db.Exec(r.Context(), "insert into media(id,storage_space_id,object_key,mime_type,byte_size,created_by) values($1,$2,$3,$4,$5,$6)", mediaID, spaceID, objectKey, "image/webp", len(converted), current(r).ID); err != nil {
		_ = s.minio.RemoveObject(r.Context(), s.mediaBucket, objectKey, minio.RemoveObjectOptions{})
		_ = s.minio.RemoveObject(r.Context(), s.mediaBucket, originalKey, minio.RemoveObjectOptions{})
		fail(w, http.StatusBadRequest, "create_failed", "Could not save photo metadata")
		return
	}
	s.audit(r, spaceID, "media", mediaID, "uploaded", map[string]string{"contentType": contentType, "convertedContentType": "image/webp"})
	respond(w, http.StatusCreated, map[string]any{"id": mediaID, "contentType": "image/webp", "byteSize": len(converted), "url": "/api/v1/media/" + mediaID})
}
func (s *server) getMedia(w http.ResponseWriter, r *http.Request) {
	if s.minio == nil {
		fail(w, http.StatusServiceUnavailable, "media_unavailable", "Media storage is unavailable")
		return
	}
	mediaID := r.PathValue("mediaID")
	var spaceID, objectKey, contentType string
	var size int64
	if err := s.db.QueryRow(r.Context(), "select storage_space_id,object_key,mime_type,byte_size from media where id=$1 and deleted_at is null", mediaID).Scan(&spaceID, &objectKey, &contentType, &size); err != nil {
		fail(w, http.StatusNotFound, "not_found", "Photo not found")
		return
	}
	if !s.can(r, spaceID, "viewer") {
		fail(w, http.StatusForbidden, "forbidden", "No access")
		return
	}
	object, err := s.minio.GetObject(r.Context(), s.mediaBucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		fail(w, http.StatusNotFound, "not_found", "Photo not found")
		return
	}
	defer object.Close()
	if _, err := object.Stat(); err != nil {
		fail(w, http.StatusNotFound, "not_found", "Photo not found")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = io.Copy(w, object)
}

func (s *server) ensureBucket(ctx context.Context) error {
	exists, err := s.minio.BucketExists(ctx, s.mediaBucket)
	if err != nil || exists {
		return err
	}
	return s.minio.MakeBucket(ctx, s.mediaBucket, minio.MakeBucketOptions{})
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func envInt64(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func allowedImageType(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/heic":
		return true
	default:
		return false
	}
}

func detectUploadImageType(data []byte) string {
	if isHEIC(data) {
		return "image/heic"
	}
	return http.DetectContentType(data)
}

func isHEIC(data []byte) bool {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return false
	}
	for offset := 8; offset+4 <= len(data) && offset < 64; offset += 4 {
		switch string(data[offset : offset+4]) {
		case "heic", "heix", "hevc", "hevx", "heim", "heis":
			return true
		}
	}
	return false
}

func convertImageToWebP(data []byte) ([]byte, error) {
	var decoded image.Image
	var err error
	if isHEIC(data) {
		decoded, err = heic.Decode(bytes.NewReader(data))
	} else {
		decoded, _, err = image.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, err
	}
	if !isHEIC(data) {
		decoded = applyEXIFOrientation(decoded, jpegEXIFOrientation(data))
	}
	var output bytes.Buffer
	if err := webp.Encode(&output, decoded, &webp.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func jpegEXIFOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for offset := 2; offset+4 <= len(data); {
		if data[offset] != 0xff {
			return 1
		}
		marker := data[offset+1]
		offset += 2
		if marker == 0xd9 || marker == 0xda || offset+2 > len(data) {
			return 1
		}
		length := int(data[offset])<<8 | int(data[offset+1])
		if length < 2 || offset+length > len(data) {
			return 1
		}
		if marker == 0xe1 && length >= 16 && string(data[offset+2:offset+8]) == "Exif\x00\x00" {
			return tiffOrientation(data[offset+8 : offset+length])
		}
		offset += length
	}
	return 1
}

func tiffOrientation(data []byte) int {
	if len(data) < 14 {
		return 1
	}
	littleEndian := string(data[:2]) == "II"
	if !littleEndian && string(data[:2]) != "MM" {
		return 1
	}
	uint16At := func(offset int) uint16 {
		if littleEndian {
			return uint16(data[offset]) | uint16(data[offset+1])<<8
		}
		return uint16(data[offset])<<8 | uint16(data[offset+1])
	}
	uint32At := func(offset int) uint32 {
		if littleEndian {
			return uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
		}
		return uint32(data[offset])<<24 | uint32(data[offset+1])<<16 | uint32(data[offset+2])<<8 | uint32(data[offset+3])
	}
	if uint16At(2) != 42 {
		return 1
	}
	ifd := int(uint32At(4))
	if ifd < 0 || ifd+2 > len(data) {
		return 1
	}
	entries := int(uint16At(ifd))
	for entry := ifd + 2; entries > 0 && entry+12 <= len(data); entries, entry = entries-1, entry+12 {
		if uint16At(entry) == 0x0112 && uint16At(entry+2) == 3 && uint32At(entry+4) >= 1 {
			if orientation := int(uint16At(entry + 8)); orientation >= 1 && orientation <= 8 {
				return orientation
			}
		}
	}
	return 1
}

func applyEXIFOrientation(source image.Image, orientation int) image.Image {
	if orientation == 1 {
		return source
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	rotated := orientation >= 5
	output := image.NewNRGBA(image.Rect(0, 0, width, height))
	if rotated {
		output = image.NewNRGBA(image.Rect(0, 0, height, width))
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			destinationX, destinationY := x, y
			switch orientation {
			case 2:
				destinationX = width - 1 - x
			case 3:
				destinationX, destinationY = width-1-x, height-1-y
			case 4:
				destinationY = height - 1 - y
			case 5:
				destinationX, destinationY = y, x
			case 6:
				destinationX, destinationY = height-1-y, x
			case 7:
				destinationX, destinationY = height-1-y, width-1-x
			case 8:
				destinationX, destinationY = y, width-1-x
			}
			output.Set(destinationX, destinationY, source.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return output
}

func mediaObjectKeys(spaceID, mediaID, contentType string) (string, string) {
	extension := map[string]string{
		"image/jpeg": "jpg",
		"image/png":  "png",
		"image/gif":  "gif",
		"image/heic": "heic",
		"image/webp": "webp",
	}[contentType]
	base := "spaces/" + spaceID + "/media/" + mediaID
	if extension == "webp" {
		return base + ".original.webp", base + ".webp"
	}
	return base + "." + extension, base + ".webp"
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	if !s.registrationEnabled {
		fail(w, http.StatusForbidden, "registration_disabled", "Registration is disabled")
		return
	}
	if s.db == nil {
		fail(w, 500, "database_unavailable", "Database unavailable")
		return
	}
	var in struct{ Username, DisplayName, Password string }
	if !decode(r, &in) || len(strings.TrimSpace(in.Username)) < 3 || len(in.Password) < 12 {
		fail(w, 400, "invalid_input", "Username (3+) and password (12+) are required")
		return
	}
	id := uuid.NewString()
	_, err := s.db.Exec(r.Context(), "insert into users(id,username,display_name,password_hash) values($1,$2,$3,$4)", id, strings.ToLower(strings.TrimSpace(in.Username)), strings.TrimSpace(in.DisplayName), hashPassword(in.Password))
	if err != nil {
		fail(w, 409, "username_taken", "Username is unavailable")
		return
	}
	s.issueSession(w, r, id)
	respond(w, 201, map[string]string{"id": id, "username": in.Username})
}
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password string }
	if !decode(r, &in) {
		fail(w, 400, "invalid_input", "Invalid JSON")
		return
	}
	var id, stored string
	if s.db == nil || s.db.QueryRow(r.Context(), "select id,password_hash from users where lower(username)=lower($1)", in.Username).Scan(&id, &stored) != nil || !verifyPassword(in.Password, stored) {
		fail(w, 401, "invalid_credentials", "Invalid username or password")
		return
	}
	s.issueSession(w, r, id)
	respond(w, 200, map[string]string{"id": id})
}
func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("session"); e == nil && s.db != nil {
		_, _ = s.db.Exec(r.Context(), "update sessions set revoked_at=now() where token_hash=$1", tokenHash(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/api", MaxAge: -1, HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(204)
}
func (s *server) me(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, r.Context().Value(userKey))
}
func (s *server) listSpaces(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	rows, e := s.db.Query(r.Context(), "select s.id,s.name,s.description,coalesce(s.address,''),m.role from storage_spaces s join memberships m on m.storage_space_id=s.id where m.user_id=$1 and m.left_at is null and s.archived_at is null order by s.created_at", u.ID)
	if e != nil {
		fail(w, 500, "query_failed", "Could not list spaces")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var d, a *string
		var role string
		_ = rows.Scan(&id, &name, &d, &a, &role)
		out = append(out, map[string]any{"id": id, "name": name, "description": d, "address": a, "role": role})
	}
	respond(w, 200, out)
}
func (s *server) createSpace(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Name, Description, Address string }
	if !decode(r, &in) || strings.TrimSpace(in.Name) == "" {
		fail(w, 400, "invalid_input", "Name is required")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "transaction_failed", "Could not create space")
		return
	}
	defer tx.Rollback(r.Context())
	id := uuid.NewString()
	if _, e = tx.Exec(r.Context(), "insert into storage_spaces(id,name,description,address,owner_user_id) values($1,$2,$3,$4,$5)", id, in.Name, in.Description, in.Address, u.ID); e == nil {
		_, e = tx.Exec(r.Context(), "insert into memberships(storage_space_id,user_id,role) values($1,$2,'owner')", id, u.ID)
	}
	if e == nil {
		e = event(r, tx, id, "space", id, "created", u.ID, map[string]string{"name": in.Name})
	}
	if e != nil {
		fail(w, 500, "create_failed", "Could not create space")
		return
	}
	_ = tx.Commit(r.Context())
	respond(w, 201, map[string]string{"id": id, "name": in.Name})
}
func (s *server) locations(w http.ResponseWriter, r *http.Request) {
	space := r.PathValue("spaceID")
	if !s.can(r, space, "viewer") {
		fail(w, 403, "forbidden", "No access")
		return
	}
	rows, e := s.db.Query(r.Context(), "select id,name,code,description,icon,parent_location_id,case when deleted_at is null then state else 'deleted' end from locations where storage_space_id=$1 order by name", space)
	if e != nil {
		fail(w, 500, "query_failed", "Could not list locations")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, n string
		var code, d, icon, parent *string
		var st string
		_ = rows.Scan(&id, &n, &code, &d, &icon, &parent, &st)
		out = append(out, map[string]any{"id": id, "name": n, "code": code, "description": d, "icon": icon, "parentLocationId": parent, "state": st})
	}
	respond(w, 200, out)
}
func (s *server) createLocation(w http.ResponseWriter, r *http.Request) {
	space := r.PathValue("spaceID")
	if !s.can(r, space, "editor") {
		fail(w, 403, "forbidden", "No edit access")
		return
	}
	var in struct{ Name, Code, Description, Icon, ParentLocationID string }
	if !decode(r, &in) || strings.TrimSpace(in.Name) == "" {
		fail(w, 400, "invalid_input", "Name is required")
		return
	}
	id := uuid.NewString()
	_, e := s.db.Exec(r.Context(), "insert into locations(id,storage_space_id,name,code,description,icon,parent_location_id) values($1,$2,$3,nullif($4,''),nullif($5,''),nullif($6,''),nullif($7,'')::uuid)", id, space, in.Name, in.Code, in.Description, in.Icon, in.ParentLocationID)
	if e != nil {
		fail(w, 400, "create_failed", "Could not create location")
		return
	}
	s.audit(r, space, "location", id, "created", map[string]string{"name": in.Name})
	respond(w, 201, map[string]string{"id": id, "name": in.Name})
}
func (s *server) boxes(w http.ResponseWriter, r *http.Request) {
	space := r.PathValue("spaceID")
	if !s.can(r, space, "viewer") {
		fail(w, 403, "forbidden", "No access")
		return
	}
	rows, e := s.db.Query(r.Context(), "select b.id,b.name,b.description,b.current_location_id,b.temporary_location_id,case when b.deleted_at is null then b.state else 'deleted' end,count(i.id) from boxes b left join items i on i.box_id=b.id and i.deleted_at is null where b.storage_space_id=$1 group by b.id order by b.name", space)
	if e != nil {
		fail(w, 500, "query_failed", "Could not list boxes")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, n, st string
		var d, c, t *string
		var count int
		_ = rows.Scan(&id, &n, &d, &c, &t, &st, &count)
		out = append(out, map[string]any{"id": id, "name": n, "description": d, "currentLocationId": c, "temporaryLocationId": t, "state": st, "itemCount": count})
	}
	respond(w, 200, out)
}
func (s *server) createBox(w http.ResponseWriter, r *http.Request) {
	space := r.PathValue("spaceID")
	if !s.can(r, space, "editor") {
		fail(w, 403, "forbidden", "No edit access")
		return
	}
	var in struct{ Name, Description, LocationID string }
	if !decode(r, &in) || strings.TrimSpace(in.Name) == "" {
		fail(w, 400, "invalid_input", "Name is required")
		return
	}
	id := uuid.NewString()
	_, e := s.db.Exec(r.Context(), "insert into boxes(id,storage_space_id,name,description,current_location_id) values($1,$2,$3,nullif($4,''),nullif($5,'')::uuid)", id, space, in.Name, in.Description, in.LocationID)
	if e != nil {
		fail(w, 400, "create_failed", "Could not create box")
		return
	}
	s.audit(r, space, "box", id, "created", map[string]string{"name": in.Name})
	respond(w, 201, map[string]string{"id": id, "name": in.Name})
}
func (s *server) moveBox(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("boxID")
	var space string
	if s.db.QueryRow(r.Context(), "select storage_space_id from boxes where id=$1 and deleted_at is null", id).Scan(&space) != nil || !s.can(r, space, "editor") {
		fail(w, 403, "forbidden", "No edit access")
		return
	}
	var in struct {
		LocationID string `json:"locationId"`
		Temporary  bool
	}
	if !decode(r, &in) {
		fail(w, 400, "invalid_input", "Invalid request")
		return
	}
	column := "current_location_id"
	if in.Temporary {
		column = "temporary_location_id"
	}
	_, e := s.db.Exec(r.Context(), "update boxes set "+column+"=nullif($1,'')::uuid,updated_at=now() where id=$2", in.LocationID, id)
	if e != nil {
		fail(w, 400, "move_failed", "Could not move box")
		return
	}
	s.audit(r, space, "box", id, "moved", map[string]string{"locationId": in.LocationID})
	w.WriteHeader(204)
}
func (s *server) items(w http.ResponseWriter, r *http.Request) {
	space := r.PathValue("spaceID")
	if !s.can(r, space, "viewer") {
		fail(w, 403, "forbidden", "No access")
		return
	}
	rows, e := s.db.Query(r.Context(), "select i.id,i.name,i.description,i.box_id,case when i.deleted_at is null then i.state else 'deleted' end,count(im.media_id),coalesce(array_agg(im.media_id::text order by im.position) filter (where im.media_id is not null),'{}') from items i left join item_media im on im.item_id=i.id where i.storage_space_id=$1 group by i.id order by i.updated_at desc", space)
	if e != nil {
		fail(w, 500, "query_failed", "Could not list items")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, n, st string
		var d, b *string
		var mediaIDs []string
		var photos int
		_ = rows.Scan(&id, &n, &d, &b, &st, &photos, &mediaIDs)
		item := map[string]any{"id": id, "name": n, "description": d, "boxId": b, "state": st, "photoCount": photos}
		if len(mediaIDs) > 0 {
			media := make([]map[string]string, 0, len(mediaIDs))
			for _, mediaID := range mediaIDs {
				media = append(media, map[string]string{"id": mediaID, "url": "/api/v1/media/" + mediaID})
			}
			item["media"] = media
		}
		out = append(out, item)
	}
	respond(w, 200, out)
}
func (s *server) createItem(w http.ResponseWriter, r *http.Request) {
	space := r.PathValue("spaceID")
	if !s.can(r, space, "editor") {
		fail(w, 403, "forbidden", "No edit access")
		return
	}
	var in struct {
		Name, Description, BoxID string
		MediaIDs                 []string
	}
	if !decode(r, &in) || strings.TrimSpace(in.Name) == "" || len(in.MediaIDs) == 0 {
		fail(w, 400, "invalid_input", "Name and at least one photo are required")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "transaction_failed", "Could not create item")
		return
	}
	defer tx.Rollback(r.Context())
	if in.BoxID != "" {
		var valid bool
		if tx.QueryRow(r.Context(), "select exists(select 1 from boxes where id=$1 and storage_space_id=$2 and deleted_at is null)", in.BoxID, space).Scan(&valid) != nil || !valid {
			fail(w, 400, "invalid_input", "Box does not belong to this space")
			return
		}
	}
	for _, mediaID := range in.MediaIDs {
		var valid bool
		if tx.QueryRow(r.Context(), "select exists(select 1 from media where id=$1 and storage_space_id=$2 and deleted_at is null)", mediaID, space).Scan(&valid) != nil || !valid {
			fail(w, 400, "invalid_input", "Photo does not belong to this space")
			return
		}
	}
	id := uuid.NewString()
	_, e = tx.Exec(r.Context(), "insert into items(id,storage_space_id,name,description,box_id) values($1,$2,$3,nullif($4,''),nullif($5,'')::uuid)", id, space, in.Name, in.Description, in.BoxID)
	for pos, media := range in.MediaIDs {
		if e == nil {
			_, e = tx.Exec(r.Context(), "insert into item_media(item_id,media_id,position,is_cover) values($1,$2,$3,$4)", id, media, pos, pos == 0)
		}
	}
	if e == nil {
		e = event(r, tx, space, "item", id, "created", current(r).ID, map[string]string{"name": in.Name})
	}
	if e != nil {
		fail(w, 400, "create_failed", "Could not create item")
		return
	}
	_ = tx.Commit(r.Context())
	respond(w, 201, map[string]string{"id": id, "name": in.Name})
}

func (s *server) patchItem(w http.ResponseWriter, r *http.Request) {
	id, spaceID, ok := s.editableEntity(r, "items", "itemID")
	if !ok {
		fail(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}
	if !s.can(r, spaceID, "editor") {
		fail(w, http.StatusForbidden, "forbidden", "No edit access")
		return
	}
	var in struct {
		Name, Description, State *string
		BoxID                    *string `json:"boxId"`
	}
	if !decode(r, &in) || (in.Name != nil && strings.TrimSpace(*in.Name) == "") {
		fail(w, http.StatusBadRequest, "invalid_input", "Invalid update")
		return
	}
	if in.BoxID != nil && *in.BoxID != "" {
		var valid bool
		if err := s.db.QueryRow(r.Context(), "select exists(select 1 from boxes where id=$1 and storage_space_id=$2 and deleted_at is null)", *in.BoxID, spaceID).Scan(&valid); err != nil || !valid {
			fail(w, http.StatusBadRequest, "invalid_input", "Box does not belong to this space")
			return
		}
	}
	_, err := s.db.Exec(r.Context(), "update items set name=coalesce($1,name),description=coalesce($2,description),state=coalesce($3,state),box_id=case when $4::text is null then box_id else nullif($4,'')::uuid end,updated_at=now() where id=$5", in.Name, in.Description, in.State, in.BoxID, id)
	if err != nil {
		fail(w, http.StatusBadRequest, "update_failed", "Could not update resource")
		return
	}
	s.audit(r, spaceID, "item", id, "updated", map[string]any{"name": in.Name, "description": in.Description, "state": in.State, "boxId": in.BoxID})
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) addItemMedia(w http.ResponseWriter, r *http.Request) {
	id, spaceID, ok := s.editableEntity(r, "items", "itemID")
	if !ok || !s.can(r, spaceID, "editor") {
		fail(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}
	var in struct{ MediaID string }
	if !decode(r, &in) || in.MediaID == "" {
		fail(w, http.StatusBadRequest, "invalid_input", "Photo is required")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		fail(w, 500, "transaction_failed", "Could not add photo")
		return
	}
	defer tx.Rollback(r.Context())
	var valid bool
	if err = tx.QueryRow(r.Context(), "select exists(select 1 from media where id=$1 and storage_space_id=$2 and deleted_at is null)", in.MediaID, spaceID).Scan(&valid); err == nil && valid {
		_, err = tx.Exec(r.Context(), "insert into item_media(item_id,media_id,position,is_cover) values($1,$2,coalesce((select max(position)+1 from item_media where item_id=$1),0),not exists(select 1 from item_media where item_id=$1)) on conflict do nothing", id, in.MediaID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), "update items set updated_at=now() where id=$1", id)
	}
	if err != nil || !valid {
		fail(w, 400, "update_failed", "Could not add photo")
		return
	}
	if err = event(r, tx, spaceID, "item", id, "photo_added", current(r).ID, map[string]string{"mediaId": in.MediaID}); err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "update_failed", "Could not add photo")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) reorderItemMedia(w http.ResponseWriter, r *http.Request) {
	id, spaceID, ok := s.editableEntity(r, "items", "itemID")
	if !ok || !s.can(r, spaceID, "editor") {
		fail(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}
	var in struct{ MediaIDs []string }
	if !decode(r, &in) {
		fail(w, 400, "invalid_input", "Invalid photo order")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		fail(w, 500, "transaction_failed", "Could not reorder photos")
		return
	}
	defer tx.Rollback(r.Context())
	rows, err := tx.Query(r.Context(), "select media_id::text from item_media where item_id=$1", id)
	if err != nil {
		fail(w, 500, "query_failed", "Could not reorder photos")
		return
	}
	existing := map[string]bool{}
	for rows.Next() {
		var mediaID string
		_ = rows.Scan(&mediaID)
		existing[mediaID] = true
	}
	rows.Close()
	if len(in.MediaIDs) != len(existing) {
		fail(w, 400, "invalid_input", "Photo list does not match item")
		return
	}
	for _, mediaID := range in.MediaIDs {
		if !existing[mediaID] {
			fail(w, 400, "invalid_input", "Photo list does not match item")
			return
		}
		delete(existing, mediaID)
	}
	for position, mediaID := range in.MediaIDs {
		if _, err = tx.Exec(r.Context(), "update item_media set position=$1,is_cover=$2 where item_id=$3 and media_id=$4", position, position == 0, id, mediaID); err != nil {
			fail(w, 500, "update_failed", "Could not reorder photos")
			return
		}
	}
	if err = event(r, tx, spaceID, "item", id, "photos_reordered", current(r).ID, map[string]any{"mediaIds": in.MediaIDs}); err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "update_failed", "Could not reorder photos")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) removeItemMedia(w http.ResponseWriter, r *http.Request) {
	id, spaceID, ok := s.editableEntity(r, "items", "itemID")
	if !ok || !s.can(r, spaceID, "editor") {
		fail(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}
	mediaID := r.PathValue("mediaID")
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		fail(w, 500, "transaction_failed", "Could not remove photo")
		return
	}
	defer tx.Rollback(r.Context())
	result, err := tx.Exec(r.Context(), "delete from item_media where item_id=$1 and media_id=$2", id, mediaID)
	if err != nil || result.RowsAffected() != 1 {
		fail(w, 404, "not_found", "Photo not found on item")
		return
	}
	if _, err = tx.Exec(r.Context(), "with ordered as (select media_id,row_number() over(order by position)-1 as next_position from item_media where item_id=$1) update item_media im set position=ordered.next_position,is_cover=(ordered.next_position=0) from ordered where im.item_id=$1 and im.media_id=ordered.media_id", id); err != nil {
		fail(w, 500, "update_failed", "Could not remove photo")
		return
	}
	if err = event(r, tx, spaceID, "item", id, "photo_removed", current(r).ID, map[string]string{"mediaId": mediaID}); err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "update_failed", "Could not remove photo")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) deleteItem(w http.ResponseWriter, r *http.Request) {
	s.archiveEntity(w, r, "items", "item", "itemID")
}
func (s *server) patchBox(w http.ResponseWriter, r *http.Request) {
	s.updateEntity(w, r, "boxes", "box", "boxID")
}
func (s *server) deleteBox(w http.ResponseWriter, r *http.Request) {
	s.archiveEntity(w, r, "boxes", "box", "boxID")
}
func (s *server) patchLocation(w http.ResponseWriter, r *http.Request) {
	s.updateEntity(w, r, "locations", "location", "locationID")
}
func (s *server) deleteLocation(w http.ResponseWriter, r *http.Request) {
	s.archiveEntity(w, r, "locations", "location", "locationID")
}

func (s *server) updateEntity(w http.ResponseWriter, r *http.Request, table, entity, idParam string) {
	id, spaceID, ok := s.editableEntity(r, table, idParam)
	if !ok {
		fail(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}
	if !s.can(r, spaceID, "editor") {
		fail(w, http.StatusForbidden, "forbidden", "No edit access")
		return
	}
	var in struct{ Name, Description, State *string }
	if !decode(r, &in) || (in.Name != nil && strings.TrimSpace(*in.Name) == "") {
		fail(w, http.StatusBadRequest, "invalid_input", "Invalid update")
		return
	}
	_, err := s.db.Exec(r.Context(), "update "+table+" set name=coalesce($1,name),description=coalesce($2,description),state=coalesce($3,state),updated_at=now() where id=$4", in.Name, in.Description, in.State, id)
	if err != nil {
		fail(w, http.StatusBadRequest, "update_failed", "Could not update resource")
		return
	}
	s.audit(r, spaceID, entity, id, "updated", map[string]any{"name": in.Name, "description": in.Description, "state": in.State})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) archiveEntity(w http.ResponseWriter, r *http.Request, table, entity, idParam string) {
	id, spaceID, ok := s.editableEntity(r, table, idParam)
	if !ok {
		fail(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}
	if !s.can(r, spaceID, "editor") {
		fail(w, http.StatusForbidden, "forbidden", "No edit access")
		return
	}
	if _, err := s.db.Exec(r.Context(), "update "+table+" set deleted_at=now(),updated_at=now() where id=$1", id); err != nil {
		fail(w, 500, "archive_failed", "Could not archive resource")
		return
	}
	s.audit(r, spaceID, entity, id, "archived", map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) editableEntity(r *http.Request, table, idParam string) (string, string, bool) {
	id := r.PathValue(idParam)
	var spaceID string
	if s.db.QueryRow(r.Context(), "select storage_space_id from "+table+" where id=$1 and deleted_at is null", id).Scan(&spaceID) != nil {
		return "", "", false
	}
	return id, spaceID, true
}
func (s *server) search(w http.ResponseWriter, r *http.Request) {
	space, q := r.PathValue("spaceID"), strings.TrimSpace(r.URL.Query().Get("q"))
	if !s.can(r, space, "viewer") {
		fail(w, 403, "forbidden", "No access")
		return
	}
	if q == "" {
		respond(w, 200, []any{})
		return
	}
	rows, e := s.db.Query(r.Context(), "select i.id,i.name,i.description,i.box_id,i.state,count(im.media_id),coalesce(array_agg(im.media_id::text order by im.position) filter (where im.media_id is not null),'{}') from items i left join item_media im on im.item_id=i.id where i.storage_space_id=$1 and i.deleted_at is null and (i.search_vector @@ websearch_to_tsquery('simple',$2) or i.name % $2) group by i.id order by similarity(i.name,$2) desc limit 50", space, q)
	if e != nil {
		fail(w, 500, "query_failed", "Search failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, n, st string
		var d, b *string
		var mediaIDs []string
		var photos int
		_ = rows.Scan(&id, &n, &d, &b, &st, &photos, &mediaIDs)
		item := map[string]any{"id": id, "name": n, "description": d, "boxId": b, "state": st, "photoCount": photos, "type": "item"}
		if len(mediaIDs) > 0 {
			media := make([]map[string]string, 0, len(mediaIDs))
			for _, mediaID := range mediaIDs {
				media = append(media, map[string]string{"id": mediaID, "url": "/api/v1/media/" + mediaID})
			}
			item["media"] = media
		}
		out = append(out, item)
	}
	respond(w, 200, out)
}
func (s *server) timeline(w http.ResponseWriter, r *http.Request) {
	entity, id := r.PathValue("entity"), r.PathValue("id")
	if !map[string]bool{"space": true, "location": true, "box": true, "item": true, "media": true}[entity] {
		fail(w, http.StatusNotFound, "not_found", "Timeline not found")
		return
	}
	var spaceID string
	if s.db.QueryRow(r.Context(), "select storage_space_id from audit_events where entity_type=$1 and entity_id=$2::uuid limit 1", entity, id).Scan(&spaceID) != nil {
		fail(w, http.StatusNotFound, "not_found", "Timeline not found")
		return
	}
	if !s.can(r, spaceID, "viewer") {
		fail(w, http.StatusForbidden, "forbidden", "No access")
		return
	}
	rows, e := s.db.Query(r.Context(), "select action,payload,occurred_at from audit_events where entity_type=$1 and entity_id=$2::uuid order by occurred_at desc", entity, id)
	if e != nil {
		fail(w, 400, "query_failed", "Timeline unavailable")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var a string
		var p json.RawMessage
		var at time.Time
		_ = rows.Scan(&a, &p, &at)
		out = append(out, map[string]any{"action": a, "payload": p, "occurredAt": at})
	}
	respond(w, 200, out)
}
func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.db == nil {
			fail(w, 401, "unauthorized", "Sign in required")
			return
		}
		c, e := r.Cookie("session")
		if e != nil {
			fail(w, 401, "unauthorized", "Sign in required")
			return
		}
		var u user
		e = s.db.QueryRow(r.Context(), "select u.id,u.username,u.display_name from sessions s join users u on u.id=s.user_id where s.token_hash=$1 and s.revoked_at is null and s.expires_at>now()", tokenHash(c.Value)).Scan(&u.ID, &u.Username, &u.DisplayName)
		if e != nil {
			fail(w, 401, "unauthorized", "Sign in required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}
func current(r *http.Request) user { return r.Context().Value(userKey).(user) }
func (s *server) can(r *http.Request, space, required string) bool {
	u := current(r)
	var role string
	if s.db.QueryRow(r.Context(), "select role from memberships where storage_space_id=$1 and user_id=$2 and left_at is null", space, u.ID).Scan(&role) != nil {
		return false
	}
	rank := map[string]int{"viewer": 1, "editor": 2, "admin": 3, "owner": 4}
	return rank[role] >= rank[required]
}
func (s *server) audit(r *http.Request, space, typ, id, action string, p any) {
	_, _ = s.db.Exec(r.Context(), "insert into audit_events(storage_space_id,entity_type,entity_id,action,actor_user_id,payload) values($1,$2,$3,$4,$5,$6)", space, typ, id, action, current(r).ID, p)
}
func event(r *http.Request, tx pgx.Tx, space, typ, id, action, actor string, p any) error {
	b, _ := json.Marshal(p)
	_, e := tx.Exec(r.Context(), "insert into audit_events(storage_space_id,entity_type,entity_id,action,actor_user_id,payload) values($1,$2,$3,$4,$5,$6)", space, typ, id, action, actor, b)
	return e
}
func decode(r *http.Request, v any) bool {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v) == nil
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, msg string) {
	respond(w, status, map[string]string{"code": code, "message": msg})
}
func tokenHash(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func (s *server) issueSession(w http.ResponseWriter, r *http.Request, userID string) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	_, _ = s.db.Exec(r.Context(), "insert into sessions(user_id,token_hash,expires_at) values($1,$2,$3)", userID, tokenHash(token), time.Now().Add(30*24*time.Hour))
	http.SetCookie(w, &http.Cookie{Name: "session", Value: token, Path: "/api", HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
}
func hashPassword(p string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	hash := argon2.IDKey([]byte(p), salt, 3, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(salt) + ":" + base64.RawStdEncoding.EncodeToString(hash)
}
func verifyPassword(p, stored string) bool {
	parts := strings.Split(stored, ":")
	if len(parts) != 2 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[0])
	if e != nil {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(parts[1])
	if e != nil {
		return false
	}
	got := argon2.IDKey([]byte(p), salt, 3, 64*1024, 4, uint32(len(want)))
	return string(got) == string(want)
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			origin := r.Header.Get("Origin")
			if origin != "" && !strings.Contains(origin, r.Host) {
				fail(w, 403, "csrf_rejected", "Cross-origin request rejected")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

var _ = errors.New
