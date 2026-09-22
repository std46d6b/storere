# Доработка Storere: завершение MVP, API-only данные и приватные фото

## Goal

Довести `storere` до работающего MVP, в котором React получает все пользовательские данные только через `/api/v1`, а изображения загружаются и отдаются исключительно авторизованным Go API без публичных или подписанных прямых URL MinIO.

## Current context / assumptions

- Репозиторий: `github.com/std46d6b/storere`, ветка `main`, текущий HEAD на момент планирования — `34f1349`.
- Backend — один 90-строчный файл `backend/cmd/api/main.go`; он использует `net/http`, `pgx` и PostgreSQL. В нём есть базовые endpoints auth, spaces, locations, boxes, items, search и timeline.
- Frontend — React/Vite. `frontend/src/App.tsx` показывает только `demoItems`; RTK Query описан в `frontend/src/api/api.ts`, но hooks не используются UI.
- В миграции есть `media` и `item_media`, Compose запускает MinIO, но нет загрузки, чтения, проверки типа/размера и отображения изображений.
- Nginx проксирует `/api/` в API. Production MinIO не должен иметь опубликованного портa; текущий `9001:9001` допустим только для profile `dev` и должен быть удалён из default Compose.
- Доменная терминология уже определена: `storage_space` — граница доступа; `location` — место в дереве; `box`; `item`; `media`.
- Политика для этого релиза: все фото хранятся в приватном bucket `storere`; браузер никогда не получает MinIO credentials, presigned URL или постоянный `object_key`. API получает multipart файл, проверяет его, кладёт в MinIO и возвращает `/api/v1/media/{id}`. API stream-ит байты фотографии после проверки membership.
- Не включать в этот цикл: OCR/распознавание, QR, email delivery, публичные ссылки, платежи, real-time websocket. Для сброса пароля оставить endpoint без реальной почтовой доставки только если владелец отдельно подтвердит этот UX; по умолчанию **не рекламировать** неработающую кнопку восстановления.

## Architecture / proposed approach

Разделить текущий монолитный `main.go` на HTTP transport, services и PostgreSQL/MinIO adapters, но не вводить микросервисы. Все операции чтения и записи выполняются через Go `/api/v1`; React получает DTO только через RTK Query, а локальный Redux содержит лишь UI-состояние (выбранное пространство, темы, открытые диалоги).

MinIO остаётся внутренней зависимостью API: `media` содержит metadata и private `object_key`, API upload-ит объект и API stream-ит его для просмотра. Каждая media-операция сначала проверяет, что текущий пользователь состоит в `storage_space_id`; для entity cover/content media API возвращает **API URL**, а не object-storage URL.

## API contract to implement

Создать `api/openapi.yaml` как единственный контракт. Минимальный набор ответов и endpoints:

| Method / path | Auth | Назначение |
|---|---|---|
| `POST /api/v1/auth/register` | no | регистрация, только когда registration toggle включён |
| `POST /api/v1/auth/login`, `POST /logout`, `GET /me` | mixed | HTTP-only session lifecycle |
| `GET/POST /api/v1/spaces` | yes | список и создание пространства |
| `GET/PATCH /api/v1/spaces/{id}` | member / owner | detail и настройки |
| `GET/POST /api/v1/spaces/{id}/members`, `PATCH/DELETE /.../members/{userID}` | admin | membership |
| `GET/POST /api/v1/spaces/{id}/locations` | viewer/editor | дерево мест |
| `GET/PATCH/DELETE /api/v1/locations/{id}` | viewer/editor | detail, update, soft-delete |
| `GET/POST /api/v1/spaces/{id}/boxes` | viewer/editor | список и создание |
| `GET/PATCH/DELETE /api/v1/boxes/{id}` | viewer/editor | detail, update, soft-delete |
| `POST /api/v1/boxes/{id}/move` | editor | permanent/temporary move с историей |
| `GET/POST /api/v1/spaces/{id}/items` | viewer/editor | список и создание |
| `GET/PATCH/DELETE /api/v1/items/{id}` | viewer/editor | detail, update, soft-delete |
| `POST /api/v1/items/{id}/box` | editor | назначение/изъятие с `item_box_history` |
| `GET/POST /api/v1/spaces/{id}/tags` | viewer/editor | tags |
| `POST /api/v1/media` | editor | multipart upload через API в private MinIO |
| `GET /api/v1/media/{id}` | member | API-stream фото; `Cache-Control: private, max-age=86400` |
| `DELETE /api/v1/media/{id}` | editor | soft-delete metadata и delete object после transaction/outbox policy |
| `GET /api/v1/spaces/{id}/search?q=&cursor=` | viewer | FTS/trigram search и cursor pagination |
| `GET /api/v1/{entity}/{id}/timeline` | member | append-only history |
| `GET /health`, `GET /ready` | no | liveness/readiness |

All error responses must be:

```json
{"code":"forbidden","message":"You do not have access to this storage space","fieldErrors":{"name":"required"}}
```

The `fieldErrors` key is omitted unless applicable. Do not expose database errors, bucket names, keys, passwords, or stack traces.

## Step-by-step tasks

### 1. Establish a clean baseline and prevent generated files from returning

1. Read `README.md`, `.gitignore`, `frontend/package.json`, `backend/go.mod`, current migrations and `git status --short --branch`.
2. Add these exact ignore rules to `.gitignore` if absent:

```gitignore
node_modules/
frontend/node_modules/
frontend/dist/
frontend/*.tsbuildinfo
frontend/vite.config.js
frontend/vite.config.d.ts
.env
.env.*
!.env.example
```

3. Do not retain generated `frontend/dist`, `node_modules`, `*.tsbuildinfo`, or generated Vite config JS in Git. Remove them from Git index with `git rm -r --cached …` only if `git ls-files` shows they are tracked; do not delete the lockfile.
4. Add `.env.example` with **names only**, no credentials:

```dotenv
DATABASE_URL=postgres://storere:storere_dev_only@postgres:5432/storere?sslmode=disable
COOKIE_SECURE=false
SESSION_TTL_HOURS=720
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=storere
MINIO_SECRET_KEY=change-this-in-a-real-environment
MINIO_BUCKET=storere
MINIO_USE_SSL=false
MAX_UPLOAD_BYTES=15728640
REGISTRATION_ENABLED=true
```

5. Verify before the first implementation commit:

```bash
git status --short
cd frontend && pnpm install --frozen-lockfile && pnpm test && pnpm typecheck && pnpm build
cd ../backend && docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./...
```

Expected: all test/build commands exit `0`; `git status` contains only intentionally modified source/config files.

Commit: `chore: establish clean repository baseline`.

### 2. Make the API contract and generated client types authoritative

1. Create `api/openapi.yaml` with OpenAPI 3.1, server URL `/api/v1`, `cookieAuth` security scheme (`type: apiKey`, `in: cookie`, `name: session`), shared `Error`, `User`, `Space`, `Location`, `Box`, `Item`, `Media`, `Tag`, `TimelineEvent`, and page/cursor schemas.
2. Include every endpoint in the API table above. The `Media` schema must be exactly:

```yaml
Media:
  type: object
  required: [id, contentType, byteSize, url]
  properties:
    id: { type: string, format: uuid }
    contentType: { type: string, example: image/webp }
    byteSize: { type: integer, format: int64 }
    url: { type: string, pattern: '^/api/v1/media/[0-9a-f-]{36}$' }
```

3. Add `openapi-typescript` as a frontend dev dependency and scripts to `frontend/package.json`:

```json
"generate:api": "openapi-typescript ../api/openapi.yaml -o src/api/generated.ts",
"check:api": "pnpm generate:api && git diff --exit-code -- src/api/generated.ts"
```

4. Generate and commit `frontend/src/api/generated.ts`. Handwritten API DTO types in `frontend/src/api/api.ts` must import generated types instead of duplicating shapes.
5. Add `frontend/src/api/api.contract.test.ts` first. It must read `generated.ts` imports and validate `Media.url` is used by the API module, not `objectKey` or `minio`. Run it and observe an import/type failure before replacing handwritten types.
6. Verification:

```bash
cd frontend
pnpm generate:api
pnpm check:api
pnpm test -- api.contract
pnpm typecheck
```

Expected: exit `0`; re-running `pnpm generate:api` produces no Git diff.

Commit: `feat: add authoritative OpenAPI contract`.

### 3. Refactor backend into testable layers without changing behaviour

1. Create these exact directories and move code by responsibility:

```text
backend/cmd/api/main.go
backend/internal/config/config.go
backend/internal/httpapi/router.go
backend/internal/httpapi/response.go
backend/internal/httpapi/auth_handlers.go
backend/internal/httpapi/inventory_handlers.go
backend/internal/httpapi/media_handlers.go
backend/internal/auth/password.go
backend/internal/auth/session.go
backend/internal/domain/models.go
backend/internal/domain/roles.go
backend/internal/repository/postgres.go
backend/internal/repository/inventory.go
backend/internal/storage/minio.go
backend/internal/service/auth.go
backend/internal/service/inventory.go
backend/internal/service/media.go
```

2. Define dependency interfaces in `backend/internal/service` rather than passing `*pgxpool.Pool` into handlers:

```go
type MediaStore interface {
    Put(ctx context.Context, key, contentType string, body io.Reader, size int64) error
    Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
    Remove(ctx context.Context, key string) error
}

type InventoryRepository interface {
    IsMember(ctx context.Context, userID, spaceID uuid.UUID, minimum domain.Role) (bool, error)
    // Add narrow methods per use case; do not add a generic Execute method.
}
```

3. Keep `http.ServeMux`. `NewRouter(deps Dependencies) http.Handler` must be the only router construction point. `main.go` reads config, opens pgx/MinIO clients, builds `Dependencies`, and serves it.
4. Write tests **before** each extracted unit:
   - `backend/internal/auth/password_test.go`: password hash verifies correct password and rejects incorrect password.
   - `backend/internal/httpapi/response_test.go`: invalid body returns `400` and exact error JSON.
   - `backend/internal/domain/roles_test.go`: viewer cannot edit, editor can edit, admin can manage members, only owner can transfer ownership/delete space.
5. For each file: run one focused test, confirm it fails for a missing symbol/behaviour, implement the smallest function, rerun focused test, then run `go test ./...`.

```bash
cd backend
docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./internal/auth -run TestVerifyPassword -count=1
docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./...
```

Expected: focused test first fails, then all packages report `ok`.

Commit after the extraction only: `refactor: split API transport and services`.

### 4. Correct database schema, migrations and audit history

1. Add `backend/migrations/000002_inventory_completion.up.sql` and `.down.sql`; do not edit `000001_init.up.sql` after a shared environment may have run it.
2. In `000002` add:
   - `box_location_history` with `from_location_id`, `to_location_id`, `placement_kind`, actor, note and timestamp.
   - `item_box_history` with from/to box, actor, note and timestamp.
   - `box_media`, `location_media` and a unique partial index for one cover per entity.
   - `temporary_reason` and `temporary_until` on `boxes` only if not already present.
   - `deleted_by uuid REFERENCES users(id)` on `items`, `boxes`, `locations`, `media`.
   - partial indexes filtering `deleted_at IS NULL` and all foreign-key indexes.
   - a case-insensitive tag uniqueness index: `CREATE UNIQUE INDEX tags_space_name_lower_uq ON tags(storage_space_id, lower(name));`.
3. Never use a `deleted` state enum: the deletion state is `deleted_at IS NOT NULL`. Retain only `active`, `temporarily_unavailable`, `archived` in the `state` check constraint.
4. Add `backend/internal/repository/inventory_integration_test.go` using Testcontainers-Go and migrations. Test, in order:
   - creating an active item without `item_media` is rejected by the service;
   - a box move inserts an audit event and `box_location_history` in the same transaction;
   - `deleted_at` items are absent from normal search;
   - querying another space returns no rows;
   - an active case-insensitive duplicate tag is rejected.
5. Run the test red before each service/repository behaviour is implemented. Do not accept a test that mocks PostgreSQL for SQL isolation/transaction behaviour.
6. Verify:

```bash
cd backend
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$PWD:/src" -w /src golang:1.24 go test ./internal/repository -run Integration -count=1
docker compose down -v
docker compose up -d postgres
# Wait for `docker compose ps` to report postgres healthy, then:
docker compose run --rm migrate
```

Expected: integration tests pass; migrator exits `0`; `schema_migrations` reports versions `1` and `2`.

Commit: `feat: complete inventory schema and audit history`.

### 5. Implement private MinIO media through API only

1. Add MinIO Go SDK dependency:

```bash
cd backend
go get github.com/minio/minio-go/v7@latest
go mod tidy
```

2. Implement `backend/internal/storage/minio.go`. Constructor must create the bucket at startup and set no public policy. Its public surface must match `MediaStore`; it must not expose `PresignedGetObject`, `PresignedPutObject`, endpoint URL, access key or secret to handlers/DTOs.
3. Add `POST /api/v1/media` handler with this exact acceptance policy:
   - authenticated `editor` of supplied `spaceId`;
   - `multipart/form-data` part name `file` and text field `spaceId`;
   - max `MAX_UPLOAD_BYTES` (default 15 MiB) via `http.MaxBytesReader`;
   - use `http.DetectContentType` on first 512 bytes and permit only JPEG, PNG, WebP; reject HEIC until a real conversion pipeline is installed;
   - generate UUID object key `spaces/{spaceID}/media/{mediaID}`; never use original filename;
   - upload the stream to MinIO, insert `media` row only after successful object storage write; if DB insertion fails, delete the newly stored object;
   - return `201` with `Media` DTO and `url: "/api/v1/media/{id}"`.
4. Implement `GET /api/v1/media/{id}`:
   - authenticate;
   - query media joined to its space, verify viewer membership before fetching object;
   - `Content-Type` from DB; set `X-Content-Type-Options: nosniff`, `Cache-Control: private, max-age=86400`; use `http.ServeContent`/range support or a correctly sized `io.Copy`;
   - return `404` for deleted/missing media and `403` for a user outside the space; never redirect to MinIO.
5. Add `DELETE /api/v1/media/{id}` with editor check and prevent deletion if it would leave an active item with zero images. In the first release, soft-delete DB media and remove object synchronously; only use an outbox if retries are actually added in the same change.
6. Change `docker-compose.yml`:
   - remove `ports: ["9001:9001"]` from default MinIO service;
   - supply `MINIO_*` vars to API;
   - add a separate `minio-console` service under `profiles: ["dev"]` if console access is needed;
   - replace `latest` images with immutable version tags/digests after choosing current supported versions.
7. Write tests before code:
   - `backend/internal/service/media_test.go` with an in-memory `MediaStore`: non-image rejected, oversized rejected, editor upload succeeds, viewer upload forbidden.
   - `backend/internal/httpapi/media_handlers_test.go`: unauthenticated GET returns 401; a member receives bytes and `private` cache header; another-space user receives 403; response body/header never include `minio`, `object_key`, or URL scheme.
   - `backend/internal/storage/minio_integration_test.go`: with Testcontainers MinIO, uploaded bytes round-trip and bucket anonymous policy is not readable.
8. Verification:

```bash
cd backend
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$PWD:/src" -w /src golang:1.24 go test ./internal/service ./internal/httpapi ./internal/storage -count=1
docker compose up -d --build
curl -i http://localhost:8000/api/v1/media/not-a-uuid
```

Expected: Go tests pass; unauthenticated curl returns `401` JSON; no MinIO host/port appears in response headers/body.

Commit: `feat: serve private media through authorized API`.

### 6. Finish auth, membership and settings behaviour

1. Add repository/service/handler tests before endpoints for membership create/list/update/remove and ownership transfer.
2. Add a real `app_settings.registration_enabled` read for each registration request or a short bounded cache invalidated by settings update; do not freeze this value at process start as current `main.go` does.
3. Add `PATCH /api/v1/admin/settings/registration` restricted to a deployment-defined system admin list (`SYSTEM_ADMIN_USERNAMES` comma-separated). If no system admins are configured, endpoint returns `404`; never let arbitrary space owner alter global registration.
4. Implement membership rules exactly:
   - owner: all actions including transfer owner;
   - admin: manage non-owner members and restore deleted records;
   - editor: CRUD inventory/media but no members or restore;
   - viewer: reads only;
   - do not remove/demote the final owner.
5. Audit every membership/settings transition in `audit_events` in the same transaction.
6. Verification commands per test cycle:

```bash
cd backend
docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./internal/service -run 'Test(ChangeRole|RejectLastOwnerRemoval|RegistrationToggle)' -count=1
docker run --rm -v "$PWD:/src" -w /src golang:1.24 go test ./...
```

Expected: all pass; an editor receives `403` from member/settings mutations.

Commit: `feat: complete roles and registration controls`.

### 7. Complete inventory API use-cases before rendering them

1. Add tests and minimal implementations for update, archive, soft-delete, restore, detail and relationship endpoints in `backend/internal/service/inventory_test.go` and `backend/internal/httpapi/inventory_handlers_test.go`.
2. Make every mutation use one service transaction that writes both current entity state and an audit event. For movement/box-assignment, also insert specialised history row.
3. `POST /items/{id}/box` must accept `{ "boxId": "uuid-or-null", "note": "optional" }`, confirm item and box are in the same space, update `items.box_id`, write `item_box_history`, and write `audit_events` atomically.
4. Box list/detail response must return `previewMedia` as at most four `Media` DTOs (`/api/v1/media/{id}`); do not make frontend construct URLs from object keys.
5. Search must query name, description, tag, box and location. Use `websearch_to_tsquery('simple', $q)` plus trigram fallback; use `(updated_at, id)` cursor pagination rather than offset.
6. Add focused integration tests for each exact invariant, then run:

```bash
cd backend
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$PWD:/src" -w /src golang:1.24 go test ./internal/repository ./internal/service -count=1
```

Expected: `ok` for every tested package.

Commit: `feat: complete inventory lifecycle API`.

### 8. Replace hardcoded frontend data with RTK Query data flow

1. Delete `demoItems` and the `useMemo` search in `frontend/src/App.tsx` only after API hooks/render states are ready.
2. Create exact feature files:

```text
frontend/src/features/auth/AuthGate.tsx
frontend/src/features/spaces/SpaceSelector.tsx
frontend/src/features/items/ItemGrid.tsx
frontend/src/features/items/ItemCard.tsx
frontend/src/features/items/ItemFormDialog.tsx
frontend/src/features/boxes/BoxGrid.tsx
frontend/src/features/locations/LocationTree.tsx
frontend/src/features/search/SearchPage.tsx
frontend/src/features/media/MediaUploader.tsx
frontend/src/features/timeline/Timeline.tsx
frontend/src/features/ui/uiSlice.ts
frontend/src/routes/AppRouter.tsx
```

3. Update `frontend/src/api/api.ts` to use generated OpenAPI types. Every API request uses `fetchBaseQuery({ baseUrl: '/api/v1/', credentials: 'include' })`; do not use `fetch`, Axios, `localStorage`, mock arrays or hardcoded authenticated entities outside tests.
4. Implement `AuthGate`: call `GET /auth/me`; show login/register only for `401`; only render app routes after a valid session. Registration UI must respect `GET /auth/registration-status` (add this read-only endpoint) and not display a registration link when false.
5. Implement `SpaceSelector` using `useSpacesQuery`, persist only `selectedSpaceId` in `uiSlice`/`sessionStorage`, and validate selected ID is still in the current `spaces` response.
6. Implement item and search views using `useItemsQuery` and `useSearchQuery`. Each item card gets picture source only from `item.coverMedia.url`:

```tsx
{item.coverMedia ? (
  <img src={item.coverMedia.url} alt={item.name} loading="lazy" />
) : (
  <div aria-label={`Нет фото: ${item.name}`} className="image-placeholder" />
)}
```

No frontend source file may contain `MINIO_`, `objectKey`, `http://minio`, `https://minio`, `presigned`, or a hardcoded demo item.
7. Add tests first:
   - `frontend/src/features/items/ItemGrid.test.tsx`: MSW returns `/api/v1/spaces/space-1/items`; rendered item is API result, not demo data.
   - `frontend/src/features/media/MediaUploader.test.tsx`: submits multipart to `/api/v1/media`, uses returned `/api/v1/media/{id}` in subsequent item mutation.
   - `frontend/src/features/auth/AuthGate.test.tsx`: 401 shows login; authenticated response renders routes.
   - `frontend/src/api/no-storage-leak.test.ts`: assert no response/UI contains a MinIO endpoint or object key.
8. Run red/green per file:

```bash
cd frontend
pnpm test -- ItemGrid --runInBand
pnpm test -- MediaUploader --runInBand
pnpm test -- AuthGate --runInBand
pnpm test
pnpm typecheck
pnpm build
```

Expected: the first focused command fails before each component/hook exists, then passes; final suite/build exit `0`.

Commit: `feat: render inventory exclusively from API data`.

### 9. Build usable CRUD, history, archive and responsive PWA UI

1. Wire the permanent `+` action to `ItemFormDialog`, `BoxFormDialog`, `LocationFormDialog`, and tag creation. It must call mutations and invalidate RTK Query tags; it must not merely open inert buttons.
2. `ItemFormDialog` must disable submit until upload returns at least one media ID; display API field errors; set first photo cover by default.
3. Add pages/routes for item detail, box detail (including contents/preview media), location tree/detail, space members/settings, timeline, archive/trash and restore. Use `GET /timeline` for history, never compute history client-side from current fields.
4. Implement theme persistence via `localStorage` only for non-sensitive `theme` UI preference. Do not persist profile, sessions, API data, credentials or MinIO URLs.
5. Add `vite-plugin-pwa` (or explicit service-worker build) to cache static assets only; API responses and `/api/v1/media/*` must use network and must not be stored by the service worker. Add offline fallback text: “Для просмотра актуальных данных нужна сеть.”
6. Add tests first using Testing Library/MSW for dialog disabled state, mutation error, archive restore and dark/light persistence. Add Playwright specs under `frontend/e2e/` for: register → create space → upload photo → create item → search → inspect image requested from `/api/v1/media/` → archive → restore.
7. Verification:

```bash
cd frontend
pnpm test
pnpm exec playwright test
pnpm build
```

Expected: all specs pass at 360px and desktop viewport; browser request assertions show `/api/v1/media/` and no MinIO host.

Commit: `feat: complete inventory management PWA`.

### 10. Production hardening, CI and acceptance evidence

1. Split Compose files:
   - `docker-compose.yml`: production-like internal Postgres/MinIO, API, web on `8000:80`; no secrets hardcoded except explicit development defaults loaded from `.env`.
   - `docker-compose.dev.yml`: optional MinIO console and bind mounts.
2. Pin all container images by immutable version/digest. Add API `GET /ready` that checks PostgreSQL and MinIO bucket connectivity; web nginx should proxy `/health` and `/ready` to API, rather than returning SPA HTML.
3. Add `.github/workflows/ci.yml` using pinned action SHAs, least permissions, concurrency and timeout. Jobs:
   - backend: `go test ./...`, `go vet ./...` in Go container;
   - frontend: `pnpm install --frozen-lockfile`, `pnpm check:api`, `pnpm test`, `pnpm typecheck`, `pnpm build`;
   - compose: `hermes verify --json` or equivalent `docker compose up` readiness check.
4. Add `README.md` architecture diagram/text, exact start/test commands, all environment variables, security model for private images, backup/restore commands, and statement that MinIO must not be Internet-exposed.
5. Add `docs/backup-restore.md` with a tested PostgreSQL `pg_dump`/`pg_restore` and MinIO `mc mirror` procedure. Do not claim backups are automatic unless a scheduled job is actually deployed.
6. Run quality checks and inspect outputs before each final commit:

```bash
git diff --check
cd backend && docker run --rm -v "$PWD:/src" -w /src golang:1.24 sh -c 'go test ./... && go vet ./...'
cd ../frontend && pnpm check:api && pnpm test && pnpm typecheck && pnpm build
cd .. && hermes verify --json
git status --short --branch
```

Expected: all commands exit `0`, verification reports readiness `200`, and working tree is clean after commit.

7. Before merging/pushing, obtain an independent code review focused on: authz scope filtering, IDOR access to `/media/{id}`, multipart size/type validation, transactional audit/history writes, absence of MinIO URLs in DTOs, and removed demo data.

Commit: `chore: add production verification and CI`.

## Tests / validation

Apply strict TDD for every behaviour change:

1. Add the narrowest failing test in the same feature package.
2. Run only that test with `-count=1` (Go) or test name (Vitest) and record the expected failure: missing behaviour, incorrect status, or assertion mismatch — never a compilation/setup accident.
3. Add only enough production code to make it pass.
4. Re-run the focused test, then the package suite, then the full backend/frontend command for that layer.
5. Commit the green vertical slice before beginning an unrelated slice.

Required end-state acceptance checks:

- A visitor cannot view `/api/v1/spaces`, `/api/v1/items`, `/api/v1/media/{id}`, or a timeline.
- A member of Space A cannot access data or media belonging to Space B by UUID manipulation.
- The UI contains no hardcoded inventory records and initial data comes from `/api/v1` requests.
- Browser network trace for a photo contains `/api/v1/media/{uuid}` and contains no request to MinIO, object-storage hostname, direct bucket path, or presigned URL.
- A new active item cannot be created without at least one successfully uploaded and validated image.
- Update/move/assign/archive/restore result in correct entity state plus immutable audit event, and moves/assignments additionally have specialised history records.
- `docker compose up --build`, backend tests/vet, frontend tests/typecheck/build, Playwright, and `hermes verify --json` all exit `0`.

## Risks, tradeoffs, and open questions

- **API-streamed images vs presigned URLs:** API streaming is less efficient but directly satisfies “картинки не напрямую”, makes authorisation revocation immediate and prevents bucket URL leaks. Add CDN/signed URL only after the user explicitly changes this privacy requirement.
- **Large uploads:** Proxying 15 MiB through API raises API bandwidth/CPU usage. The hard limit, image type policy and no-HEIC-until-conversion choice keep this bounded.
- **MinIO availability:** Synchronous DB/object writes require compensating delete on DB failure. If durable retry is needed, add an outbox in a dedicated future slice; do not claim atomic cross-system transactions.
- **Current Compose secrets:** development passwords are currently in source. Replace them with `.env` defaults and document that production must use a secret store before deploying publicly.
- **Registration setting ownership:** global registration requires a system-admin policy independent of storage-space owner. This plan uses `SYSTEM_ADMIN_USERNAMES`; confirm a different identity source if desired.
- **Username/email:** current model makes email optional, so genuine emailed password reset is intentionally outside MVP. Confirm whether email must become mandatory before implementing email-based recovery.
- **Initial repository history:** a previous commit included generated dependency files before they were removed in a subsequent commit. Decide whether repository history should be rewritten with `git filter-repo`; rewriting requires coordinated force-push and must not happen without explicit approval.
