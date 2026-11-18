# Agents (outbound Docker hosts)

An **agent** connects a Docker host to a locci server from the host's side.
The host dials out; the server never connects in. Use it for hosts behind
NAT, a firewall or a home router, or anywhere you'd rather not open a Docker
or SSH port.

An agent environment then works like any other: containers, images,
networks, volumes, logs, stats, the console, deploy jobs.

## How it works

```
 Docker host                                   locci server
 ┌──────────────────────────┐                  ┌───────────────────────────┐
 │ locci agent              │  wss://...       │ /api/v1/agent/connect     │
 │   dials out, one         │ ───────────────► │   checks the agent token  │
 │   WebSocket              │                  │                           │
 │                          │  yamux streams   │ every Docker call for     │
 │ each stream is piped  ◄──┼──────────────────┼── this environment opens  │
 │ into the Docker socket   │                  │   a stream in the tunnel  │
 └──────────────────────────┘                  └───────────────────────────┘
```

1. The agent opens one WebSocket to `/api/v1/agent/connect` with
   `Authorization: Bearer la_...`. The server looks up which environment the
   token belongs to and tells the agent (`X-Locci-Environment`).
2. The WebSocket carries a [yamux](https://github.com/hashicorp/yamux)
   session. The server is the side that opens streams: every Docker API
   connection the server makes for this environment (a list, a log stream,
   a console) becomes one stream.
3. The agent copies each stream into its local Docker socket and back. It
   doesn't interpret the Docker API; it is a pipe to the socket.
4. yamux pings every 20 seconds, which also keeps proxies from closing an
   idle tunnel.
5. If the connection drops, the agent reconnects with backoff (1 second,
   doubling to 30, back to 1 after a minute of uptime). If the server
   refuses the token (`401`), the agent stops for good: the token was
   rotated or the environment removed, and retrying won't help.
6. The server shows the environment as up as soon as the tunnel connects,
   and as down when it closes, through the same 30-second health loop as
   other environments.

A second agent with the same token replaces the first one's tunnel.

## Set up

### 1. Create the environment and get a token

**Web UI:** Environments, **Add environment**, kind **Agent (outbound)**.
The token (`la_...`) is shown once in a dialog, with a ready-to-run
command; locci keeps only its hash.

**CLI** (admin):

```sh
locci env add edge --kind agent --label "Edge box"
```

It prints the token and the commands below.

### 2. Run the agent on the Docker host

The agent is the same `locci` binary, or the same image, as the server.

**Docker:**

```sh
docker run -d --name locci-agent --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e LOCCI_AGENT_TOKEN=la_... \
  locci/docker agent --server https://locci.example.com
```

No ports and no volume are needed.

**Compose:**

```yaml
services:
  locci-agent:
    image: locci/docker:latest
    command: ["agent", "--server", "https://locci.example.com"]
    restart: unless-stopped
    environment:
      LOCCI_AGENT_TOKEN: ${LOCCI_AGENT_TOKEN}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

**Binary with systemd** (`/etc/systemd/system/locci-agent.service`):

```ini
[Unit]
Description=locci agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
EnvironmentFile=/etc/locci-agent.env
ExecStart=/usr/local/bin/locci agent
Restart=always
RestartSec=5
# The user must be able to use the Docker socket (root, or in the docker group).
User=root

[Install]
WantedBy=multi-user.target
```

with `/etc/locci-agent.env` (mode `0600`):

```sh
LOCCI_SERVER=https://locci.example.com
LOCCI_AGENT_TOKEN=la_...
```

then `systemctl enable --now locci-agent`.

### 3. Check it

Within a few seconds the environment shows as up in the switcher, on the
Environments page and in `locci env ls` (URL column: `(outbound agent)`).
Then `locci ps -E edge` lists that host's containers.

## Options

| Flag | Environment | Default | What |
|---|---|---|---|
| `--server` | `LOCCI_SERVER` | (required) | the locci server's URL, `https://...` (`http://` for testing) |
| `--token` | `LOCCI_AGENT_TOKEN` | (required) | the agent token; prefer the variable, since flags show in process lists |
| `--docker-host` | `DOCKER_HOST` | `unix:///var/run/docker.sock` | the Docker to expose: `unix:///path.sock` (rootless Docker, Podman's socket) or `tcp://127.0.0.1:2375` |

The agent honours `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY` for reaching the
server through an outbound proxy.

## Tokens

- Only the token's SHA-256 hash is stored, and it's shown once.
- **Rotate** it if it may have leaked: the environment's menu, **Rotate agent
  token**, or `locci env edit edge --rotate-agent-token`. The running agent
  is disconnected at once and stops (it gets `401`); restart it with the new
  token.
- **Removing** the environment disconnects the agent and invalidates the
  token.
- Connects and disconnects are in the audit log as `agent:<name>`.

## Security

- The token can do one thing: serve that environment's Docker to the server.
  It can't call the API, and the agent accepts nothing from anyone but the
  server it dialled.
- The agent verifies the server's TLS certificate (normal HTTPS rules), so
  run the server with a valid certificate. It only talks to the server it
  was pointed at.
- Whoever controls the locci server controls every connected host's Docker,
  which is root on those hosts. That's the same for every environment kind;
  protect the server accordingly ([SECURITY.md](SECURITY.md)).
- Plain users' limits apply to agent environments as to any other, and the
  agent's own container is protected (the image is labelled
  `io.locci.protected=true`), so plain users can't stop it or open a console
  in it.
- An admin can limit who may use the environment
  ([ENVIRONMENTS.md](ENVIRONMENTS.md#who-can-use-an-environment)).

## Behind proxies

The server side needs WebSockets passed through for
`/api/v1/agent/connect`; see [REVERSE_PROXY.md](REVERSE_PROXY.md). Nothing
special is needed on the agent's side beyond outbound HTTPS. Cloudflare
Access in front of the server needs a bypass for `/api/v1/agent/connect`
(the agent can't do Access's browser login).

## Troubleshooting

| The agent logs | Meaning |
|---|---|
| `connected server=... environment=edge` | working |
| `the server refused the agent token` and exits | the token was rotated or the environment removed: get a new token |
| `tunnel down, reconnecting err="websocket: bad handshake"` | the server (or a proxy) answered without upgrading to a WebSocket: wrong `--server` URL, or a proxy not passing WebSockets |
| `tunnel down ... x509: ...` | the server's TLS certificate isn't trusted (self-signed, wrong name) |
| `tunnel down ... connection refused` or `i/o timeout` | the server isn't reachable from this host |

| locci shows | Meaning |
|---|---|
| environment stays `down` | no agent connected: check the agent's logs |
| up, but calls fail with `permission denied` on the socket | the agent's user can't use the Docker socket (add it to the `docker` group or run as root) |
| up, but calls fail with `no such file` | `--docker-host` points at a socket that doesn't exist (rootless Docker: `unix:///run/user/1000/docker.sock`) |

## Limits

- One server per agent. To connect a host to two servers, run two agents.
- The tunnel carries the Docker API only; it isn't a general network
  tunnel to the host.
