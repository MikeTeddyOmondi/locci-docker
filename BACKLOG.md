# Backlog

Planned work, roughly in priority order. Each epic has the design and a task list;
tick items off as they land and move them into `CHANGELOG.md`.

---

## 1. Multiple Docker hosts ("environments")

Manage more than the local Docker host: add remote hosts, pick one in the UI/CLI,
and see the same dashboard, containers, images, networks, volumes, logs, stats
and console for it as for the local host today.

### Why it fits the current design

All Docker access already goes through one `internal/docker.Service` (a Docker
client + helpers). Multiple hosts means holding **one `docker.Service` per
environment** and picking the right one per request. Handlers, the CLI and the
UI pages stay the same; they just gain an environment parameter.

### Design

**Connection types**

| Kind | URL example | Notes |
|---|---|---|
| `local` | `unix:///var/run/docker.sock` | today's behaviour; created automatically as the default environment |
| `tcp+tls` | `tcp://10.0.0.5:2376` | Docker's TLS port with client certs (CA, cert, key) |
| `ssh` | `ssh://deploy@10.0.0.5` | uses Docker's SSH connection helper; no Docker port exposed |
| `agent` | (later) | a `locci agent` on the remote host dials out to the server over a WebSocket tunnel, so hosts behind NAT/CGNAT work without opening ports |

**Storage** (`internal/store`, new tables)

- `environments`: `id`, `name`, `kind`, `url`, `tls_ca`, `tls_cert`, `tls_key`
  (encrypted), `labels`, `is_default`, `created_at`, `last_seen_at`, `status`
- `environment_users` (later, for RBAC): `environment_id`, `user_id`, `role`

TLS keys and other secrets are encrypted at rest with AES-GCM using
`LOCCI_SECRET_KEY`, and the API never returns them.

**Server** (`internal/env`, new package)

- A small registry: `map[envID]*docker.Service`, connecting lazily on first use
  and reconnecting on error.
- A background loop pings each environment every 30s and stores
  `status` (`up`/`down`) and `last_seen_at` for the UI.
- Middleware resolves the environment for each request and puts its
  `docker.Service` on the gin context; handlers use that instead of the single
  `s.Docker`.

**API**

- `GET/POST /api/v1/environments`, `GET/PATCH/DELETE /api/v1/environments/:env`
- `POST /api/v1/environments/test` to check a connection before saving it
- `GET /api/v1/environments/:env/info` for host details (same shape as `/system/info`)
- All resource routes under `/api/v1/environments/:env/...`
  (`/containers`, `/images`, `/networks`, `/volumes`, logs, stats, exec, exec/ws)
- **Backward compatibility:** the current `/api/v1/containers`, `/networks`, ...
  routes keep working and use the default environment, so existing
  integrations and RabbitMQ jobs don't change.
- Deploy jobs get an optional `host` field naming the environment (default: the
  default environment). Their existing `environment` field is the sender's own
  label (e.g. `production`) and is left alone.

**Web UI**

- Environment switcher in the nav (name + up/down dot); the choice is in the
  URL (`/e/:env/containers`) so links and reloads keep it.
- "Environments" page: cards for every host with status, Docker version, OS,
  CPU/memory and container counts; clicking one opens its dashboard.
- "Add environment" dialog per connection type, with a **Test connection**
  button before saving.
- The existing dashboard, containers, images, networks and volumes pages are
  reused as-is under the selected environment.

**CLI**

- `locci env ls | add | rm | test | use <name>`
- Global `--environment` / `-E` flag and `LOCCI_ENV`; `locci env use` saves the
  default in the CLI config.

### Tasks

- [x] `environments` table + store methods; seed a `local` default on first start
- [x] secret encryption helper (`LOCCI_SECRET_KEY`), used for TLS keys
- [x] `internal/env` registry with lazy connect and health loop
- [x] `docker.New` options for `tcp+tls` and `ssh` connections
- [x] environment CRUD + test routes (admin only)
- [x] env-scoped resource routes; default-env aliases for the current routes
- [x] `host` field on deploy jobs (`environment` was already taken by senders)
- [x] UI: switcher, environments page, add/test dialog, env-aware routes
- [x] CLI: `env` commands, `-E/--environment` flag, saved default
- [x] tests: registry, routing, backward-compatible routes
- [ ] later: per-environment access for users (`environment_users`)
- [ ] later: `agent` connection type (outbound tunnel)

**Done when:** an admin can add a remote host over TLS or SSH, test it,
switch to it in the UI and CLI, and do everything they can do on the local
host today, while the old `/api/v1/...` routes behave exactly as before.

---

## 2. Single sign-on with OIDC

Let users log in through an OpenID Connect provider, including open-source
identity servers, alongside (or instead of) local passwords.

### Providers to support

Any OIDC provider with discovery (`/.well-known/openid-configuration`) works
through one generic implementation:

- **Open source / self-hosted:** Keycloak, Authentik, Zitadel, Authelia, Dex,
  Kanidm, Ory (Hydra/Kratos)
- **Hosted:** Google, Microsoft Entra ID, GitLab, Okta, Auth0
- **GitHub** is OAuth2 only, not OIDC. Support it through Dex, or add a small
  dedicated GitHub provider later.

### Design

**Default stays as is.** Username/password login (and `AUTH_TOKEN`) remains the
default and needs no setup. OIDC is off until an admin adds and enables a
provider: with none enabled, `/auth/providers` returns an empty list, the login
page shows only the password form, and the OIDC routes answer 404. Turning off
password login is a separate, explicit admin choice.

Libraries: `github.com/coreos/go-oidc/v3` (discovery, ID-token verification) and
`golang.org/x/oauth2` (code exchange).

**Storage**

- `oidc_providers`: `id`, `slug`, `display_name`, `issuer_url`, `client_id`,
  `client_secret` (encrypted), `scopes`, `enabled`, `auto_create_users`,
  `default_role`, `admin_group`, `allowed_email_domains`
- `user_identities`: `provider_id`, `subject`, `user_id`, `email`, `created_at`
  (unique on `provider_id` + `subject`); one user can have several identities
- `users.password_hash` becomes optional for SSO-only users

**Login flow (authorization code + PKCE)**

1. `GET /api/v1/auth/providers` lists enabled providers for the login page.
2. `GET /api/v1/auth/oidc/:provider/start` creates `state`, `nonce` and a PKCE
   verifier, keeps them in a short-lived signed HttpOnly cookie, and redirects
   to the provider.
3. `GET /api/v1/auth/oidc/:provider/callback` checks `state`, exchanges the
   code, verifies the ID token (issuer, audience, expiry, `nonce`), and requires
   `email_verified` when matching by email.
4. Finds the user through `user_identities`, or creates one when
   `auto_create_users` is on and the email domain is allowed. Admin role comes
   from `admin_group` in the `groups` claim, otherwise `default_role`.
5. Issues a normal `lc_` API token (same as password login) and redirects to
   `/login#token=...`. Putting the token in the fragment keeps it out of server
   logs. The UI stores it and clears the URL.

**Admin**

- API + UI page to add, edit, test and enable/disable providers (admin only)
- Option to turn off password login once SSO works (`AUTH_TOKEN` and
  `locci user` on the host stay as break-glass access)
- Login attempts and identity linking go to the audit log

**CLI**

- `locci login --sso <provider>`: starts a localhost callback listener, opens the
  browser, and receives the token. The server only redirects to `127.0.0.1`
  for this flow.
- Device authorization flow (`--device`) for headless machines, for providers
  that support it.

### Tasks

- [x] `oidc_providers` + `user_identities` tables; optional password hash (SSO-only users store `''`, which password login refuses)
- [x] provider CRUD + test-discovery routes (admin)
- [x] start/callback handlers with state, nonce, PKCE, signed cookie
- [x] user matching/creation, domain allow-list, group → role mapping
- [x] UI: provider buttons on the login page; token pickup from the URL fragment
- [x] UI: admin providers page
- [x] setting to disable password login
- [x] CLI: `login --sso` (loopback); break-glass `locci auth password-login on`
- [x] CLI: `login --sso <provider> --device` (device authorization flow)
- [x] tests with a fake OIDC provider (`httptest` issuer + signed tokens)
- [x] docs: setup guides (manual and scripted) for Keycloak, Authentik, Pocket ID and Dex, each tested end to end
- [ ] later: guides for hosted providers (Google, Entra ID) and Zitadel

**Done when:** a user can sign in with Keycloak (or any OIDC provider) from the
web UI and the CLI, gets the right role from their groups, and every login
shows up in the audit log.

---

## Later

Smaller or longer-term items from earlier design discussions:

- [ ] Move from the deprecated `github.com/docker/docker` module to `github.com/moby/moby/client`
- [ ] Compose stacks: deploy from an editor, upload or Git; webhook redeploys
- [ ] Registries: saved credentials and catalog browsing (including the Locci OCI registry)
- [ ] App templates with one-click deploy
- [ ] Volume file browser
- [ ] Image builds with embedded BuildKit (containerd worker)
- [ ] containerd runtime behind a `Runtime` interface (CNI networking, Traefik KV/HTTP provider for routing)
- [ ] GitHub Actions compatible CI on top of the build service
- [ ] Per-environment access for users (also listed in epic 1)
