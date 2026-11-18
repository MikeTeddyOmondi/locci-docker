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
- **compose.yaml** and `just compose-up` / `compose-down` / `compose-logs`:
  builds the image from source and runs it with the Docker socket, the
  `locci-data` volume and an optional `.env`.
- **Justfile** grouped into setup, develop, build, cli, docker and project
  recipes, with a comment on every command: `just cli <args>` runs any CLI
  command from source, and project recipes cover `status`, `branch`, `commit`
  (runs tests first), `push`, `changes`, `release` and `rename` (rewrites the
  Go module path).
- **Tests** for the store, the auth flow and Docker-down behaviour.
- **Documentation** in `docs/`: usage, environments (preparing TLS and SSH
  hosts), CLI, API, Docker, configuration, security, OIDC (planned) and
  development guides. The README links to them and its quick start now uses
  `just setup` and Bun.
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
  - `docker.NewWithOptions` reaches `tcp+tls` hosts with in-memory PEM
    certificates and `ssh` hosts through `golang.org/x/crypto/ssh` (no `ssh`
    binary or `~/.ssh` needed, so it works inside the container image). SSH
    forwards to the remote Docker socket (`/var/run/docker.sock`, or a path in
    the URL such as `ssh://me@host/run/user/1000/docker.sock` for rootless
    Docker), pins the host key on first connect and refuses a changed key, and
    redials a dropped connection. Keys must be unencrypted (deploy keys).
    Once a key is pinned only its type is negotiated, so a server with several
    host keys can't trip the check by offering another one. The remote sshd
    must allow `AllowStreamLocalForwarding` (and `AllowTcpForwarding`); the
    error says so when it refuses. Fingerprints read `<type> SHA256:...`, like
    `ssh-keygen -lf`.
    An unreachable SSH host reports the SSH error (for example "connection
    refused") instead of Docker's generic message about a placeholder host.
  - `internal/env` registry: one cached `docker.Service` per environment,
    connected on first use. A cached client remembers a hash of the settings it
    was built from, so editing a host rebuilds it automatically. A health loop
    pings every host concurrently, records `up`/`down` and `last_seen_at`,
    drops the client of a host that fails (the next request reconnects) and
    pins an SSH host key the first time one is seen.
  - Every Docker route also exists under `/api/v1/environments/:env/...`
    (containers, logs, stats, exec and the exec WebSocket, images, networks,
    volumes, `system/info`, plus `/environments/:env/info`). Middleware picks
    the environment's client for each request. The original routes, including
    the four legacy container routes, keep their paths and payloads and use
    the default environment; `/health` checks the default environment.
    `locci serve` starts the registry and its 30s health loop.
  - Environment API. Anyone signed in can `GET /environments` and
    `GET /environments/:env`; admins can `POST /environments`,
    `PATCH`/`DELETE /environments/:env`, `POST /environments/:env/default`
    and `POST /environments/test`, which connects to unsaved settings and
    returns the Docker version, OS, CPUs, memory and (for SSH) the host key
    fingerprint. TLS and SSH keys are write-only: stored encrypted, answered
    only as `has_secret`. Creating an SSH host pins its key right away when
    reachable; changing its URL clears the pin. Saving a key without
    `LOCCI_SECRET_KEY` returns 412 with setup instructions. The default can't
    be deleted (409) and an environment's kind can't change. The name `test`
    is reserved.
    When the test body names an existing environment, blank certificates and
    key are taken from the stored one (server side only) and, for the same
    URL, its pinned SSH host key is enforced, so edits can be tested without
    re-entering secrets.
  - Deploy jobs accept an optional `deployConfig.host` naming the environment
    to deploy to; without it they use the current default, as before. The
    existing `environment` field is untouched, since senders already use it
    for their own stage names.
  - CLI: `locci env ls|add|rm|test|use|default`. `env add` tests the
    connection first (refusing an unreachable host unless `--no-test`), shows
    the Docker version and SSH fingerprint, and pins exactly that key. Every
    command takes `-E/--environment` (not `--env`, which `run` uses for
    container variables), then `LOCCI_ENV`, then the choice saved by
    `locci env use` (`--clear` follows the server default again). The client
    rewrites Docker resource paths to `/environments/<env>/...`, so existing
    commands work against any host unchanged.
    `locci env edit NAME` changes only the flags given (`--label`, `--url`,
    `--tls-*`, `--ssh-key`, `--reset-host-key`), tests the merged settings
    with the stored key first and pins the key it showed.
  - Web UI: the fleet pages live under `/e/<env>/...` (dashboard,
    containers, images, networks, volumes, logs, stats, console), and the API
    client scopes Docker calls (including the terminal WebSocket) to that
    environment. Old URLs such as `/containers/<id>` redirect to the last-used
    or default environment, and an unknown environment shows a clear empty
    state. The sidebar has an environment switcher with live status dots
    that keeps you in the same section. A new Environments page shows a card
    per host (status, Docker version, CPUs, memory, running containers, OS)
    with admin actions to set the default, check now or remove, and an Add
    dialog per connection kind with **Test connection**, PEM file loading and
    the SSH fingerprint to compare before it is pinned. **Edit** reuses the
    dialog: name and kind are fixed, certificates are prefilled, a blank key
    keeps the stored one, Test connection works without re-entering it, and an
    SSH host's pinned key can be forgotten (it is dropped automatically when
    the URL changes).
  - Agent environments: a fourth kind, `agent`, for hosts
    that dial out to the server. Each has an agent token (`la_...`), stored
    only as a hash in `agent_tokens`; issuing a new one replaces the old.
    `internal/tunnel` carries Docker connections over the agent's one
    outbound WebSocket: yamux (`hashicorp/yamux`) multiplexes it, the
    server's hub opens a stream per Docker connection, and the agent pipes
    each stream into its local Docker socket (half-closing so exec and attach
    end cleanly). A new agent for the same environment replaces the old one.
    `GET /api/v1/agent/connect` takes the agent token as a Bearer header,
    upgrades to the WebSocket, tells the agent its environment name
    (`X-Locci-Environment`) and attaches the tunnel; connects and disconnects
    are audited. Creating an agent environment returns its token once;
    `PATCH {"rotate_agent_token": true}` issues a new one and drops the
    connected agent; deleting the environment drops it too. Environments of
    this kind report `agent_connected` live, and requests while no agent is
    connected say so instead of a generic Docker error (custom dialers now
    report their real error, as SSH already did).
    CLI: `locci agent --server URL` (token from `LOCCI_AGENT_TOKEN`, so it
    stays out of process lists) dials out, serves the local Docker
    (`--docker-host`, default `DOCKER_HOST` or the local socket), reconnects
    with backoff and stops for good when its token is refused.
    `locci env add NAME --kind agent` prints the token and the commands to
    start the agent; `locci env edit NAME --rotate-agent-token` replaces it.
    UI: an "Agent (outbound)" kind in the Add dialog (no address or keys),
    a one-time dialog with the token and the commands to start the agent
    (binary and `docker run`), cards that say whether the agent is
    connected, and a "Rotate agent token" action. Tested end to end with
    `locci agent` running as its own process against the local Docker:
    containers, logs, stats, the browser console and exec all work through
    the tunnel, and a rotated token disconnects the agent, which then stops.

- **Single sign-on with OIDC (in progress).**
  - Tables `oidc_providers` (client secret encrypted like environment keys),
    `user_identities` (provider subject to user, unique per provider) and
    `settings`. SSO-only users keep `password_hash = ''` because SQLite can't
    drop the `NOT NULL` in place; password login refuses an empty hash, and
    `locci user passwd` can give such a user a password later. Users report
    `has_password`. Password login stays on by default.
  - `internal/sso` (`go-oidc` v3, `x/oauth2`): authorization code flow with
    PKCE. State, nonce and verifier travel in a cookie sealed with
    `LOCCI_SECRET_KEY` and expire after 10 minutes; the ID token's issuer,
    audience, expiry and nonce are verified, and email or groups missing from
    it are read from userinfo. Users are found by linked identity, else by an
    existing username equal to the verified email (then linked), else created
    when the provider allows it (username: verified email, preferred username
    or `<provider>-<subject>`, with a suffix on clashes). An allowed-domains
    list requires a verified email in one of them. With an admin group set,
    the role follows membership on every sign-in. `internal/sso/ssotest` is a
    fake provider (discovery, JWKS, PKCE-checking token endpoint, userinfo)
    for tests.
  - Routes. Public: `GET /auth/providers` (enabled providers and whether
    password login is on), `GET /auth/oidc/:provider/start` and
    `.../callback`, which answer 404 for providers that aren't enabled. The
    browser lands on `/login#token=...` (a fragment, so the token never
    reaches server logs); the CLI's loopback listener gets
    `http://127.0.0.1:<port>/callback?token=...`. Sign-ins and failures go to
    the audit log as `LOGIN`. Admin: `/sso/providers` (list, create, update,
    delete, `test` runs discovery) with the redirect URI to register, and
    `/sso/settings` to turn password login off, which is refused while no
    provider is enabled; the last enabled provider can't be disabled or
    removed while it is off. `LOCCI_PUBLIC_URL` fixes the base URL used in
    redirect URIs; otherwise it comes from the request (`X-Forwarded-*`
    aware).
  - CLI: `locci login --sso <provider>` listens on 127.0.0.1, opens the
    browser (printing the URL too) and saves the token the server sends back.
    `locci auth password-login on|off` writes the setting straight to the
    database on the server host, as break-glass access when SSO is broken.
  - Device flow (RFC 8628) for machines without a browser:
    `POST /auth/oidc/:provider/device` returns the user code and verification
    URL; `POST .../device/token` with the returned handle answers `pending`,
    `slow_down` or a token. The server talks to the provider (it holds the
    client secret, sent as `client_secret_basic` on both requests); the CLI
    only gets the user code and a handle sealed with `LOCCI_SECRET_KEY`, so
    the device code never leaves the server and can't be used with another
    provider. Users are matched exactly as in the browser flow, and sign-ins
    are audited as `LOGIN`. Providers without a
    `device_authorization_endpoint` get a clear error.
    The device request carries PKCE (S256) and the poll its verifier, kept
    in the sealed handle: Keycloak requires it when the client enforces PKCE.
    Tested end to end against Keycloak with `locci login --device`.
  - A groups claim shaped as an object counts its keys as the group names,
    so Zitadel's `urn:zitadel:iam:org:project:roles` works as the groups
    claim; Entra ID's `authorization_declined` counts as a denied device
    sign-in.
  - CLI: `locci login --sso <provider> --device` prints the verification URL
    and code (and the one-click URL when the provider gives one), then polls
    at the provider's interval, backing off on `slow_down`, until approval,
    denial or expiry.
  - Login page: a "Continue with ..." button per enabled provider, the
    password form only while password login is on, and pickup of the token
    (or error) from the URL fragment, which is then cleared. Users signed up
    through SSO carry an "SSO" badge on the Users page.
  - Sign-in page (admin, under Harbour master): the password-login switch
    (disabled until a provider is enabled), a card per provider with an
    enable switch, its rules and the redirect URI to copy, and an add/edit
    dialog with **Discover** to check the issuer before saving. A blank
    secret on edit keeps the stored one. Tested end to end against Dex in
    the browser and with `locci login --sso`.
  - Docs: `docs/OIDC.md` (defaults, user matching, troubleshooting) and a
    guide per self-hosted provider in `docs/oidc/` (Keycloak, Authentik,
    Pocket ID, Dex), each with manual and scripted setup. All four were
    tested end to end in a browser: Keycloak (group to admin role and back),
    Authentik (needs `authorization_code` in the provider's grant types and a
    verified-email mapping for domain lists), Pocket ID (needs the `groups`
    scope; emails arrive unverified) and Dex (web and `locci login --sso`).
  - Guides for hosted providers in `docs/oidc/`: Google, Microsoft Entra
    ID, Zitadel and GitLab, with scripted setups where the provider has a
    CLI or API for it (Entra via `az`, GitLab via its API). They follow each
    provider's documentation and say so; they weren't tested end to end.

### Changed

- The Docker image is now `locci/docker` (Justfile `image`), and the docs, the
  agent setup dialog and `locci env add --kind agent` use that name.
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

- `just up` passed `.env` into the container unchanged, so a host-style
  `DATABASE_URL=file:locci.db` put the database outside the `/data` volume and
  a host `DOCKER_HOST` (such as a Colima socket path) left `local` down. It
  now always sets `DATABASE_URL`, `DOCKER_HOST`, `SERVER_PORT` and `GIN_MODE`
  for the container.
- Web UI pages such as `/login` were served with HTTP status 404 (gin presets
  it for unmatched routes) although the page itself loaded; they now return
  200. Unknown `/api/` paths still answer a JSON 404.
- Concurrent writes to a local SQLite database (audit log entries, health
  checks) could fail with `database is locked`. A `file:` database now uses a
  single connection so writes queue instead of failing.
- Deploy jobs that hit the retry limit were acked and then republished anyway,
  so they retried forever and double-acked the message.
- Creating a container failed whenever the pull failed, even if the image
  existed locally; it now falls back to the local image.
- The exec WebSocket started the command before the origin check, so a
  rejected cross-site request could still run it. The origin is now checked
  first.
- `svelte-kit sync` now runs before `dev`/`build` (and on install), fixing the
  "Cannot find base config file ./.svelte-kit/tsconfig.json" warning.

### Security

- The HTTP server has a header read timeout, an idle timeout and a 64 KiB
  header limit; request bodies are capped at 1 MiB (413 above that).
- `locci serve` shuts down gracefully on SIGINT/SIGTERM: it stops taking
  requests and waits up to 25 seconds for running ones and a running deploy
  job (`compose.yaml` and `just up` give the container 30 seconds).
- Every response carries a Content-Security-Policy (own scripts only, plus the
  SvelteKit bootstrap by hash; no framing), `X-Frame-Options: DENY`,
  `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, and HSTS
  when served over HTTPS.
- `X-Forwarded-For`, `-Proto` and `-Host` count only from trusted proxies
  (`LOCCI_TRUSTED_PROXIES`, default loopback and private networks), so a
  client can't forge its IP in logs or the scheme and host of OIDC redirects.
- `?token=` is accepted only on WebSocket requests (the console); other URLs
  no longer carry tokens into browser history and proxy logs.
- `AUTH_TOKEN` is compared in constant time, and a login for an unknown user
  takes as long as a wrong password, so timing doesn't reveal usernames.
- Password login is rate limited: after 10 failures from one IP, or 20 for one
  username from anywhere, within 15 minutes, logins answer 429 with
  `Retry-After` until the window ends. The SSO device-flow endpoints allow 30
  requests a minute per IP. Failed and refused logins are in the audit log.
- API tokens expire after `LOCCI_TOKEN_TTL_DAYS` (default 30) without use;
  each use pushes the date back, and tokens from before the upgrade start
  their 30 days at the upgrade. `GET /auth/tokens` and
  `DELETE /auth/tokens/:id`, `locci sessions [revoke ID]` and a Sessions
  dialog in the web UI's user menu list and sign out your own sessions.
- Plain users can no longer reach the hosts through Docker: new containers
  get named volumes only (no host paths), `traefik.*` and `io.locci.*` labels
  are admin-only, a subdomain another container routes can't be taken, and
  protected containers (labelled `io.locci.protected=true`, privileged, or
  with the Docker socket or `/` mounted) can't be changed or opened in a
  console, and their environment is hidden when inspected. The image carries
  the label, so the server and agents are protected. Admins are not limited;
  `LOCCI_USER_HOST_ACCESS=true` restores the old behaviour for everyone.
- A container's subdomain must be one DNS label (lowercase letters, digits,
  dashes): it is part of Traefik's label keys, where a dot or backtick could
  change other routes.

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
