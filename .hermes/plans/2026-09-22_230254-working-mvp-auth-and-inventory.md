# Storere — рабочий MVP с авторизацией и полным учётом

## Goal

Довести текущий проект до рабочего MVP: пользователь регистрируется и входит через безопасную cookie-сессию, управляет пространствами, местами, коробками, вещами и фотографиями, а React получает весь пользовательский контент только из защищённого `/api/v1`.

## Current context / assumptions

- Репозиторий: `/home/lol/projects/storere`, ветка `main`; на момент планирования HEAD — `2978c1d`.
- Уже есть Docker Compose с PostgreSQL, миграциями, Go API, MinIO и React/nginx. MinIO не опубликован наружу — это правильное production-ограничение.
- Backend пока находится почти полностью в `backend/cmd/api/main.go`. Он содержит базовые routes auth, spaces, locations, boxes, items, search, timeline; не хватает detail/update/delete APIs, membership, tag APIs, корректной history и полноценного media pipeline.
- `GET /api/v1/media/{mediaID}` защищён авторизацией, но намеренно возвращает `501 media_not_configured`; upload endpoint отсутствует.
- `frontend/src/App.tsx` визуально соответствует желаемой Vercel/Notion-стилистике, но показывает `demoItems`, а не данные пользователя. Не менять общую визуальную тему без продуктовой необходимости.
- RTK Query уже использует `baseUrl: '/api/v1/'` и `credentials: 'include'` в `frontend/src/api/api.ts`; hooks должны стать единственным transport-слоем для данных UI.
- Пользовательское требование: картинка обязательна для вещи. Это означает two-step flow: сначала private upload в API, затем create/update item с `mediaIds`; сервер запрещает создание вещи без хотя бы одного доступного в том же пространстве media ID.
- Роли: `owner > admin > editor > viewer`. Viewer только читает; editor меняет инвентарь; admin управляет участниками; owner управляет пространством и не может быть удалён обычным endpoint-ом.
- Отложить: OCR, QR, публичные ссылки, email-reset, realtime и офлайн-изменения. PWA остаётся installable shell с cached static assets, но без фальшивого offline CRUD.

## Architecture / proposed approach

Разбить Go-код на маленькие transport/service/repository пакеты без микросервисов: HTTP handlers декодируют DTO и формируют ошибки, services проверяют инварианты и права, adapters работают с PostgreSQL и MinIO. Сохранять один deployable API и PostgreSQL transactions для доменных изменений и audit events.

React оставить Vite + Redux Toolkit + RTK Query: RTK Query получает и invalidates server data; Redux хранит только UI-состояние (тема, выбранное space ID, открытый диалог). Медиа хранятся в private MinIO bucket, в DOM попадает только authenticated same-origin URL `/api/v1/media/{id}`; object key, credentials и presigned URL никогда не возвращаются браузеру.

## Definition of done

1. Новый пользователь может зарегистрироваться, войти, выйти, восстановить сессию после reload и создать первое пространство.
2. Можно создать, изменить, архивировать/восстановить места, коробки и вещи; у вещи невозможно сохранить форму без фото.
3. Можно загрузить фото, увидеть preview, назначить cover, удалить attachment; доступ к media проверяется по membership.
4. Поиск находит вещи по имени, описанию и тегам; карточка показывает реальный box/location/tags/cover photo.
5. Перемещение коробки, назначение вещи в коробку, архивирование и изменения пишут историю с actor/time/payload.
6. Участники и роли реально влияют на разрешённые действия как в API, так и в UI.
7. Нет demo-данных, открытого MinIO, публичных object URLs или секретов в Git.
8. `go test ./...`, frontend tests/typecheck/build, Compose smoke и `hermes verify --json` проходят.

## Step-by-step tasks

### 0. Establish an executable baseline and task branches

1. Read `README.md`, `docker-compose.yml`, `.env.example`, `backend/go.mod`, `frontend/package.json`, all existing tests, and the current migration before editing.
2. Run the exact baseline checks:

```bash
git status --short --branch
cd backend && docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./...
cd ../frontend && pnpm install --frozen-lockfile && pnpm test && pnpm typecheck && pnpm build
cd .. && docker compose config >/dev/null
```

Expected: all commands exit `0`. Record any existing failure before changing code.

3. Keep each completed vertical slice independently committable. Use the commit names named below; do not commit generated files, `.env`, volumes, or secrets.

### 1. Freeze API DTOs and HTTP conventions before feature work

**Files:** `api/openapi.yaml`, `backend/cmd/api/main_test.go`, `backend/cmd/api/media_test.go`, new `backend/internal/httpapi/errors.go`.

1. Write failing handler tests for:
   - unknown authenticated resource returns `404 {"code":"not_found",...}`;
   - unauthenticated protected endpoint returns `401`;
   - invalid JSON/form data returns `400 invalid_input`;
   - viewer mutation returns `403 forbidden`;
   - every error has `code` and `message`, and only validation errors contain `fieldErrors`.
2. Run the targeted red test:

```bash
cd backend && docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./cmd/api -run 'Test(Unauthorized|Forbidden|Invalid|NotFound)' -count=1
```

Expected before implementation: at least the newly added test fails.

3. Implement central response types instead of returning anonymous maps everywhere:

```go
type ErrorResponse struct {
    Code        string            `json:"code"`
    Message     string            `json:"message"`
    FieldErrors map[string]string `json:"fieldErrors,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
    writeJSON(w, status, ErrorResponse{Code: code, Message: message, FieldErrors: fields})
}
```

4. Complete `api/openapi.yaml` for all MVP paths, including request bodies, normal status codes, shared `ErrorResponse`, `Media`, auth cookie security scheme and `401/403/404` responses. Treat it as API source-of-truth; do not generate a client during this MVP.
5. Rerun exact tests, then all backend tests. Expected: exit `0`.
6. Commit: `refactor: standardize API contracts and errors`.

### 2. Refactor backend by responsibility without changing public behavior

**Files:** split `backend/cmd/api/main.go` into `backend/internal/httpapi/server.go`, `routes.go`, `auth_handlers.go`, `space_handlers.go`, `inventory_handlers.go`, `middleware.go`, `responses.go`; create `backend/internal/service/` and `backend/internal/store/` only where a feature needs it.

1. First add a regression test proving `NewServer` registers all existing paths and health still returns JSON `{"status":"ok"}`.
2. Move one cohesive handler group at a time; preserve handler names and route paths. `main.go` should only load config, open dependencies, construct the server and call `ListenAndServe`.
3. Put role checking behind a single method/service, not in duplicated raw SQL conditionals. Required API:

```go
type Role string
const (
    RoleViewer Role = "viewer"
    RoleEditor Role = "editor"
    RoleAdmin  Role = "admin"
    RoleOwner  Role = "owner"
)
func (s *Service) RequireRole(ctx context.Context, spaceID, userID string, minimum Role) error
```

4. After each move:

```bash
cd backend && gofmt -w cmd/api internal && docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./...
```

Expected: exit `0`; no route behaviour change.
5. Commit: `refactor: separate API handlers from inventory services`.

### 3. Make session authentication production-safe and complete

**Files:** `backend/internal/httpapi/auth_handlers.go`, `backend/internal/httpapi/middleware.go`, `backend/internal/service/auth.go`, `backend/cmd/api/main_test.go`, new migration `backend/migrations/000002_auth_constraints.up.sql` and down migration.

1. Add failing tests for register, login, logout and `/auth/me`:
   - registration disabled yields `403 registration_disabled`;
   - registration creates a session cookie with `HttpOnly`, `SameSite=Lax`, `Path=/api`, and `Secure` when `COOKIE_SECURE=true`;
   - bad password and unknown username produce the same `401 invalid_credentials` response;
   - logout revokes session and clears cookie;
   - a revoked/expired session cannot read `/auth/me`.
2. Implement bounded request body decoding, normalized usernames, `crypto/subtle.ConstantTimeCompare` password-hash verification, session revocation and `last_seen_at` update. Preserve Argon2id; encode parameters alongside the hash for future migration rather than hardcoding an undocumented representation.
3. Add `CHECK` constraints in migration for normalized practical input lengths, then validate input in handlers before queries. Do not expose DB constraint errors.
4. Verify:

```bash
cd backend && docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./... -count=1
```

Expected: exit `0`, including new cookie/session tests.
5. Commit: `feat: complete secure session authentication`.

### 4. Finish private MinIO media upload and authenticated streaming

**Files:** `backend/go.mod`, `backend/go.sum`, `backend/internal/media/minio.go`, `backend/internal/service/media.go`, `backend/internal/httpapi/media_handlers.go`, `backend/cmd/api/media_test.go`, `docker-compose.yml`, `.env.example`, `api/openapi.yaml`.

1. Add failing tests that mock an object storage interface—not a real MinIO server—for:
   - `POST /api/v1/media` without auth is `401`;
   - editor can upload a valid image to their space and receives `{id, contentType, byteSize, url}` where `url == "/api/v1/media/<id>"`;
   - viewer cannot upload (`403`);
   - non-image, empty upload, content-type mismatch and file over `MAX_UPLOAD_BYTES` return `400`/`413`;
   - foreign-space member cannot GET the media (`403`/`404` policy documented consistently);
   - GET streams bytes with stored MIME type and `Cache-Control: private`.
2. Introduce the minimal interface:

```go
type ObjectStore interface {
    EnsureBucket(ctx context.Context, bucket string) error
    Put(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string) error
    Get(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error)
    Remove(ctx context.Context, bucket, key string) error
}
```

3. Implement the MinIO adapter with `github.com/minio/minio-go/v7`. Construct it from `MINIO_ENDPOINT`, `MINIO_USE_SSL`, access key, secret key and `MINIO_BUCKET`. `EnsureBucket` executes at API startup; never publish MinIO ports or credentials to frontend containers.
4. In upload handler require multipart fields `spaceId` and `file`; use `http.MaxBytesReader`; detect type from initial bytes via `http.DetectContentType`, allow JPEG/PNG/WebP only, rewind a temporary file/buffer, create a UUID object key such as `spaces/<spaceID>/media/<mediaID>`, then persist metadata only after object write succeeds. If DB insert fails, delete the just-created object.
5. `GET /media/{id}` must query `media.storage_space_id`, enforce membership first, stream the object with `Content-Type`, `Content-Length` when known, `X-Content-Type-Options: nosniff`, and never return a MinIO error body or object key.
6. Implement media deletion as database soft delete plus object deletion; deny deletion if it is still attached until the request explicitly detaches it. Item update controls attachment lifecycle.
7. Configure `MAX_UPLOAD_BYTES=15728640` in `.env.example`; pin MinIO image rather than using an unreviewed floating tag.
8. Run red/green tests and an integration smoke:

```bash
cd backend && docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./... -count=1
cd .. && docker compose up -d --build && docker compose ps
```

Expected: backend tests exit `0`; postgres/api/web are running; migrate completed successfully. Do not print secrets.
9. Commit: `feat: add private authenticated media storage`.

### 5. Complete inventory CRUD, archival and history

**Files:** `backend/internal/httpapi/{space,location,box,item,tag}_handlers.go`, `backend/internal/service/inventory.go`, `backend/internal/store/postgres.go`, new migration `backend/migrations/000003_inventory_history.up.sql` and down migration, `backend/cmd/api/inventory_test.go`, `api/openapi.yaml`.

1. Write failing tests for all user-visible resource transitions:
   - owner creates space and owner membership atomically;
   - editor creates/updates location, box and item; viewer cannot mutate;
   - item creation/update requires one or more media IDs owned by the same space;
   - box cannot reference location from another space;
   - item cannot reference box from another space;
   - archive excludes resource from normal lists; restore returns it;
   - moving box records old/current/temporary location and expiry in history;
   - assigning/removing an item box records old/new box IDs;
   - each write creates exactly one append-only audit event.
2. Add explicit REST endpoints; do not overload collection routes:

```text
GET/PATCH/DELETE /api/v1/spaces/{spaceID}
GET/PATCH/DELETE /api/v1/locations/{locationID}
GET/PATCH/DELETE /api/v1/boxes/{boxID}
POST              /api/v1/boxes/{boxID}/move
GET/PATCH/DELETE /api/v1/items/{itemID}
POST              /api/v1/items/{itemID}/box
POST              /api/v1/items/{itemID}/restore
GET/POST          /api/v1/spaces/{spaceID}/tags
PATCH/DELETE      /api/v1/tags/{tagID}
GET               /api/v1/{entity}/{id}/timeline
```

3. Use `archived_at` for space and `deleted_at` for location/box/item/media. Return `404` for deleted entities in normal reads; expose an explicit `includeArchived=true` only for editors/admins where the UI needs restore.
4. Fix the current timeline authorization hole: resolve the entity’s `storage_space_id` before selecting audit events, call `RequireRole(..., RoleViewer)`, validate entity type against an allowlist (`space`, `location`, `box`, `item`, `media`).
5. Add tag attachment into item create/update (`tagIds`) and list tag DTOs alongside item. Validate every tag belongs to the item’s storage space.
6. Run:

```bash
cd backend && docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./... -count=1
```

Expected: exit `0`; tests assert database transaction outcomes and access control.
7. Commit: `feat: complete inventory lifecycle and audit history`.

### 6. Add space membership management

**Files:** `backend/internal/httpapi/membership_handlers.go`, `backend/internal/service/membership.go`, `backend/cmd/api/membership_test.go`, `api/openapi.yaml`.

1. Write failing tests: owner/admin can list/change roles; editor/viewer cannot; owner cannot be removed/demoted; user cannot join duplicate active membership; removed member loses access immediately.
2. Implement exact endpoints:

```text
GET    /api/v1/spaces/{spaceID}/members
POST   /api/v1/spaces/{spaceID}/members       # body: {"username":"...","role":"viewer"}
PATCH  /api/v1/spaces/{spaceID}/members/{userID}
DELETE /api/v1/spaces/{spaceID}/members/{userID}
```

3. Membership uses known username only; do not invent email invitation flows. Preserve a historical `left_at`, reactivate only through explicit re-add.
4. Audit member actions with the target user ID and prior/new role.
5. Run targeted then full backend suite; expected exit `0`.
6. Commit: `feat: manage storage space members and roles`.

### 7. Replace the demo screen with authenticated React application shell

**Files:** replace `frontend/src/App.tsx`; add `frontend/src/features/auth/AuthPage.tsx`, `AppShell.tsx`, `RequireSession.tsx`, `frontend/src/features/ui/uiSlice.ts`, `frontend/src/api/api.ts`, `frontend/src/App.test.tsx`, tests under `frontend/src/features/auth/`.

1. Add failing component tests for:
   - unauthenticated `/auth/me` result shows login/register instead of inventory;
   - successful session shows user name and the existing app shell;
   - disabled registration hides registration form;
   - session expiry from any API request returns user to login without a white screen.
2. Add RTK Query endpoints `me`, `login`, `register`, `logout`, and settings only as necessary. Use a `baseQuery` wrapper that dispatches logout UI state on a `401`, but never stores session token in Redux/localStorage.
3. Preserve the current dark/light visual system and sidebar geometry. Replace hard-coded nav buttons with tabs/pages: Search, Things, Boxes, Locations, History, Settings. The globally visible desktop add button and mobile FAB always open the creation menu.
4. Implement accessible login/register forms: labels, `autoComplete`, validation messages bound with `aria-describedby`, submit disabled while pending, and error messages from API.
5. Verify:

```bash
cd frontend && pnpm test && pnpm typecheck && pnpm build
```

Expected: all exit `0`; tests make no network call outside MSW/fetch mocks.
6. Commit: `feat: add authenticated React application shell`.

### 8. Build real RTK Query data pages while preserving the visual language

**Files:** expand `frontend/src/api/api.ts`; add `frontend/src/features/{spaces,items,boxes,locations,tags,history}/`; add associated `*.test.tsx`; update `frontend/src/App.tsx` only for composition.

1. Extend typed API models to include `Media.url`, `Tag`, `Location`, `Box`, detail and mutation payloads. Add tags `Space`, `Item`, `Box`, `Location`, `Tag`, `History`, `Member`, `Media`; mutations must invalidate only affected IDs/list tags.
2. Build the first-run empty state: if `useSpacesQuery` returns `[]`, show “Создать пространство” with modal fields name/description/address. On success select new ID and load it.
3. Build item list/search page:
   - debounce search by 250 ms;
   - use `/spaces/{id}/search` only with non-empty query; otherwise list items;
   - show real cover `<img src={media.url}>`, item name, tags, box/location and state;
   - give meaningful loading, error and empty states;
   - do not include fallback `demoItems`.
4. Build boxes and locations pages with list/detail panels, create/edit/archive/restore controls gated by role. A box card must display permanent and temporary location separately.
5. Build item detail/edit view: update name/description/tags/box/state, show all photos, choose cover and show timeline. Use mutation pending states and API field errors.
6. Build history tab with entity filters and readable audit rows: actor display name, action, relative time plus exact timestamp on hover. Render payload intentionally—never dump raw JSON as the principal UI.
7. Add React Testing Library tests for real query states: load, empty, server error, viewer disabled controls, editor creation success, search result, and no result. Use deterministic mocked responses.
8. Verify after each feature slice:

```bash
cd frontend && pnpm test && pnpm typecheck && pnpm build
```

Expected: exit `0`.
9. Commits:
   - `feat: load inventory views through RTK Query`
   - `feat: manage boxes and locations in the UI`
   - `feat: add item details, search, tags and history`.

### 9. Implement mandatory-photo upload UX

**Files:** `frontend/src/features/media/MediaUploader.tsx`, `MediaUploader.test.tsx`, `frontend/src/features/items/ItemForm.tsx`, `frontend/src/api/api.ts`, `frontend/src/styles.css` (only local additions).

1. Write failing tests that assert an item form cannot submit without uploaded media, does submit after a successful upload, displays a failed upload error, and passes only returned `mediaIds` to create/update item.
2. Implement media mutation with `FormData`; do not manually set multipart `Content-Type`:

```ts
uploadMedia: build.mutation<Media, { spaceId: string; file: File }>({
  query: ({ spaceId, file }) => {
    const body = new FormData()
    body.set('spaceId', spaceId)
    body.set('file', file)
    return { url: 'media', method: 'POST', body }
  },
  invalidatesTags: ['Media'],
})
```

3. Validate type/size client-side for immediate feedback, but keep server validation authoritative. Provide preview via `URL.createObjectURL` only before upload; revoke preview URLs on cleanup. After success render API `media.url`.
4. Render image loading fallback and `alt` derived from item name; never expose MinIO endpoint/object key in the page, logs, types or test snapshots.
5. Run frontend suite/typecheck/build. Expected: exit `0`.
6. Commit: `feat: upload and display private item photos`.

### 10. Add settings, registration toggle and membership UI

**Files:** `backend/internal/httpapi/settings_handlers.go`, `backend/cmd/api/settings_test.go`, `frontend/src/features/settings/SettingsPage.tsx`, test files, `api/openapi.yaml`.

1. Decide and document a bootstrap policy: only an authenticated user who owns at least one space may alter registration settings; if global setting remains singleton, introduce an explicit first-admin bootstrap migration/config rather than an unauthenticated admin route.
2. Add red tests proving non-owner cannot toggle registration and that disabled registration changes frontend behaviour after refresh.
3. Implement `GET/PATCH /api/v1/settings` only if the bootstrap policy is safe and testable. Otherwise omit the UI toggle in MVP and use deployment env / DB seed policy; do not create a security footgun.
4. Build Members section under space settings: list username + display name + role, add by username, role selector, remove action. Hide/disable unavailable actions for non-admins and owner-protected membership.
5. Commit: `feat: add settings and membership controls`.

### 11. PWA, accessibility and responsive completion

**Files:** `frontend/public/manifest.webmanifest`, `frontend/index.html`, `frontend/src/styles.css`, new `frontend/src/features/ui/ResponsiveNav.test.tsx`; add service worker only if a maintained Vite-compatible implementation is available in the existing dependency policy.

1. Keep the existing manifest but verify `name`, `short_name`, `display: standalone`, start URL, theme/background colors and at least 192x192/512x512 icons.
2. Add responsive CSS: sidebar becomes a labelled accessible drawer/navigation at mobile widths; FAB stays reachable; dialogs trap no functionality behind overlay and close with Escape.
3. Use semantic `button`, `label`, `nav`, `main`, headings and focus styles. Verify keyboard navigation for login, add menu and item form.
4. Do not claim full offline CRUD. If adding a service worker, cache only static assets/app shell and provide a clear offline read failure state for API data.
5. Run frontend checks, then a manual viewport smoke at 390px and 1440px. Expected: no horizontal overflow, visible add action, usable navigation.
6. Commit: `feat: complete responsive installable app shell`.

### 12. Documentation, CI and production verification

**Files:** `README.md`, `.env.example`, `docker-compose.yml`, `deploy/nginx/default.conf`, `.github/workflows/ci.yml`, optionally `deploy/dokploy-compose.yml` if Dokploy requires a distinct compose variant.

1. Add CI jobs:
   - backend: `docker run ... golang:1.24 go test ./...`;
   - frontend: `pnpm install --frozen-lockfile`, `pnpm test`, `pnpm typecheck`, `pnpm build`;
   - compose: `docker compose config`.
2. Document local setup, configuration variables by name (not values), migrations, tests, account/first-space flow, roles, media privacy and Dokploy deployment steps. State explicitly that production uses `COOKIE_SECURE=true` and that MinIO has no public ports.
3. Add health/readiness distinction. `/health` is process liveness; `/ready` verifies PostgreSQL and object-storage readiness. Update Compose healthchecks accordingly.
4. Verify exact release gate:

```bash
git diff --check
cd backend && docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./... -count=1
cd ../frontend && pnpm install --frozen-lockfile && pnpm test && pnpm typecheck && pnpm build
cd .. && docker compose config >/dev/null && docker compose up -d --build
docker compose ps
hermes verify --json
```

Expected: all commands exit `0`; migrations complete; API/web run; verification reports readiness at `http://127.0.0.1:8000/`.

5. Run a manual end-to-end browser acceptance sequence using two accounts:
   - register Account A → create space → location → box;
   - upload photo → create item → find it through search;
   - move box temporarily → view history;
   - invite Account B as viewer → confirm B sees data but cannot mutate;
   - request `/api/v1/media/<id>` as a non-member → rejected;
   - confirm no browser network request targets MinIO host.
6. Commit: `chore: document and verify production MVP`.

## Tests / validation matrix

| Layer | Required proof |
|---|---|
| Domain/service | Role ranking, same-space references, media-required item invariant, archive/restore, history payloads |
| HTTP API | Auth cookie lifecycle, input validation, `401/403/404`, all CRUD routes, media stream, no data leaks |
| Storage | Mock object store unit tests and Compose integration upload/read smoke |
| Frontend | Auth gates, RTK Query loading/error/empty states, photo-required form, role-disabled controls, search, add button |
| End-to-end | Two-account role scenario and no direct MinIO browser access |
| Quality | `gofmt`, `git diff --check`, Go tests, frontend tests/typecheck/build, Compose config, `hermes verify --json` |

Every implementation task follows RED → GREEN → REFACTOR: write one test that fails for the required behavior, run only that test to confirm failure, add the smallest implementation, run the targeted test, run its complete suite, then commit the coherent slice. Do not write large untested handler blocks and try to debug the entire application at the end.

## Risks, tradeoffs and open questions

1. **Existing monolithic API:** moving all code at once risks regressions. Preserve external routes and introduce packages incrementally behind tests.
2. **MinIO consistency:** object upload and PostgreSQL transaction cannot be truly atomic without an outbox/reconciliation job. For MVP, compensate by deleting uploaded object if DB metadata insertion fails; log failures safely. Add an outbox only when reliability requirements warrant it.
3. **Registration toggle security:** a global toggle has no safe unauthenticated management path. It must have a documented bootstrap/owner policy or remain deployment-managed; never add a public “enable registration” route.
4. **Image processing:** no resizing/transcoding in MVP. Enforce type/size at upload and use native images. Add background conversion only after real performance evidence.
5. **Location semantics:** current schema supports permanent and temporary box locations but does not model arbitrary multiple temporary stays. MVP records transitions in audit history; a dedicated movement table is a later enhancement if reporting becomes necessary.
6. **PWA:** service workers can serve stale assets. Limit caching to app shell/static files until offline data synchronization is designed.
7. **No email/reset:** account recovery is intentionally not shown. Add a real email provider and rate-limited reset tokens as a separate security-reviewed feature.
