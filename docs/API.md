# HTTP API

Base path: `/api/v1`. Requests and responses are JSON unless noted. Errors are
`{"error": "message"}` with a fitting status code (Docker "not found" is 404,
"conflict" is 409, bad input is 400).

## Authentication

Send a token as `Authorization: Bearer <token>`. WebSockets can't set headers,
so the exec WebSocket also accepts `?token=<token>`; other requests don't.

A token expires after `LOCCI_TOKEN_TTL_DAYS` (default 30) without use; every
use pushes the date back. An expired, revoked or unknown token gets 401.
Failed password logins are rate limited (429 with `Retry-After`).

Tokens come from:

- `POST /api/v1/auth/login` with `{"username": "...", "password": "..."}`,
  which returns `{"token": "lc_...", "user": {...}}`, or
- the static `AUTH_TOKEN`, if the server sets one (acts as an admin called
  `system`).

| Route | Auth | Notes |
|---|---|---|
| `GET /health` | none | `200 {"status":"healthy"}` when the default environment's Docker answers, else `503` |
| `POST /auth/login` | none | password login |
| `GET /auth/me` | user | the current user (with `has_password`: false for SSO-only users) |
| `POST /auth/logout` | user | revokes the token |
| `GET /auth/tokens` | user | the caller's sessions: `id`, `name`, `created_at`, `last_used_at`, `expires_at`, `current` (never the token) |
| `DELETE /auth/tokens/:id` | user | signs out one of the caller's sessions |

`POST /auth/login` answers `403` while password login is turned off.

## Single sign-on

See [OIDC.md](OIDC.md). Public routes:

| Route | Notes |
|---|---|
| `GET /auth/providers` | `{"password_login": true, "providers": [{"slug","display_name"}]}`; the list is empty until a provider is enabled |
| `GET /auth/oidc/:provider/start[?cli_port=N]` | redirects to the provider; `404` unless the provider is enabled |
| `GET /auth/oidc/:provider/callback` | finishes sign-in; redirects to `/login#token=...` (or `#error=...`), or for the CLI to `http://127.0.0.1:N/callback?token=...` |
| `POST /auth/oidc/:provider/device` | device flow: `{"user_code", "verification_uri", "verification_uri_complete", "expires_in", "interval", "handle"}`; `400` if the provider has no device endpoint |
| `POST /auth/oidc/:provider/device/token` | `{"handle": "..."}`; `{"status": "pending"}` or `"slow_down"` while waiting (poll at `interval` seconds, add 5 on `slow_down`), `{"status": "ok", "token", "user"}` once approved, `400` on denial or expiry |

Admin routes:

| Route | Notes |
|---|---|
| `GET /sso/providers` | `{"providers": [...], "password_login": bool}`; each has `has_secret` and the `redirect_uri` to register, never the secret |
| `POST /sso/providers` | create; body below; `412` without `LOCCI_SECRET_KEY` |
| `PATCH /sso/providers/:provider` | fields you send change; `client_secret` "" keeps the stored one, `"clear_secret": true` removes it |
| `DELETE /sso/providers/:provider` | linked identities go too; the users stay |
| `POST /sso/providers/test` | `{"issuer_url": "..."}`; runs discovery: `{"ok", "authorization_endpoint", "token_endpoint", "scopes_supported", "warnings"}` or `{"ok": false, "error"}` |
| `GET /sso/settings` | `{"password_login", "public_url"}` |
| `PUT /sso/settings` | `{"password_login": false}`; `409` while no provider is enabled |

Provider body: `slug`, `display_name`, `issuer_url`, `client_id`,
`client_secret`, `scopes` (default `openid email profile`), `enabled`,
`auto_create_users`, `default_role` (`user` or `admin`), `admin_group`,
`groups_claim` (default `groups`), `allowed_email_domains` (comma separated).

## Environments

An environment is one Docker host; see [ENVIRONMENTS.md](ENVIRONMENTS.md).
Responses never include keys, only `has_secret`.

| Route | Auth | Notes |
|---|---|---|
| `GET /environments` | user | `{"environments": [...]}`, the default first |
| `GET /environments/:env` | user | one environment |
| `GET /environments/:env/info` | user | Docker host info (same shape as `/system/info`) |
| `POST /environments` | admin | create; `201` |
| `PATCH /environments/:env` | admin | update; empty fields and missing keys are kept |
| `DELETE /environments/:env` | admin | `409` for the default |
| `POST /environments/:env/default` | admin | make it the default |
| `POST /environments/test` | admin | try settings without saving; always `200` with `{"ok": true, "host": {...}}` or `{"ok": false, "error": "..."}` |

Create, update and test body:

```json
{
  "name": "edge",
  "label": "Edge box",
  "kind": "ssh",
  "url": "ssh://deploy@10.0.0.5",
  "ssh_key": "-----BEGIN OPENSSH PRIVATE KEY-----\n...",
  "ssh_host_key": "ecdsa-sha2-nistp256 AAAA...",
  "labels": {"site": "nbo"}
}
```

- `kind` is `local`, `tcp+tls` or `ssh`. TLS uses `tls_ca`, `tls_cert` and
  `tls_key` (PEM strings).
- `ssh_host_key` pins the key a previous test returned; without it, creating
  an SSH environment pins whatever key the host presents (when reachable).
- `kind` `agent` needs no URL or keys. Creating one returns its
  `agent_token` (`la_...`) **once**; `PATCH {"rotate_agent_token": true}`
  issues a new one and disconnects the agent. Agent environments report
  `agent_connected`.
- `PATCH` also accepts `"reset_host_key": true`. `kind` can't change; changing
  `url` drops the pinned host key.
- `POST /environments/test` with the `name` of an existing environment fills
  blank certificates and key from the stored environment, so edits can be
  tested without resending secrets.
- Saving a key without `LOCCI_SECRET_KEY` on the server returns `412`.

Example environment:

```json
{
  "id": 3, "name": "edge", "label": "Edge box", "kind": "ssh",
  "url": "ssh://deploy@10.0.0.5", "labels": {}, "is_default": false,
  "status": "up", "last_seen_at": "2026-11-17T22:46:17Z",
  "created_at": "2026-11-17T22:33:52Z", "has_secret": true,
  "ssh_host_key": "ecdsa-sha2-nistp256 AAAA...",
  "ssh_fingerprint": "ecdsa-sha2-nistp256 SHA256:6YbW..."
}
```

## Agent connection

| Route | Auth | Notes |
|---|---|---|
| `GET /agent/connect` | `Authorization: Bearer la_...` (agent token) | WebSocket upgrade used by `locci agent`; the response header `X-Locci-Environment` names the environment. The connection then carries yamux streams to the agent's Docker socket. `401` for an unknown token. |

## Docker resources

Every route in this section exists twice:

- `/api/v1/<route>` uses the **default** environment, exactly as before
  environments existed, and
- `/api/v1/environments/<env>/<route>` uses the named one.

Unknown environments are `404`; a host whose key can't be decrypted is `503`.

### Containers

| Route | Notes |
|---|---|
| `GET /containers` | `{"containers": [...]}` (Docker's container summaries, all states) |
| `POST /containers` | create (pulls the image first); `201 {"container_id","name","warnings","domain"}` |
| `GET /containers/:id` | Docker's inspect output |
| `DELETE /containers/:id?force=true` | remove (stops first when `force`) |
| `POST /containers/:id/{start,stop,restart,kill,pause,unpause}` | `{"container_id","action","status":"ok"}` |
| `GET /containers/:id/logs?follow=true&tail=200` | plain text stream; `tail` can be `all` |
| `GET /containers/:id/stats` | `{"cpu_percent","mem_usage","mem_limit","mem_percent","net_rx","net_tx","pids"}` |
| `POST /containers/:id/exec` | `{"cmd": ["nginx","-v"]}` runs to completion; `{"exit_code","output"}` |
| `GET /containers/:id/exec/ws?cmd=sh&token=...` | interactive TTY over WebSocket (see below) |

Create body:

```json
{
  "name": "web",
  "image": "nginx:alpine",
  "subdomain": "web",
  "ports": {"80": "8080"},
  "environment": ["KEY=value"],
  "networks": ["proxy-network"],
  "volumes": ["data:/data"],
  "labels": {"team": "web"},
  "command": []
}
```

`ports` maps container port to host port (empty or `0` picks one). Traefik
labels routing `<subdomain>.<TRAEFIK_DOMAIN>` to the first port are added.

**Exec WebSocket:** binary or text frames from the client are stdin, except a
JSON text frame `{"resize": [cols, rows]}`. Output arrives as binary frames.
Cross-site origins are rejected before anything runs.

### Images

| Route | Notes |
|---|---|
| `GET /images` | `{"images": [...]}` |
| `POST /images/pull` | `{"image": "nginx:alpine"}` |
| `POST /images/prune` | removes dangling images; Docker's prune report |
| `DELETE /images/:id?force=true` | `{"deleted": [...]}` |

### Networks

| Route | Notes |
|---|---|
| `GET /networks` | `{"networks": [...]}` |
| `POST /networks` | `{"name": "proxy-network", "driver": "bridge", "labels": {}}`; `201 {"network_id","name","warnings"}` |
| `GET /networks/:id` | Docker's inspect output |
| `DELETE /networks/:id` | |

### Volumes

| Route | Notes |
|---|---|
| `GET /volumes` | `{"volumes": [...]}` |
| `POST /volumes` | `{"name": "data", "driver": "local", "labels": {}}`; `201` |
| `DELETE /volumes/:name?force=true` | |

### System

| Route | Notes |
|---|---|
| `GET /system/info` | Docker's `info` for the host |

## Admin

| Route | Notes |
|---|---|
| `GET /users` | `{"users": [...]}` |
| `POST /users` | `{"username","password","role"}`, role `admin` or `user`; passwords need 8+ characters |
| `DELETE /users/:name` | also revokes their tokens |
| `GET /audit?limit=100` | `{"audit": [...]}`: every non-GET request (time, user, method, path, status) |

## Backward compatibility

The original v1 API keeps its paths and payloads and acts on the default
environment:

- `GET/POST /api/v1/networks`
- `GET/POST /api/v1/containers`
- `POST /api/v1/containers/start` with `{"container_id": "..."}`
- `DELETE /api/v1/containers` with `{"container_id": "...", "force": true}`

The last two exist only on the default environment; under
`/environments/:env` use `POST /containers/:id/start` and
`DELETE /containers/:id`.
