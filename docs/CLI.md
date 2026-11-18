# CLI

The same `locci` binary is the server and the client. Client commands talk to
a locci server over its [HTTP API](API.md), so they work from any machine.

```sh
just install            # builds and installs to $GOBIN (~/go/bin)
locci --help            # every command, grouped into Server and Client
```

Global flags:

| Flag | Meaning |
|---|---|
| `-o json` | print raw JSON instead of tables (for scripts) |
| `-E NAME`, `--environment NAME` | run against that environment (Docker host) |

## Logging in

```sh
locci login --url https://locci.example.com -u mike     # prompts for the password
locci login --url http://localhost:8998 --token $AUTH_TOKEN
locci login --url https://locci.example.com --sso keycloak # single sign-on in your browser
locci login --url https://locci.example.com --sso keycloak --device  # no browser here: shows a code
locci whoami
locci logout                                             # revokes the token
```

`--sso` opens the browser at the server's sign-in page for that provider and
listens on `127.0.0.1` for the result (the URL is printed too, for machines
where no browser opens). Add `--device` on machines without a browser: it
prints a URL and a code to enter on any other device, then waits for the
approval. The provider names are the slugs shown on the
Sign-in page; see [OIDC.md](OIDC.md).

The URL and token are saved in the CLI config file (see
[CONFIGURATION.md](CONFIGURATION.md#cli)); `LOCCI_URL` and `LOCCI_TOKEN`
override them.

## Choosing the Docker host

Which environment a command uses, first match wins:

1. `-E NAME` / `--environment NAME`
2. `LOCCI_ENV`
3. the one saved with `locci env use NAME`
4. the server's default environment

```sh
locci env ls                       # * marks the one commands use
locci env use edge                 # save a choice
locci ps -E local                  # one command elsewhere
locci env use --clear              # follow the server default again
```

(The flag isn't `--env` because `locci run -e/--env` sets container
variables, like `docker run`.)

## Environments

```sh
locci env ls
locci env add NAME --kind ssh|tcp+tls|local|agent --url URL [--label TEXT]
              [--ssh-key FILE] [--tls-ca FILE --tls-cert FILE --tls-key FILE] [--no-test]
locci env edit NAME [--label TEXT] [--url URL] [--ssh-key FILE] [--tls-* FILE]
               [--reset-host-key] [--rotate-agent-token] [--no-test]
locci env test [NAME]              # reachable? prints Docker version, OS, CPUs, memory
locci env use NAME | --clear
locci env default NAME             # make it the server default (admin)
locci env rm NAME                  # admin; the host itself is not touched
```

`add` and `edit` test the connection first and print the SSH host key
fingerprint that gets pinned. For `--kind agent` (no URL), `add` prints the
agent token and the commands to start the agent instead.

## Agent (on a Docker host behind NAT)

```sh
LOCCI_AGENT_TOKEN=la_... locci agent --server https://locci.example.com
locci agent --server https://locci.example.com --docker-host unix:///run/user/1000/docker.sock
```

It dials out to the server, serves the local Docker over that connection,
reconnects with backoff, and exits when its token is refused (rotated or the
environment removed). Prefer `LOCCI_AGENT_TOKEN` over `--token` so the token
doesn't appear in process lists. `LOCCI_SERVER` can replace `--server`. See [ENVIRONMENTS.md](ENVIRONMENTS.md) for
preparing hosts.

## Containers

```sh
locci ps
locci run nginx:alpine --name web -p 80:8080 --network proxy-network -e KEY=value -v data:/data
locci run nginx:alpine --name web --no-start          # create only
locci start|stop|restart|kill|pause|unpause web [more...]
locci logs web --tail 100
locci logs -f web
locci exec web -- nginx -v                            # runs to completion
locci stats web
locci inspect web
locci rm web [more...]
```

`run` flags: `--name` (required), `--subdomain` (Traefik, defaults to the
name), `-p CONTAINER:HOST` (repeatable), `-e KEY=VALUE`, `-v SRC:DST`,
`--network`, `-l KEY=VALUE`, `--no-start`. Anything after the image is the
command.

For an interactive shell use the Console tab in the web UI.

## Images, networks, volumes

```sh
locci image ls | pull REF | rm REF... | prune
locci network ls | create NAME [--driver bridge] | rm NAME...
locci volume ls | create NAME | rm NAME...
locci info                                            # Docker host details
```

## Admin

```sh
locci audit                                           # recent changes made through the API
```

On the server host, directly against the database (no login needed):

```sh
locci user add mike --admin                           # prompts for the password
locci user ls
locci user passwd mike                                # also gives an SSO-only user a password
locci user rm mike
locci auth password-login on                          # break-glass if single sign-on is broken
```

From source, `just cli <args>` runs any of these without installing, for
example `just cli ps -E edge`.
