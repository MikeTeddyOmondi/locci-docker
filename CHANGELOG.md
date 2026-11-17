# Changelog

All notable changes to this project. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). There are no tagged
releases yet, so older entries are grouped by date. Planned work lives in
`BACKLOG.md`.

## [Unreleased]

The project grows from a single-file Docker API wrapper into one `locci` binary
that is an API server, a web UI and a CLI.

### Added

- **Single binary with a Cobra CLI.** `locci serve` runs the API (and the
  RabbitMQ deploy consumer when `RABBITMQ_URL` is set); `locci user` manages
  users on the server host.
- **libsql storage** (`internal/store`, `github.com/tursodatabase/go-libsql`).
  `DATABASE_URL` is a local file (`file:locci.db`) or a remote libsql/Turso
  database. Holds:
  - users with bcrypt passwords and an `admin`/`user` role
  - API tokens (random `lc_` tokens; only their SHA-256 hash is stored)
  - an audit log of every non-GET API request
- **Authentication.** `POST /api/v1/auth/login`, `GET /auth/me`,
  `POST /auth/logout`. Bearer tokens via the `Authorization` header, or
  `?token=` for WebSockets. `AUTH_TOKEN` is now optional and still works as a
  static admin token.
- **Admin API.** `GET/POST /users`, `DELETE /users/:name`, `GET /audit`.
- **Container API.** `GET/DELETE /containers/:id`,
  `POST /containers/:id/{start,stop,restart,kill,pause,unpause}`,
  `GET /containers/:id/logs` (plain-text stream, `follow` and `tail`),
  `GET /containers/:id/stats`, `POST /containers/:id/exec`, and
  `GET /containers/:id/exec/ws` for an interactive TTY over WebSocket.
- **Image, network, volume and system API.** List/pull/remove/prune images;
  inspect/remove networks; list/create/remove volumes; `GET /system/info`.
- **CLI client commands.** `login`, `logout`, `whoami`, `ps`, `run`, `rm`,
  `logs`, `inspect`, `stats`, `exec`, `start`, `stop`, `restart`, `kill`,
  `pause`, `unpause`, `image`, `network`, `volume`, `info`, `audit`, with
  `-o json` for scripting. The server URL and token are saved by `locci login`
  (`LOCCI_URL` / `LOCCI_TOKEN` override them).
- **Embedded web UI.** Svelte 5 + SvelteKit, built with `adapter-static` into
  `internal/ui/dist` and embedded with `go:embed`; served for every non-API
  route with SPA fallback. Pages: login, dashboard, containers (list, create,
  detail with overview, live logs, stats, xterm.js console, inspect), images,
  networks, volumes, users and audit log.
- **UI redesign with shadcn-svelte.** Tailwind CSS v4, shadcn-svelte
  components (bits-ui), Lucide icons, a light/dark theme toggle (mode-watcher),
  sonner toasts, reusable data tables, skeleton loaders, empty states,
  copyable IDs and confirmation prompts.
- **Modal forms** for pulling images and creating networks, volumes and users.
- **Three-container logo** used in the UI and as the favicon.
- **Build and tooling.** Multi-stage `Dockerfile` (web build → Go build with
  cgo → Debian slim runtime with the database on a `/data` volume);
  `.dockerignore`; `.env.sample` with `DATABASE_URL`; README with quick start,
  layout and the API route list.
- **Justfile** grouped into setup, develop, build, cli, docker and project
  recipes, with a comment on every command: `just cli <args>` runs any CLI
  command from source, and project recipes cover `status`, `branch`, `commit`
  (runs tests first), `push`, `changes`, `release` and `rename` (rewrites the
  Go module path).
- **Tests** for the store, the auth flow and Docker-down behaviour.
- **Environments (multiple Docker hosts), in progress.**
  - `environments` table (name, kind `local`/`tcp+tls`/`ssh`, url, TLS CA and
    cert, an encrypted secret for the TLS key or SSH key, pinned SSH host key,
    labels, default flag, health status). `locci serve` seeds a `local` default
    on first start whose empty URL means "DOCKER_HOST or the local socket", so
    existing installs behave as before. A partial unique index allows only one
    default, and the default can't be deleted.
  - `internal/secret`: AES-256-GCM encryption for stored secrets, keyed by
    `LOCCI_SECRET_KEY` (32 random bytes, base64). Without the key the server
    still runs; only saving a remote host's key is refused, with a message
    explaining how to generate one.

### Changed

- Code split from `cmd/main.go` into internal packages: `cli`, `client`,
  `config`, `docker`, `jobs`, `server`, `store`, `ui`. Entry point is now
  `cmd/locci`.
- Go module path renamed to `github.com/MikeTeddyOmondi/locci-docker`.
- The RabbitMQ consumer reuses the same Docker code as the HTTP API, so `create`
  jobs now pull the image and map ports like the API does.
- Logging uses `log/slog` directly; request logs record the path only, never
  the query string (which may carry a token).
- Docker "not found" / "conflict" errors map to HTTP 404 / 409 instead of 500.
- A missing `.env` file is no longer fatal.
- The web UI installs and builds with Bun; `bun.lock` is tracked.
- The original v1 routes (`/networks`, `/containers`, `/containers/start`,
  `DELETE /containers`) keep the same paths and payloads.

### Fixed

- Deploy jobs that hit the retry limit were acked and then republished anyway,
  so they retried forever and double-acked the message.
- Creating a container failed whenever the pull failed, even if the image
  existed locally; it now falls back to the local image.
- The exec WebSocket started the command before the origin check, so a
  rejected cross-site request could still run it. The origin is now checked
  first.
- `svelte-kit sync` now runs before `dev`/`build` (and on install), fixing the
  "Cannot find base config file ./.svelte-kit/tsconfig.json" warning.

### Notes

- go-libsql needs cgo: build with `CGO_ENABLED=1` on glibc.

## 2025-10-07: Pull images before creating containers

### Added

- Pull the image from its registry before creating a container
  (PR #4 by balagrivine).

## 2025-10-05: Refactor and structured logging

### Changed

- Split the large handlers into smaller methods and introduced structured
  logging (PR #1 by balagrivine).

## 2025-07-14 to 2025-08-04: Fixes and updates

### Changed

- Deploy jobs now arrive wrapped in a message envelope (`projectId`,
  `deployConfig`, `timestamp`), and jobs carry project and trigger metadata.
- Default queue renamed to `deploy.trigger`; default Traefik domain updated.
- Label merging simplified with `maps.Copy`; first attempt at pulling the image
  before container creation.
- Renamed `HTTP.rest` to `.http`.

## 2025-06-07: Initial setup

### Added

- Gin HTTP API over the Docker Engine SDK with bearer-token auth:
  `GET /health`, `POST/GET /api/v1/networks`, `POST/GET /api/v1/containers`,
  `POST /api/v1/containers/start`, `DELETE /api/v1/containers`.
- Traefik routing labels (`<subdomain>.<domain>`, TLS via Let's Encrypt) on
  created containers.
- RabbitMQ consumer for deploy jobs, with retries.
- Justfile, `.env.sample`, example HTTP requests and test scripts.
