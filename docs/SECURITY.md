# Security

## What locci can do

Access to a Docker socket (local, over TLS or over SSH) is root on that host.
Every signed-in locci user can manage containers on every environment, so
treat a locci account like shell access to those machines. Admins can also
manage users, environments and read the audit log.

## Accounts and tokens

- Passwords are stored as bcrypt hashes; they need 8+ characters.
- Logging in (web UI or `locci login`) issues an API token `lc_<64 hex>`. Only
  its SHA-256 hash is stored; logging out deletes it. Deleting a user deletes
  their tokens.
- `AUTH_TOKEN` is an optional static admin token for integrations. Leave it
  empty unless you need it, and treat it like a root password.
- Request logs record the method, path and status, never the query string
  (the WebSocket console passes its token as `?token=`).

## Stored secrets

- The TLS client key or SSH private key of a remote environment is encrypted
  with AES-256-GCM using `LOCCI_SECRET_KEY` before it reaches the database,
  with a fresh nonce per value.
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

## Web console

The exec WebSocket checks the browser's `Origin` against the request's host
before anything runs, so other sites can't open a shell with a user's token.

## Audit log

Every request that changes something (anything but GET) is recorded with the
time, user, method, path and status, and shown to admins under Audit log and
by `locci audit`. Logins are recorded too.

## Deployment checklist

- Serve locci over HTTPS only.
- Set `LOCCI_SECRET_KEY` and back it up with the database.
- Keep `AUTH_TOKEN` empty unless an integration needs it.
- Create named admin accounts (`locci user add NAME --admin`) instead of
  sharing one.
- Give remote hosts their own deploy key or client certificate, so one can be
  revoked without touching the others.

Report vulnerabilities privately to the maintainer rather than in a public
issue.
