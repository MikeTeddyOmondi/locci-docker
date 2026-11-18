# Documentation

| Guide | What it covers |
|---|---|
| [USAGE.md](USAGE.md) | Using the web UI: dashboard, containers, images, networks, volumes, users, audit log |
| [ENVIRONMENTS.md](ENVIRONMENTS.md) | Managing several Docker hosts over TLS or SSH, and preparing those hosts |
| [AGENTS.md](AGENTS.md) | Outbound agents for hosts behind NAT: how they work, running one, tokens, troubleshooting |
| [CLI.md](CLI.md) | The `locci` command line: login, choosing a host, every command |
| [API.md](API.md) | The HTTP API: authentication, every route, payloads, backward compatibility |
| [DOCKER.md](DOCKER.md) | Running locci itself in Docker, and building the image |
| [CONFIGURATION.md](CONFIGURATION.md) | Every environment variable and the `.env` file |
| [SECURITY.md](SECURITY.md) | Tokens, stored secrets, SSH host key pinning, the Docker socket, the audit log |
| [OIDC.md](OIDC.md) | Single sign-on: how it works, user matching, troubleshooting |
| [oidc/](oidc/) | Provider guides: self-hosted [Keycloak](oidc/KEYCLOAK.md), [Authentik](oidc/AUTHENTIK.md), [Pocket ID](oidc/POCKETID.md), [Dex](oidc/DEX.md) (tested, with scripted setups); hosted [Google](oidc/GOOGLE.md), [Entra ID](oidc/ENTRA.md), [Zitadel](oidc/ZITADEL.md), [GitLab](oidc/GITLAB.md) |
| [DEVELOPMENT.md](DEVELOPMENT.md) | Working on locci: tooling, layout, tests, conventions |

Planned work lives in [`../BACKLOG.md`](../BACKLOG.md) and what has landed in
[`../CHANGELOG.md`](../CHANGELOG.md).
