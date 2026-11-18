# Running in Docker

The [`Dockerfile`](../Dockerfile) builds everything in three stages:

1. **web**: `oven/bun` installs the UI's packages from `bun.lock`
   (`--frozen-lockfile`) and builds the SvelteKit app.
2. **build**: Go compiles the single static binary with the UI embedded,
   cross-compiling for each target platform (no emulation for multi-arch).
3. **runtime**: Debian slim with the binary, `ca-certificates` and nothing
   else. The database lives on the `/data` volume
   (`DATABASE_URL=file:/data/locci.db`), the server listens on `8998`.

## Quick start

```sh
cp .env.sample .env                      # set LOCCI_SECRET_KEY if you'll add remote hosts
just image                               # docker build
just up                                  # run it with the host's Docker socket and a data volume
docker exec -it locci locci user add admin --admin
open http://localhost:8998
```

`just up` is:

```sh
docker run -d --name locci -p 8998:8998 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v locci-docker-data:/data \
  --env-file .env \
  locci/docker:latest
```

`just docker-logs` follows the logs and `just down` removes the container
(the `locci-docker-data` volume, and with it the database, is kept).

`just up` takes settings such as `LOCCI_SECRET_KEY` from `.env`, but always sets
`DATABASE_URL`, `DOCKER_HOST`, `SERVER_PORT` and `GIN_MODE` for the container.
The `.env` values for those are meant for running on the host, such as a
relative database file or a Colima socket path.

## Compose

[`compose.yaml`](../compose.yaml) runs the same server, plus optional
identity providers for single sign-on in production mode:

```sh
just compose-up                      # locci only, then http://localhost:8998
just compose-up keycloak             # locci and Keycloak (with its PostgreSQL)
just compose-up pocket-id            # locci and Pocket ID
just compose-up keycloak pocket-id   # all of them
just compose-logs
just compose-down                    # stops everything; the volumes are kept
```

Without just: `docker compose --profile keycloak up -d`.

| Container | What | Volume |
|---|---|---|
| `locci-docker` | the server and web UI | `locci-docker-data` (the database) |
| `locci-docker-net` | holds the network and the published ports (see below) | |
| `locci-docker-keycloak` | Keycloak 26, profile `keycloak` | |
| `locci-docker-keycloak-db` | PostgreSQL 17 for Keycloak, profile `keycloak` | `locci-docker-keycloak-db` |
| `locci-docker-pocket-id` | Pocket ID 1.16, profile `pocket-id` | `locci-docker-pocket-id-data` |

- It builds the image from the current source each time (cached layers keep
  this quick), so the container never runs stale code.
  `LOCCI_TAG=v1.2.3 just compose-up` names a different image tag.
- `.env` is optional for locci alone and works the same way as with
  `just up`. The identity providers need secrets from it (below).
- It uses the same `locci-docker-data` volume as `just up`, so both see one
  database. Run one of them at a time, since both want port 8998 (and the
  container names are fixed, so one compose project per machine).
- locci, Keycloak and Pocket ID share one network namespace, held by
  `locci-docker-net` (a `registry.k8s.io/pause` container, under 1 MB, as in
  a Kubernetes pod). The published ports belong to it, so `localhost:8180`
  is Keycloak both for your browser and inside locci's container, and
  rebuilding locci doesn't disturb the providers.

### Upgrading from the `locci-data` volume

Earlier versions kept the database in a volume named `locci-data`. Copy it
into the new one once, with the server stopped:

```sh
just compose-down    # or: just down
docker run --rm -v locci-data:/from -v locci-docker-data:/to alpine cp -a /from/. /to/
just compose-up
```

Remove `locci-data` (`docker volume rm locci-data`) once the new one works.

## Single sign-on with the bundled providers

Keycloak and Pocket ID run in production mode: Keycloak with `start` (not
`start-dev`) on PostgreSQL, Pocket ID with its normal server. Both expect
TLS at a reverse proxy in front of them, like locci does
([REVERSE_PROXY.md](REVERSE_PROXY.md)).

### Settings (`.env`)

| Variable | Needed for | Default | What |
|---|---|---|---|
| `LOCCI_SECRET_KEY` | either | | locci stores the client secret with it (`openssl rand -base64 32`) |
| `KEYCLOAK_ADMIN_PASSWORD` | keycloak | (required) | the first Keycloak admin's password; read on the first start |
| `KEYCLOAK_ADMIN` | keycloak | `admin` | that admin's username |
| `KEYCLOAK_DB_PASSWORD` | keycloak | (required) | PostgreSQL password; fixed when the database is first created |
| `KEYCLOAK_URL` | keycloak | `http://localhost:8180` | the address people and locci reach Keycloak at, such as `https://auth.example.com` |
| `KEYCLOAK_PORT` | keycloak | `8180` | Keycloak's port, inside and outside |
| `POCKET_ID_ENCRYPTION_KEY` | pocket-id | (required) | encrypts Pocket ID's signing keys (`openssl rand -base64 32`); keep it with your backups |
| `POCKET_ID_URL` | pocket-id | `http://localhost:1411` | the address people and locci reach Pocket ID at, such as `https://id.example.com` |
| `POCKET_ID_PORT` | pocket-id | `1411` | Pocket ID's port, inside and outside |

A provider whose required settings are missing doesn't start, and its log
says which ones (`docker logs locci-docker-keycloak`). Compose can't require
a variable for one profile only, so the check runs when the container
starts.

The ports are published even when the profiles are off (they belong to
`locci-docker-net`). If 8180 or 1411 is taken on your machine, change the
port variable.

### Where they run

**On a server, behind a reverse proxy (the usual case).** Point a hostname
at each provider through your proxy with HTTPS, and set `KEYCLOAK_URL` /
`POCKET_ID_URL` (and `LOCCI_PUBLIC_URL` for locci) to those `https://`
addresses. locci reaches the providers at the same public addresses as
browsers do, so the server must be able to resolve and reach them. Bind the
published ports to `127.0.0.1` (or firewall them) so only the proxy reaches
them.

**On one machine, to try it.** The defaults work as they are, on plain HTTP
at `localhost`: the shared network makes `localhost` the same place for the
browser and for locci, and browsers treat `localhost` as secure, so Pocket
ID's passkeys work there. Use `localhost`, not `127.0.0.1` or a LAN address,
in the browser and in issuer URLs.

### Set it up

1. Set the variables above in `.env` and start, for example
   `just compose-up keycloak`. Create a locci admin:
   `docker exec -it locci-docker locci user add admin --admin`.
2. In the provider, create a client for locci with this redirect URI
   (`<slug>` is the name you give the provider in locci in step 3, such as
   `keycloak` or `pocketid`; use `LOCCI_PUBLIC_URL` in place of
   `http://localhost:8998` behind a proxy):

   ```
   http://localhost:8998/api/v1/auth/oidc/<slug>/callback
   ```

   - **Keycloak:** sign in to the admin console at `KEYCLOAK_URL` with
     `KEYCLOAK_ADMIN`, create a realm (such as `locci`), a confidential
     OpenID Connect client with that redirect URI, a `locci-admins` group
     and users. Then make a permanent admin account: the bootstrap admin is
     meant to be temporary. Details: [oidc/KEYCLOAK.md](oidc/KEYCLOAK.md).
   - **Pocket ID:** open `<POCKET_ID_URL>/setup` to create the admin and its
     passkey, then Administration, OIDC Clients, Add, with that callback
     URL, and a `locci-admins` group under User Groups. Details:
     [oidc/POCKETID.md](oidc/POCKETID.md).
3. In locci, Harbour master, **Sign-in**, **Add provider**: the issuer URL
   (Keycloak: `<KEYCLOAK_URL>/realms/<realm>`; Pocket ID: `POCKET_ID_URL`),
   the client ID and secret, and `locci-admins` as the admin group.
4. Sign out. The login page now shows "Continue with ...". From the CLI:
   `locci login --url http://localhost:8998 --sso <slug>`.

### Back up

- `locci-docker-data` and `LOCCI_SECRET_KEY` (see below).
- `locci-docker-keycloak-db` (or `pg_dump` in `locci-docker-keycloak-db`).
- `locci-docker-pocket-id-data` and `POCKET_ID_ENCRYPTION_KEY`.

Tested on one machine with the defaults: Keycloak starts in its `prod`
profile on PostgreSQL, a user signed in through it end to end and got a
locci session, Pocket ID starts and locci discovers it and redirects to it,
and providers with missing secrets refuse to start with a message. Not
tested: the same behind a real reverse proxy with public hostnames.

## The agent image

The same image runs the outbound agent on a remote Docker host (see
[ENVIRONMENTS.md](ENVIRONMENTS.md#agent-hosts-outbound-behind-nat)):

```sh
docker run -d --name locci-agent --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e LOCCI_AGENT_TOKEN=la_... \
  locci/docker agent --server https://locci.example.com
```

It needs no ports and no volume.

## Things to know

- **The Docker socket is root on the host.** Admins can use all of it; plain
  users can run containers but can't mount host paths or open a console in
  locci's own container (the image is labelled `io.locci.protected=true`).
  Put locci behind HTTPS (a reverse proxy such as Traefik or Caddy), keep
  strong passwords, and see [SECURITY.md](SECURITY.md).
- **Without the socket** the `local` environment is `down`, but locci still
  works as a manager of remote environments over TLS or SSH. SSH needs no
  `ssh` client in the image; locci has its own.
- **Back up `/data` together with `LOCCI_SECRET_KEY`.** The database holds
  users, tokens, environments and the audit log; the key is needed to read
  the stored remote-host keys.
- **Turso instead of a file:** set `DATABASE_URL=libsql://<db>.turso.io?authToken=...`
  and the volume becomes optional.
- **Behind a reverse proxy or tunnel** (Traefik, NGINX, Caddy, Cloudflare,
  NetBird, Tailscale), set `LOCCI_PUBLIC_URL` and pass WebSockets through;
  [REVERSE_PROXY.md](REVERSE_PROXY.md) has configurations for each.
