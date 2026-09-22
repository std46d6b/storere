# storere

PWA-каталог домашних вещей, коробок и мест хранения.

## Стек

- Go `net/http`, PostgreSQL (`pgx`), SQL migrations (`golang-migrate`)
- React + TypeScript + Redux Toolkit / RTK Query
- MinIO и Docker Compose
- Сессии в HTTP-only cookies, пароль — Argon2id

## Запуск

```bash
docker compose up --build
```

Откройте http://localhost:8080. PostgreSQL и MinIO запускаются в Docker; MinIO console доступна на http://localhost:9001.

## Проверки

```bash
docker run --rm -v "$PWD/backend:/src" -w /src golang:1.24 go test ./...
cd frontend && pnpm test && pnpm typecheck && pnpm build
```

## API

JSON REST API расположен под `/api/v1`: регистрация, сессии, пространства, места, коробки, перемещение коробок, вещи с требованием хотя бы одного media ID, поиск и история аудита.
