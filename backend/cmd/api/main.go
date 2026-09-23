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
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
	s := &server{db: db, registrationEnabled: registrationEnabled, secureCookies: os.Getenv("COOKIE_SECURE") != "false", mediaBucket: env("MINIO_BUCKET", "storere"), maxUploadBytes: envInt64("MAX_UPLOAD_BYTES", 15<<20)}
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
	mux.Handle("PATCH /api/v1/items/{itemID}/media", s.auth(http.HandlerFunc(s.replaceItemMedia)))
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
	contentType := http.DetectContentType(body)
	if !allowedImageType(contentType) {
		fail(w, http.StatusBadRequest, "invalid_image", "Only JPEG, PNG and WebP images are allowed")
		return
	}
	if err := s.ensureBucket(r.Context()); err != nil {
		fail(w, http.StatusServiceUnavailable, "media_unavailable", "Media storage is unavailable")
		return
	}
	mediaID := uuid.NewString()
	objectKey := "spaces/" + spaceID + "/media/" + mediaID
	if _, err := s.minio.PutObject(r.Context(), s.mediaBucket, objectKey, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: contentType}); err != nil {
		fail(w, http.StatusServiceUnavailable, "media_unavailable", "Could not store photo")
		return
	}
	if _, err := s.db.Exec(r.Context(), "insert into media(id,storage_space_id,object_key,mime_type,byte_size,created_by) values($1,$2,$3,$4,$5,$6)", mediaID, spaceID, objectKey, contentType, len(body), current(r).ID); err != nil {
		_ = s.minio.RemoveObject(r.Context(), s.mediaBucket, objectKey, minio.RemoveObjectOptions{})
		fail(w, http.StatusBadRequest, "create_failed", "Could not save photo metadata")
		return
	}
	s.audit(r, spaceID, "media", mediaID, "uploaded", map[string]string{"contentType": contentType})
	respond(w, http.StatusCreated, map[string]any{"id": mediaID, "contentType": contentType, "byteSize": len(body), "url": "/api/v1/media/" + mediaID})
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
	case "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
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
	rows, e := s.db.Query(r.Context(), "select i.id,i.name,i.description,i.box_id,case when i.deleted_at is null then i.state else 'deleted' end,count(im.media_id),min(im.media_id::text) filter (where im.is_cover) from items i left join item_media im on im.item_id=i.id where i.storage_space_id=$1 group by i.id order by i.updated_at desc", space)
	if e != nil {
		fail(w, 500, "query_failed", "Could not list items")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, n, st string
		var d, b, coverID *string
		var photos int
		_ = rows.Scan(&id, &n, &d, &b, &st, &photos, &coverID)
		item := map[string]any{"id": id, "name": n, "description": d, "boxId": b, "state": st, "photoCount": photos}
		if coverID != nil {
			item["media"] = []map[string]string{{"id": *coverID, "url": "/api/v1/media/" + *coverID}}
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
	s.updateEntity(w, r, "items", "item", "itemID")
}
func (s *server) replaceItemMedia(w http.ResponseWriter, r *http.Request) {
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
		fail(w, 500, "transaction_failed", "Could not replace photo")
		return
	}
	defer tx.Rollback(r.Context())
	var valid bool
	if err = tx.QueryRow(r.Context(), "select exists(select 1 from media where id=$1 and storage_space_id=$2 and deleted_at is null)", in.MediaID, spaceID).Scan(&valid); err == nil && valid {
		_, err = tx.Exec(r.Context(), "delete from item_media where item_id=$1", id)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), "insert into item_media(item_id,media_id,position,is_cover) values($1,$2,0,true)", id, in.MediaID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), "update items set updated_at=now() where id=$1", id)
	}
	if err != nil || !valid {
		fail(w, 400, "update_failed", "Could not replace photo")
		return
	}
	if err = event(r, tx, spaceID, "item", id, "photo_replaced", current(r).ID, map[string]string{"mediaId": in.MediaID}); err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "update_failed", "Could not replace photo")
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
	rows, e := s.db.Query(r.Context(), "select id,name,description,box_id,state from items where storage_space_id=$1 and deleted_at is null and (search_vector @@ websearch_to_tsquery('simple',$2) or name % $2) order by similarity(name,$2) desc limit 50", space, q)
	if e != nil {
		fail(w, 500, "query_failed", "Search failed")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, n, st string
		var d, b *string
		_ = rows.Scan(&id, &n, &d, &b, &st)
		out = append(out, map[string]any{"id": id, "name": n, "description": d, "boxId": b, "state": st, "type": "item"})
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
