# Security

## What locci can do

Access to a Docker socket (local, over TLS or over SSH) is root on that host,
and locci holds that access for every environment.

**Admins** can do anything Docker can on every environment, and manage users,
environments, single sign-on and the audit log. Treat an admin account like
root on those machines.

**Plain users** (role `user`) run and manage containers, images, networks and
volumes, but are kept from the hosts themselves:

- New containers may use named volumes only, not host paths
  (`/etc:/x`, `./data:/data`, the Docker socket).
- They can't set `traefik.*` labels (which decide where a domain's traffic
  goes) or `io.locci.*` labels, and can't take a subdomain another container
  already routes.
- They can't start, stop, remove, exec into or open a console in a
  **protected** container: one labelled `io.locci.protected=true` (the locci
  image sets it, so the server and its agents are covered), one that is
  privileged, or one with the Docker socket or the host's `/` bind-mounted
  (Traefik, Portainer, CI runners). Inspecting one hides its environment
  variables.

Admins can also limit an environment to chosen users (`locci env access`,
or Access on the Environments page); others then can't see or use it at all.

Admins can still do all of that. `LOCCI_USER_HOST_ACCESS=true` lifts the
limits for everyone, for teams where every user is trusted with the hosts.

The limits close the paths from a plain account to root on a host. They don't
isolate users from each other: a plain user can still stop, remove or read the
logs and environment of anyone's unprotected containers, publish ports, and
run any image. Label a container `io.locci.protected=true` to put it out of
their reach.

## Accounts and tokens

- Passwords are stored as bcrypt hashes; they need 8+ characters.
- Password logins are rate limited: 10 failures from one IP, or 20 for one
  username from anywhere, within 15 minutes lock that IP or username out until
  the window ends (HTTP 429). The username lockout doesn't apply at addresses
  where that user signed in successfully in the last 30 days, so guessing
  from elsewhere can't lock the owner out. Failures are in the audit log. A login for an
  unknown username takes as long as a wrong password.
- Logging in (web UI or `locci login`) issues an API token `lc_<64 hex>`. Only
  its SHA-256 hash is stored; logging out deletes it. Deleting a user deletes
  their tokens.
- A token expires after `LOCCI_TOKEN_TTL_DAYS` (default 30) without use. Users
  see and sign out their sessions in the web UI (user menu, Sessions) or with
  `locci sessions`.
- The web UI's token is an HttpOnly session cookie (`SameSite=Lax`, `Secure`
  over HTTPS): page scripts can't read it. Requests that change something
  must also carry an `X-Locci-CSRF` header, which other sites can't send
  (locci allows no cross-origin requests), so a page elsewhere can't act
  with the cookie. The console WebSocket checks the page's origin.
- `AUTH_TOKEN` is an optional static admin token for integrations. Leave it
  empty unless you need it, and treat it like a root password. It is compared
  in constant time and never expires.
- `locci user ...` writes straight to the database named by `DATABASE_URL`,
  without the server (see [CLI.md](CLI.md#admin)). Users are created and
  promoted only there or by an admin through the API: no endpoint creates
  the first admin. So access to the database is admin access. For a local
  file that means the host; for Turso, the `libsql://` URL and its auth
  token are an admin key, usable from anywhere and not in locci's audit log.
  Keep that token as secret as `AUTH_TOKEN`, scope it to this database, and
  rotate it in Turso if it leaks.
- Tokens go in the `Authorization` header. Only WebSocket requests (the
  console) may pass one as `?token=`, since browsers can't set headers there.
- Request logs record the method, path and status, never the query string.

## HTTP

- Every response sets a Content-Security-Policy that allows only locci's own
  scripts (plus SvelteKit's inline bootstrap, by hash), styles, fonts and
  images, and forbids framing (`frame-ancestors 'none'`, `X-Frame-Options:
  DENY`); also `X-Content-Type-Options: nosniff`, `Referrer-Policy:
  no-referrer`, and HSTS over HTTPS.
- Request bodies are limited to 1 MiB and request headers must arrive within
  10 seconds. There is no overall timeout, since log streams, image pulls, the
  console and agents stay open.
- `X-Forwarded-For`, `-Proto` and `-Host` are believed only from
  `LOCCI_TRUSTED_PROXIES` (default loopback and private networks). If clients
  can reach locci directly from such a network, narrow it to your proxy's
  address.
- SIGTERM stops locci gracefully: running requests and a running deploy job
  get up to 25 seconds.

## Single sign-on

- OIDC uses the authorization code flow with PKCE. State, nonce and the PKCE
  verifier travel in an HttpOnly, SameSite=Lax cookie sealed with
  `LOCCI_SECRET_KEY`, valid for 10 minutes and used once. The ID token's
  issuer, audience, expiry and nonce are verified.
- The resulting token reaches the browser in the URL fragment, which is never
  sent to servers or written to logs, and the page removes it at once. The CLI
  receives it on a listener bound to `127.0.0.1`.
- The device flow is driven by the server, which holds the client secret. The
  CLI only sees the user code and a handle sealed with `LOCCI_SECRET_KEY`
  (containing the device code and a PKCE verifier), so the device code can't
  be read, altered or used with another provider.
- Accounts are linked by email only when the provider says the email is
  verified. Only enable providers you trust to verify emails, and use the
  allowed-domains list to keep strangers out of providers that admit anyone
  (such as Google).
- An admin group in the provider makes locci roles follow group membership on
  every sign-in.
- Turning password login off is refused while no provider is enabled;
  `locci auth password-login on` on the server host undoes it.

## Stored secrets

- OIDC client secrets, and the TLS client key or SSH private key of a remote
  environment, are encrypted with AES-256-GCM using `LOCCI_SECRET_KEY` before
  they reach the database, with a fresh nonce per value.
- The API never returns a key, only `has_secret`. Edits keep the stored key
  unless a new one is sent.
- Without `LOCCI_SECRET_KEY` the server refuses to store keys rather than
  storing them in the clear.
- Losing or changing the key makes stored keys unreadable (the hosts go
  `down`); re-enter them by editing each environment.

## SSH host keys

The first successful connection pins the host's SSH key, and from then on only
that key type is negotiated and a different key is refused (`ssh host key
changed`). Compare the fingerprint shown when adding a host with
`ssh-keygen -lf` on the host. Forget the pin only when you know the host was
rebuilt. Use a dedicated deploy key for locci, not your personal key.

## Agents

- An agent token (`la_...`) gives exactly one thing: the right to serve that
  environment's Docker to the server. It can't call the API. Only its SHA-256
  hash is stored; it is shown once.
- The agent connects out over `wss://` with normal TLS verification, so it
  only talks to the real server. Run the server with a valid certificate.
- Whoever controls the locci server controls every connected host's Docker,
  as with any other environment kind.
- Rotate the token if it leaks: the running agent is dropped and the old
  token stops working. Connects and disconnects are in the audit log as
  `agent:<name>`.

## Web console

The exec WebSocket checks the browser's `Origin` against the request's host
before anything runs, so other sites can't open a shell with a user's token.

## Audit log

Every request that changes something (anything but GET) is recorded with the
time, user, method, path and status, and shown to admins under Audit log and
by `locci audit`. Logins are recorded too.

## Deployment checklist

- Serve locci over HTTPS only, and set `LOCCI_TRUSTED_PROXIES` to your
  reverse proxy if anything else shares its network.
- Give people the plain `user` role unless they need admin; keep
  `LOCCI_USER_HOST_ACCESS` off.
- Set `LOCCI_SECRET_KEY` and back it up with the database.
- Keep `AUTH_TOKEN` empty unless an integration needs it.
- With Turso, keep the database auth token only on the server.
- Create named admin accounts (`locci user add NAME --admin`) instead of
  sharing one.
- Give remote hosts their own deploy key or client certificate, so one can be
  revoked without touching the others.

Report vulnerabilities privately to the maintainer rather than in a public
issue.
