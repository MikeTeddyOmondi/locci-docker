# Running in Docker

The [`Dockerfile`](../Dockerfile) builds everything in three stages:

1. **web**: `oven/bun` installs the UI's packages from `bun.lock`
   (`--frozen-lockfile`) and builds the SvelteKit app.
2. **build**: Go with cgo (go-libsql needs it) compiles the single binary with
   the UI embedded.
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
  <image>:dev
```

`just docker-logs` follows the logs and `just down` removes the container
(the `locci-data` volume, and with it the database, is kept).

## Compose

```yaml
services:
  locci:
    image: <image>:<tag>        # from `just image tag=<tag>`
    restart: unless-stopped
    ports: ["8998:8998"]
    env_file: .env
    environment:
      LOCCI_SECRET_KEY: ${LOCCI_SECRET_KEY}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - locci-data:/data
volumes:
  locci-data:
```

## Things to know

- **The Docker socket is root on the host.** Anyone who can log in to locci
  as a user can run containers there. Put locci behind HTTPS (a reverse proxy
  such as Traefik or Caddy), keep strong passwords, and see
  [SECURITY.md](SECURITY.md).
- **Without the socket** the `local` environment is `down`, but locci still
  works as a manager of remote environments over TLS or SSH. SSH needs no
  `ssh` client in the image; locci has its own.
- **Back up `/data` together with `LOCCI_SECRET_KEY`.** The database holds
  users, tokens, environments and the audit log; the key is needed to read
  the stored remote-host keys.
- **Turso instead of a file:** set `DATABASE_URL=libsql://<db>.turso.io?authToken=...`
  and the volume becomes optional.
- **Behind a reverse proxy**, forward WebSockets for
  `/api/v1/.../exec/ws` (the web console) and keep the `Host` header: the
  WebSocket origin check compares the browser's `Origin` with it.
