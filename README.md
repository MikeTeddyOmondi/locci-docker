# Locci Docker

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
- **Storage**: SQLite in pure Go (a local file) or remote libsql/Turso for users, tokens, environments and the audit log
- **Deploy jobs**: optional RabbitMQ consumer

## Install from a release

On Linux or macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/MikeTeddyOmondi/locci-docker/main/install.sh | bash
```

To read [install.sh](install.sh) first, download it with `-o install.sh` and
run `bash install.sh` instead.

It downloads the latest release's archive for your OS and CPU, checks the
signature on `SHA256SUMS` against the release key (pinned by fingerprint in
the script; skipped with a note if gpg isn't installed) and the archive's
checksum, then copies `locci` to `/usr/local/bin`. Settings go before `bash`:
`| LOCCI_VERSION=v0.1.0 bash` picks a release, and
`| LOCCI_INSTALL_DIR=~/.local/bin bash` installs without sudo.

Windows, or by hand: each
[release](https://github.com/MikeTeddyOmondi/locci-docker/releases) has
`locci-docker-<version>-<os>-<arch>` archives (`.zip` for Windows) holding
`locci`, this README and the licenses; verifying them is in
[docs/RELEASING.md](docs/RELEASING.md#what-is-signed-and-how-to-verify-it).

Then point it at a server (`locci login --url https://locci.example.com`), or
run one (`locci serve`, below). The Docker image is `locci/docker`.

## Quick start

```sh
just setup                            # .env, Go modules, web packages (needs Go 1.27+ and Bun)
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
    container_name: locci-docker
    restart: unless-stopped
    ports: ["8998:8998"]
    env_file: .env
    environment:
      LOCCI_SECRET_KEY: ${LOCCI_SECRET_KEY}
      DATABASE_URL: file:/data/locci.db         # keep the database on the volume
      DOCKER_HOST: unix:///var/run/docker.sock  # the socket mounted below
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - locci-docker-data:/data
volumes:
  locci-docker-data:
    name: locci-docker-data
```

Then `docker exec -it locci-docker locci user add admin --admin` and open
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
| [Agents](docs/AGENTS.md) | outbound agents for hosts behind NAT |
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

- The binary is pure Go and static (`CGO_ENABLED=0`), so it cross-compiles
  anywhere. Signed release binaries and images: [docs/RELEASING.md](docs/RELEASING.md).
- Behind Traefik, NGINX, Cloudflare, NetBird and the like: set
  `LOCCI_PUBLIC_URL`; see [docs/REVERSE_PROXY.md](docs/REVERSE_PROXY.md).
- Access to a Docker socket is root on that host. Serve locci over HTTPS and
  read [docs/SECURITY.md](docs/SECURITY.md).

## License

Licensed under either of

- MIT license ([MIT-LICENSE.md](MIT-LICENSE.md))
- Apache License, Version 2.0 ([APACHE-LICENSE.md](APACHE-LICENSE.md))

at your option. Unless you state otherwise, any contribution you submit for
inclusion is dual licensed as above, without additional terms or conditions.
