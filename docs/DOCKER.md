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
  -v locci-data:/data \
  --env-file .env \
  locci/docker:latest
```

`just docker-logs` follows the logs and `just down` removes the container
(the `locci-data` volume, and with it the database, is kept).

`just up` takes settings such as `LOCCI_SECRET_KEY` from `.env`, but always sets
`DATABASE_URL`, `DOCKER_HOST`, `SERVER_PORT` and `GIN_MODE` for the container.
The `.env` values for those are meant for running on the host, such as a
relative database file or a Colima socket path.

## Compose

[`compose.yaml`](../compose.yaml) runs the same container:

```sh
just compose-up        # docker compose up -d --build, then http://localhost:8998
just compose-logs
just compose-down      # the locci-data volume is kept
```

- It builds the image from the current source each time (cached layers keep
  this quick), so the container never runs stale code.
- `.env` is optional and works the same way as with `just up`.
- `LOCCI_PORT=9000 just compose-up` publishes on another port.
- It uses the same `locci-data` volume as `just up`, so both see one database.
  Run one of them at a time, since both want port 8998.
- `LOCCI_TAG=v1.2.3 just compose-up` names a different image tag.
- The server shares its network with a tiny `net` service
  (`registry.k8s.io/pause`, under 1 MB) that holds the published ports. That
  is what lets the identity providers below sit on `localhost` for both your
  browser and locci; rebuilding locci doesn't disturb them.

## Identity providers for trying SSO

`compose.yaml` can also start Keycloak or Pocket ID next to locci, so you can
try single sign-on without setting up a provider elsewhere. They're compose
profiles: off unless you name them.

```sh
just compose-up keycloak             # locci and Keycloak
just compose-up pocket-id            # locci and Pocket ID
just compose-up keycloak pocket-id   # both
just compose-down                    # stops all of them; their data volumes are kept
```

Without just: `docker compose --profile keycloak up -d`.

| Provider | Opens at | Issuer URL for locci | First sign-in |
|---|---|---|---|
| Keycloak 26 | http://localhost:8180 | `http://localhost:8180/realms/<realm>` | `admin` / `change-me` |
| Pocket ID 1.16 | http://localhost:1411 | `http://localhost:1411` | http://localhost:1411/setup creates the admin and a passkey |

Every container in the set shares one network, so `localhost:8180` is
Keycloak both in your browser and inside locci's container. The issuer URL
must be the same for both: locci fetches the provider's keys and tokens
from it, and the provider puts it in every token. Use `localhost` (not
`127.0.0.1` or a LAN address) in the issuer and in your browser.

### Set it up

1. Set `LOCCI_SECRET_KEY` in `.env` (`openssl rand -base64 32`); locci needs
   it to store the client secret. Start with `just compose-up keycloak` (or
   `pocket-id`) and create a locci admin:
   `docker compose exec locci locci user add admin --admin`.
2. In the provider, create a client for locci with this redirect URI
   (`<slug>` is the name you give the provider in locci in step 3, such as
   `keycloak` or `pocketid`):

   ```
   http://localhost:8998/api/v1/auth/oidc/<slug>/callback
   ```

   - **Keycloak:** sign in to the admin console, create a realm (such as
     `locci`), then a confidential OpenID Connect client with that redirect
     URI, a `locci-admins` group and a user. Step by step, and a scripted
     version: [oidc/KEYCLOAK.md](oidc/KEYCLOAK.md).
   - **Pocket ID:** after `/setup`, Administration, OIDC Clients, Add, with
     that callback URL; a `locci-admins` group under User Groups. See
     [oidc/POCKETID.md](oidc/POCKETID.md).
3. In locci, Harbour master, **Sign-in**, **Add provider**: the issuer URL from
   the table, the client ID and secret, and `locci-admins` as the admin
   group. Or use the API call in the provider's guide, with the issuer above.
4. Sign out. The login page now shows "Continue with ...". From the
   CLI: `locci login --url http://localhost:8998 --sso <slug>`.

### Settings

Set these in `.env` to change the defaults:

| Variable | Default | What |
|---|---|---|
| `KEYCLOAK_PORT` | `8180` | Keycloak's port, inside and outside (the issuer URL follows it) |
| `KEYCLOAK_ADMIN`, `KEYCLOAK_ADMIN_PASSWORD` | `admin`, `change-me` | the first Keycloak admin; only read on Keycloak's first start |
| `POCKET_ID_PORT` | `1411` | Pocket ID's port, inside and outside |
| `POCKET_ID_ENCRYPTION_KEY` | a fixed trial value | encrypts Pocket ID's signing keys; set your own (`openssl rand -base64 32`) before keeping any data |

The ports are published even when the profiles are off (they belong to the
`net` service). If 8180 or 1411 is taken on your machine, change the
variable.

### Limits

These setups are for trying SSO on one machine. Keycloak runs in
development mode with its built-in database, and both use plain HTTP on
`localhost` (Pocket ID's passkeys work there because browsers treat
`localhost` as secure). For a provider other people sign in to, run it
properly behind HTTPS and give locci its public issuer URL; see
[OIDC.md](OIDC.md).

Tested: both providers start, locci in its container discovers each issuer
at `localhost` and redirects to it for sign-in, and that still holds after
locci is rebuilt. A full browser sign-in through these compose-run
providers wasn't repeated here; the provider guides cover that against the
same versions.

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
