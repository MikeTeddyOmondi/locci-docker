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
