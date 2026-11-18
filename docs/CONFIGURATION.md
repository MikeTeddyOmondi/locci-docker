# Configuration

`locci serve` reads its settings from environment variables. A `.env` file in
the working directory is loaded first if it exists; real environment variables
win over it. Copy [`.env.sample`](../.env.sample) to start.

## Server

| Variable | Default | Purpose |
|---|---|---|
| `SERVER_PORT` | `8998` | HTTP port for the API and the web UI |
| `GIN_MODE` | `release` | `debug` prints every route and request at startup |
| `DATABASE_URL` | `file:locci.db` | libsql database: a local file, or `libsql://<db>.turso.io?authToken=...` for Turso |
| `AUTH_TOKEN` | empty | Optional static admin token for machine-to-machine calls. Leave empty to require user logins only |
| `LOCCI_SECRET_KEY` | empty | 32 random bytes, base64. Encrypts the TLS and SSH keys of remote environments and OIDC client secrets, and seals the sign-in cookie. Generate with `openssl rand -base64 32` |
| `LOCCI_PUBLIC_URL` | empty | The address people open locci at, such as `https://locci.example.com`. Used for OIDC redirect URIs; without it they come from each request (`X-Forwarded-*` aware) |
| `DOCKER_HOST` | empty | Docker daemon for the `local` environment. Empty means the local socket. `DOCKER_TLS_VERIFY` and `DOCKER_CERT_PATH` are honoured too |
| `TRAEFIK_DOMAIN` | `locci.cloud` | Base domain for the Traefik labels put on created containers (`<subdomain>.<domain>`) |

### `LOCCI_SECRET_KEY`

- Without it the server runs normally and the `local` environment works.
  Adding a `tcp+tls` or `ssh` environment is refused (HTTP 412) with
  instructions.
- Keep it with your backups of the database. If it changes or is lost, stored
  keys can't be decrypted: those environments go `down` and their keys have to
  be entered again (Edit the environment and paste the key).
- It is a different thing from `AUTH_TOKEN`: one encrypts data at rest, the
  other is a login credential.

## Deploy jobs (RabbitMQ)

The consumer only starts when `RABBITMQ_URL` is set.

| Variable | Default | Purpose |
|---|---|---|
| `RABBITMQ_URL` | empty | `amqp://user:pass@host:5672/` |
| `QUEUE_NAME` | `deploy.trigger` | Durable queue to consume |
| `DEPLOY_JOB_TIMEOUT` | `1800` | Seconds a single job may run |

Jobs go to the default environment unless they set `deployConfig.host`; see
[ENVIRONMENTS.md](ENVIRONMENTS.md#deploy-jobs).

## CLI

The CLI keeps its own small config file, written by `locci login` and
`locci env use`:

- macOS: `~/Library/Application Support/locci/config.json`
- Linux: `~/.config/locci/config.json`
- Windows: `%AppData%\locci\config.json`

| Variable | Overrides |
|---|---|
| `LOCCI_URL` | the saved server URL |
| `LOCCI_TOKEN` | the saved token |
| `LOCCI_ENV` | the saved environment (the `-E/--environment` flag beats it) |
