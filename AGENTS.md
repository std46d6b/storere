# AGENTS.md

## Frontend

Run frontend commands from `frontend/` with `pnpm`.

## Before every commit

1. Run `pnpm format` before staging changes.
2. Run the relevant checks: at minimum `pnpm test`, `pnpm typecheck`, and `pnpm build` for frontend changes.
3. Do not commit generated artifacts, credentials, or `.hermes/deployment.logs`.

## Completion

After completing and verifying a task, create a Conventional Commit and push it directly to `origin/main`. Confirm that the remote branch points at the pushed commit before reporting completion.
