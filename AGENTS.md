# AGENTS.md

Guidance for coding agents (and people) changing locci. Read it before you
change anything; the docs it links are the detail.

## What this is

One Go binary, `locci`, that is three things:

- **`locci serve`**: the API server (Gin, `/api/v1`) with the SvelteKit web UI
  embedded via `go:embed`, and an optional RabbitMQ deploy-job consumer.
- **The CLI** (Cobra): `locci ps`, `locci run`, `locci env ...`, talking to a
  server's API.
- **`locci agent`**: runs on a Docker host behind NAT and dials out to the
  server, which then reaches that host's Docker through the tunnel.

It manages several Docker hosts ("environments": local socket, `tcp+tls`,
`ssh`, `agent`), with users, API tokens, OIDC single sign-on and an audit
log in SQLite (or remote libsql/Turso).

## Layout

```
cmd/locci            main
internal/cli         Cobra commands (serve, agent, user, auth, and the client commands)
internal/client      HTTP client the CLI uses (rewrites paths to /environments/<env>/...)
internal/config      environment variables
internal/docker      Docker client wrapper, connection kinds, the plain-user policy checks
internal/env         environment registry: connections, health loop
internal/jobs        RabbitMQ deploy consumer
internal/secret      AES-GCM sealing with LOCCI_SECRET_KEY
internal/server      HTTP API: auth, sessions, rate limits, proxies, policy, websockets
internal/sso         OIDC (code flow with PKCE, device flow); sso/ssotest is a fake provider
internal/store       SQLite / libsql: users, tokens, environments, access, OIDC, audit
internal/tunnel      agent tunnel (yamux over a WebSocket)
internal/ui          embeds web/ build output from internal/ui/dist
web/                 SvelteKit (Svelte 5 runes), shadcn-svelte, Tailwind v4, Bun
docs/                user and operator docs; docs/oidc/ per identity provider
```

## Commands

`just --list` shows everything. The ones you need:

| Task | Command |
|---|---|
| first-time setup | `just setup` |
| API server from source | `just dev` (restart it after Go changes: `go run` doesn't reload) |
| web UI with hot reload | `just dev-web` (Vite proxies `/api` to `just dev`) |
| vet and test | `just test` (or `CGO_ENABLED=0 go test ./...`) |
| web type check | `cd web && bun run check` |
| build the UI, then the binary | `just all` |
| Docker image | `just image`, then `just compose-up` |
| vulnerability scan | `./scripts/govulncheck.sh` |

## Before you call a change done

1. `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass.
2. `cd web && bun run check` has no errors (two existing warnings in
   `DataTable.svelte` are known).
3. `./scripts/govulncheck.sh` passes.
4. For behaviour a user sees, run it: the server against a real Docker
   daemon and the UI in a browser. Tests use a Docker endpoint that doesn't
   answer, so they don't prove Docker calls work.
5. Update `CHANGELOG.md` under `## [Unreleased]` (Added, Changed, Fixed,
   Security) and tick or add items in `BACKLOG.md`.
6. Say what you didn't verify.

## Rules

**Compatibility**
- The original v1 routes (`POST/GET /api/v1/networks`,
  `POST/GET /api/v1/containers`, `POST /api/v1/containers/start`,
  `DELETE /api/v1/containers`) keep their paths and payloads and act on the
  default environment.
- Existing databases must open after an upgrade. New columns go in
  `addedColumns` in `internal/store/store.go`, with a follow-up statement if
  existing rows need a value.

**Security**
- Secrets (TLS and SSH keys, OIDC client secrets) are sealed with
  `LOCCI_SECRET_KEY` before they're stored and are never returned by the API.
- Never log tokens. Request logs record the path only, never the query.
- Plain users must not reach the host: no host paths in containers, no
  `traefik.*` or `io.locci.*` labels, no changes to or console in protected
  containers (`docker.ProtectedReason`). Admins aren't limited. See
  [docs/SECURITY.md](docs/SECURITY.md).
- The web UI's sign-in is an HttpOnly cookie. With the cookie, state-changing
  requests need the `X-Locci-CSRF` header; `web/src/lib/api.js` sends it.
  Don't put tokens in `localStorage` or URLs.
- Anything that depends on the client's address or the public URL goes
  through `fromTrustedProxy` / `baseURL` / `LOCCI_PUBLIC_URL`, never raw
  `X-Forwarded-*` headers. See [docs/REVERSE_PROXY.md](docs/REVERSE_PROXY.md).

**Go**
- Go version: the `go` line in `go.mod`. The binary is pure Go
  (`CGO_ENABLED=0`): don't add cgo dependencies; it would break the static
  cross-compiled releases.
- Keep it simple: the standard library first, small packages, no frameworks
  beyond what's here.
- Don't hard-code the project name anywhere new (the Justfile has `bin` and
  `image` variables).

**Web UI**
- Svelte 5 runes, shadcn-svelte components (no plain HTML `<select>`),
  Lucide icons (`@lucide/svelte`), sonner toasts (bottom right), skeletons
  while loading, `cursor-pointer` on buttons.
- Listings use `DataTable.svelte` (10 rows per page by default).
- Forms open in modals (`FormDialog.svelte`), except creating a container,
  which has its own page.
- Container actions live in the options menu (`ContainerActions.svelte`).
- Build output goes to `internal/ui/dist`. A web build deletes
  `internal/ui/dist/.gitkeep`; restore it (`git checkout internal/ui/dist/.gitkeep`)
  so `go:embed` keeps working on a fresh clone.

**Writing**
- No em or en dashes and no curly quotes anywhere: code, comments, docs,
  UI copy, commit messages. Use a period, comma, colon or parentheses.
- Docs say what was tested end to end and what wasn't.

**Git**
- Conventional Commits, one line, no body:
  `feat(server): ...`, `fix(auth): ...`, `docs: ...`. Explanations belong in
  `CHANGELOG.md` or `BACKLOG.md`.
- One logical change per commit.
- No AI attribution in commits or pull requests (no `Co-Authored-By`
  trailers, no "Generated with" notes).
- Never push, tag, force-push or rewrite history without the maintainer
  asking.
- Ask before touching uncommitted changes you didn't make.

## Where things go

| Adding | Put it in |
|---|---|
| an API route | `internal/server/server.go` (`Router`, or `resourceRoutes` for Docker resources so it exists for the default and every `/environments/:env`); handlers next to their kind (`containers.go`, `environments.go`, ...) |
| a Docker operation | `internal/docker` (`ops.go`), then a handler that uses `dk(c)` |
| a table | `schema` in `internal/store/store.go` (`CREATE TABLE IF NOT EXISTS`) |
| a column on an existing table | `addedColumns` in `internal/store/store.go`, with a follow-up statement for existing rows if they need a value |
| a timestamp you read from the database | scan it through `stamp{&field}` (`internal/store/stamp.go`), so the API always returns RFC 3339 |
| a setting | `internal/config/config.go`, wired in `internal/cli/serve.go`, documented in `docs/CONFIGURATION.md` |
| a CLI command | `internal/cli`, registered in `root.go`; it inherits `-E/--environment` and `-o json` |
| a page or dialog | `web/src/routes/...` and `web/src/lib/components/`; call the API through `web/src/lib/api.js` (`get`, `post`, `del`, `api`, `stream`), which adds the CSRF header and environment scoping |

## Testing

- Server tests start a real router with `newTestServer(t)`
  (`internal/server/server_test.go`): a temporary SQLite file, the static
  token `"static"` as an admin, and a `local` environment pointing at a
  Docker endpoint that doesn't answer. Use `call(t, method, url, token, body)`
  for bearer requests and `cookieCall` (`session_test.go`) for the browser's
  cookie and CSRF header.
- Docker being down in tests is deliberate: assert on the status codes that
  come before Docker (400, 403, 404, 409); anything that needs Docker gets a
  5xx there. Exercise real Docker by running the server.
- OIDC tests use the fake provider in `internal/sso/ssotest`.
- The login rate limiter counts per client IP. A test that fails logins
  many times uses up `127.0.0.1`'s allowance for that test server, so use
  a fresh server per test, or set `X-Forwarded-For` (loopback is a trusted
  proxy) to vary the client address.
- Behind a proxy: a quick bed is NGINX and Traefik containers forwarding to
  `host.docker.internal:<port>`, and Dex for OIDC (`docs/oidc/DEX.md` has a
  scripted setup). Remove such containers when done.

## Gotchas

- `go run` (`just dev`) doesn't reload. After Go changes restart it, or the
  web UI talks to an old server and new routes answer 404.
- Patched Go releases refuse to link C libraries that carry LTO bitcode;
  that's why the project dropped go-libsql for pure-Go SQLite. Don't bring
  cgo back.
- On Colima, bind mounts from outside `$HOME` show up empty in containers;
  copy files in with `docker cp` instead. The socket is
  `unix://$HOME/.colima/default/docker.sock`.
- With Colima, `docker build` can keep running after it prints
  `naming to docker.io/... done`. The image is ready at that point; stop the
  client.
- `.env` is written for running on the host (`DATABASE_URL=file:locci.db`,
  a host `DOCKER_HOST`). `compose.yaml` and `just up` override those for the
  container; keep it that way.
- The `local` environment is created on first start with an empty URL,
  which means "`DOCKER_HOST` at startup, else the local socket". Giving it
  a URL (`locci env edit local --url ...`) makes it ignore `DOCKER_HOST`.
- The release workflow needs the `DOCKER_USERNAME`, `DOCKER_TOKEN`,
  `RELEASE_GPG_KEY` and `RELEASE_GPG_PASSPHRASE` secrets
  ([docs/RELEASING.md](docs/RELEASING.md)).

## Where to read more

| Topic | Doc |
|---|---|
| using the UI and CLI | [docs/USAGE.md](docs/USAGE.md), [docs/CLI.md](docs/CLI.md) |
| the HTTP API | [docs/API.md](docs/API.md) |
| settings | [docs/CONFIGURATION.md](docs/CONFIGURATION.md) |
| Docker hosts and agents | [docs/ENVIRONMENTS.md](docs/ENVIRONMENTS.md) |
| single sign-on | [docs/OIDC.md](docs/OIDC.md), [docs/oidc/](docs/oidc/) |
| security model | [docs/SECURITY.md](docs/SECURITY.md) |
| proxies and tunnels | [docs/REVERSE_PROXY.md](docs/REVERSE_PROXY.md) |
| running in Docker | [docs/DOCKER.md](docs/DOCKER.md) |
| development | [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) |
| releases | [docs/RELEASING.md](docs/RELEASING.md) |
| what's next | [BACKLOG.md](BACKLOG.md) |
