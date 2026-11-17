# locci

A self-hosted Docker manager: API server, web UI and CLI in **one Go binary**.

- **API**: Gin, `/api/v1`, bearer-token auth (user tokens or a static `AUTH_TOKEN`)
- **Web UI**: Svelte 5 / SvelteKit, embedded with `go:embed`
- **CLI**: Cobra, talks to the same API
- **Storage**: libsql (local file or Turso) for users, tokens and the audit log
- **Deploy jobs**: optional RabbitMQ consumer

## Quick start

```sh
cp .env.sample .env
just web-install && just all          # needs Node 22+, Go 1.24+, a C compiler
./locci user add admin --admin        # prompts for a password
./locci serve                         # http://localhost:8998
```

From another terminal or machine:

```sh
locci login --url http://localhost:8998 -u admin
locci ps
locci run nginx:alpine --name web -p 80:8080 --network proxy-network
locci logs -f web
locci exec web -- nginx -v
locci stats web
```

`locci --help` lists every command; `-o json` prints raw JSON.

## Layout

```
cmd/locci            main.go → internal/cli
internal/cli         cobra commands (server + client)
internal/client      HTTP client used by the CLI
internal/config      env / .env settings
internal/docker      the only package that talks to Docker
internal/jobs        RabbitMQ deploy consumer
internal/server      Gin router, handlers, auth, audit, exec WebSocket
internal/store       libsql: users, tokens, audit log
internal/ui          embeds internal/ui/dist (built from web/)
web/                 SvelteKit app
```

## API

Original v1 routes are unchanged:
`POST/GET /networks`, `POST/GET /containers`, `POST /containers/start`, `DELETE /containers`.

Added:

| Area | Routes |
|---|---|
| Auth | `POST /auth/login`, `GET /auth/me`, `POST /auth/logout` |
| Containers | `GET/DELETE /containers/:id`, `POST /containers/:id/{start,stop,restart,kill,pause,unpause}`, `GET /containers/:id/logs?follow=true&tail=200`, `GET /containers/:id/stats`, `POST /containers/:id/exec`, `GET /containers/:id/exec/ws` |
| Images | `GET /images`, `POST /images/pull`, `POST /images/prune`, `DELETE /images/:id` |
| Networks | `GET/DELETE /networks/:id` |
| Volumes | `GET/POST /volumes`, `DELETE /volumes/:name` |
| System | `GET /system/info` |
| Admin | `GET/POST /users`, `DELETE /users/:name`, `GET /audit` |

## Notes

- go-libsql uses cgo: build with `CGO_ENABLED=1` on glibc (the Dockerfile handles this).
- Mounting the Docker socket gives root on the host. Keep the server behind TLS and use strong passwords.
