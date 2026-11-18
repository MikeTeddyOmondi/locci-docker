# Behind a reverse proxy or tunnel

locci works behind Traefik, NGINX, Caddy, Cloudflare (proxy or Tunnel),
NetBird, Tailscale and the like, with everything working: the web UI,
single sign-on, the console and log streams, the CLI and agents. Three
settings matter.

## The checklist

1. **Set `LOCCI_PUBLIC_URL`** to the address people open, such as
   `https://locci.example.com`. locci then takes the scheme and host from it
   for OIDC redirect URIs, the session cookie's `Secure` flag, HSTS and the
   console's origin check, whatever headers the proxy sends. It is the one
   setting that makes any proxy work.
2. **Pass WebSockets through** for `/api/v1/.../exec/ws` (the console) and
   `/api/v1/agent/connect` (agents). Most proxies do by default; NGINX needs
   the `Upgrade` headers below.
3. **Tell locci which proxy to believe** with `LOCCI_TRUSTED_PROXIES`, if the
   proxy isn't on a private network. The default trusts loopback, the
   private ranges (Docker networks, LANs) and `100.64.0.0/10` (NetBird and
   Tailscale). Only trusted proxies' `X-Forwarded-For` counts, which is what
   the audit log and the login rate limits use as the client address. Behind
   an untrusted proxy every client looks like the proxy, so they share one
   rate limit.

Also: don't let the proxy time out idle connections too early. The console
pings every 30 seconds and agents every 20, but a quiet `logs --follow` sends
nothing until a log line arrives.

## Traefik

Nothing special: Traefik passes WebSockets and the headers locci needs.

```yaml
services:
  locci:
    image: locci/docker:latest
    environment:
      LOCCI_PUBLIC_URL: https://locci.example.com
    labels:
      traefik.enable: "true"
      traefik.http.routers.locci.rule: Host(`locci.example.com`)
      traefik.http.routers.locci.tls.certresolver: letsencrypt
      traefik.http.services.locci.loadbalancer.server.port: "8998"
```

## NGINX

```nginx
map $http_upgrade $connection_upgrade { default upgrade; '' close; }

server {
  listen 443 ssl;
  server_name locci.example.com;

  location / {
    proxy_pass http://127.0.0.1:8998;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
    proxy_read_timeout 1h;   # log streams can be quiet for a while
    proxy_buffering off;     # locci also sends X-Accel-Buffering: no on streams
  }
}
```

Forgetting `proxy_set_header Host` still works: locci accepts the console's
origin from `X-Forwarded-Host` or `LOCCI_PUBLIC_URL`.

## Caddy

```
locci.example.com {
  reverse_proxy 127.0.0.1:8998 {
    flush_interval -1
  }
}
```

## Cloudflare Tunnel (cloudflared)

Point a public hostname at `http://locci:8998` (or `http://localhost:8998`).
cloudflared connects from the same host or Docker network, which locci
trusts by default; WebSockets are on by default. Set `LOCCI_PUBLIC_URL` to
the public hostname. Cloudflare closes connections idle for 100 seconds,
which the console's pings stay under.

Cloudflare Access in front works with the web UI. The CLI and agents then
need a service token or a bypass rule for `/api/v1/` paths, since they can't
do Access's browser login.

## Cloudflare proxy (orange cloud, no tunnel)

Requests arrive from Cloudflare's public addresses, so trust those:

```sh
LOCCI_TRUSTED_PROXIES="$(curl -s https://www.cloudflare.com/ips-v4 | paste -sd, -),$(curl -s https://www.cloudflare.com/ips-v6 | paste -sd, -)"
LOCCI_PUBLIC_URL=https://locci.example.com
```

Turn on WebSockets for the zone (Network settings).

## NetBird, Tailscale and other overlays

- **Reaching locci directly over the overlay** (`http://100.x.y.z:8998` or a
  MagicDNS name): no proxy is involved. Set `LOCCI_PUBLIC_URL` to the
  address people use so OIDC redirect URIs match what the identity provider
  has.
- **NetBird's reverse proxy or `tailscale serve` / Funnel**: they forward
  from the overlay range or loopback, both trusted by default. Set
  `LOCCI_PUBLIC_URL` to the published address.

## Single sign-on behind a proxy

Register exactly `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/<slug>/callback` at the
identity provider; the provider's page in locci shows it. Without
`LOCCI_PUBLIC_URL` locci builds the redirect URI from the request, which is
right only if the proxy sends `X-Forwarded-Proto` and `X-Forwarded-Host` (or
the original `Host`) and is trusted.

The browser's session is an HttpOnly cookie (`SameSite=Lax`, `Secure` over
HTTPS) on the public host. A proxy must not rewrite `Set-Cookie` domains or
paths.

## Checking a setup

```sh
curl -sI https://locci.example.com/login | grep -i 'strict-transport\|content-security'
locci login --url https://locci.example.com -u admin && locci ps
```

Then open the web UI, sign in (with SSO if set up), open a container's
Console and follow its Logs.
