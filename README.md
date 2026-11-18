# locci

A self-hosted Docker manager: API server, web UI and CLI in **one Go binary**.
Manage the local Docker host and any number of remote ones over TLS or SSH.

- **Web UI**: Svelte 5 / SvelteKit with shadcn-svelte, embedded with `go:embed`
- **API**: Gin, `/api/v1`, bearer-token auth (user tokens or a static `AUTH_TOKEN`)
- **CLI**: Cobra, talks to the same API from anywhere
- **Environments**: several Docker hosts (local socket, `tcp+tls`, `ssh`, or an
  outbound `locci agent` for hosts behind NAT), with health checks, pinned SSH
  host keys and keys encrypted at rest
- **Single sign-on**: any OpenID Connect provider (Keycloak, Authentik, Pocket ID,
  Dex, ...), off until an admin enables one; passwords stay the default
- **Storage**: libsql (local file or Turso) for users, tokens, environments and the audit log
- **Deploy jobs**: optional RabbitMQ consumer

## Quick start

```sh
just setup                            # .env, Go modules, web packages (needs Go 1.24+, Bun, a C compiler)
just all                              # build the UI, then the binary
./locci user add admin --admin        # prompts for a password
./locci serve                         # http://localhost:8998
```

Or in Docker: `just image && just up`, or `just compose-up` with the
repository's [`compose.yaml`](compose.yaml) (see [docs/DOCKER.md](docs/DOCKER.md)).

### Compose

```yaml
services:
  locci:
    image: locci/docker:latest
    restart: unless-stopped
    ports: ["8998:8998"]
    env_file: .env
    environment:
      LOCCI_SECRET_KEY: ${LOCCI_SECRET_KEY}
      DATABASE_URL: file:/data/locci.db         # keep the database on the volume
      DOCKER_HOST: unix:///var/run/docker.sock  # the socket mounted below
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - locci-data:/data
volumes:
  locci-data:
```

Then `docker exec -it <container> locci user add admin --admin` and open
http://localhost:8998.

From another terminal or machine:

```sh
locci login --url http://localhost:8998 -u admin
locci ps
locci run nginx:alpine --name web -p 80:8080 --network proxy-network
locci logs -f web

# another Docker host
locci env add edge --kind ssh --url ssh://deploy@10.0.0.5 --ssh-key ~/.ssh/locci_deploy
locci ps -E edge
```

`locci --help` lists every command; `-o json` prints raw JSON.

## Documentation

| | |
|---|---|
| [Usage](docs/USAGE.md) | the web UI, page by page |
| [Environments](docs/ENVIRONMENTS.md) | adding and preparing remote Docker hosts |
| [CLI](docs/CLI.md) | every command |
| [API](docs/API.md) | every route and payload |
| [Docker](docs/DOCKER.md) | running locci in a container |
| [Configuration](docs/CONFIGURATION.md) | environment variables |
| [Security](docs/SECURITY.md) | tokens, secrets, host keys, the Docker socket |
| [OIDC](docs/OIDC.md) | single sign-on, with [provider guides](docs/oidc/) |
| [Development](docs/DEVELOPMENT.md) | layout, tests, conventions |

Planned work: [BACKLOG.md](BACKLOG.md). Changes: [CHANGELOG.md](CHANGELOG.md).

## Compatibility

The original v1 routes keep their paths and payloads and act on the default
environment: `POST/GET /api/v1/networks`, `POST/GET /api/v1/containers`,
`POST /api/v1/containers/start`, `DELETE /api/v1/containers`.

## Notes

- go-libsql uses cgo: build with `CGO_ENABLED=1` on glibc (the Dockerfile handles this).
- Access to a Docker socket is root on that host. Serve locci over HTTPS and
  read [docs/SECURITY.md](docs/SECURITY.md).
