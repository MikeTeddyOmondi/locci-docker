# Development

## Tools

- Go 1.27+ (pure Go: no C compiler, `CGO_ENABLED=0`)
- [Bun](https://bun.sh) for the web UI (the lockfile is `web/bun.lock`)
- [just](https://just.systems) for the recipes below
- Docker, to have something to manage

`just doctor` checks them; `just setup` creates `.env`, downloads Go modules
and installs the web packages.

## Running from source

```sh
just dev          # API server from source on :8998 (reads .env)
just dev-web      # second terminal: Vite with hot reload, proxying /api to :8998
just cli ps       # any CLI command from source
```

The UI build goes to `internal/ui/dist`, which `internal/ui` embeds with
`go:embed`; `just all` builds the UI and then the binary. Without a UI build
the server shows a short "UI not built" page, so `go build` works without
Bun. Keep `internal/ui/dist/.gitkeep` (the `web` recipe restores it).

## Layout

```
cmd/locci            main.go -> internal/cli
internal/cli         cobra commands (server + client)
internal/client      HTTP client used by the CLI (scopes paths to an environment)
internal/config      environment variables / .env
internal/docker      the only package that talks to Docker (local, tcp+tls, ssh)
internal/env         one Docker connection per environment, health loop
internal/jobs        RabbitMQ deploy consumer
internal/secret      AES-GCM for stored keys (LOCCI_SECRET_KEY)
internal/server      Gin router, handlers, auth, audit, exec WebSocket
internal/store       SQLite (modernc.org/sqlite) or remote libsql: users, tokens, environments, audit log
internal/ui          embeds internal/ui/dist
web/                 SvelteKit app (Svelte 5, shadcn-svelte, Tailwind v4)
docs/                these guides
```

Rules that keep it simple:

- Only `internal/docker` imports the Docker client.
- Handlers get the request's Docker connection from the context (`dk(c)`),
  set by the environment middleware; they never pick a host themselves.
- The schema is a list of `CREATE TABLE IF NOT EXISTS` statements in
  `internal/store/store.go`, run on every start. Add tables there; changes to
  existing columns need a migration.
- The original v1 routes and their payloads must not change.

## Web UI

- Pages for one Docker host live under `web/src/routes/e/[env]/`. Use
  `href('/containers')` from `$lib/env.svelte.js` for links so they stay in the
  current environment; `api.js` scopes Docker API calls automatically.
- Use the shared components (`DataTable`, `FormDialog`, `PageHeader`,
  `StatusBadge`, `EmptyState`, `ListSkeleton`) and shadcn-svelte components
  (`bunx shadcn-svelte@latest add <name>`).
- `cd web && bun run check` runs svelte-check.

## Tests

```sh
just test         # go vet + go test ./...
```

- Server tests use a temporary SQLite file and `httptest`; a Docker host at
  `tcp://127.0.0.1:1` stands in for "Docker is down", and a small fake Docker
  API (answering `/_ping` and a few endpoints) for a working one.
- `internal/docker` tests run a real TLS server with generated certificates
  and an in-process SSH server that forwards to a fake Docker socket.
- Run `go test -race` on packages with goroutines (`env`, `docker`, `server`).
- For features that touch Docker, also try them against the real daemon and a
  second host (see the Docker-in-Docker example in
  [ENVIRONMENTS.md](ENVIRONMENTS.md#preparing-a-tls-host)).

## Commits

- Conventional Commits, one line: `feat(env): ...`, `fix(server): ...`,
  `docs: ...`. One change per commit.
- Tick the task in `BACKLOG.md` and add it under `[Unreleased]` in
  `CHANGELOG.md` with the change.
- `just commit "msg"` runs the tests before committing.
