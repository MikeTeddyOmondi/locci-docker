# Single sign-on (OIDC)

**Status: planned, not implemented yet.** This page describes how it will
work; the full design and task list are in
[`../BACKLOG.md`](../BACKLOG.md#2-single-sign-on-with-oidc).

## What stays the default

Username and password login (and `AUTH_TOKEN`) stay the default and need no
setup. OIDC is off until an admin adds and enables a provider:

- With no provider enabled, the login page shows only the password form,
  `GET /api/v1/auth/providers` returns an empty list and the OIDC routes
  answer 404.
- Turning password login off is a separate, explicit admin choice. `locci user`
  on the server host and `AUTH_TOKEN` remain as break-glass access.

## How it will work

- Any provider with OpenID Connect discovery
  (`/.well-known/openid-configuration`): Keycloak, Authentik, Zitadel,
  Authelia, Dex, Kanidm, Ory, Google, Microsoft Entra ID, GitLab, Okta, Auth0.
  GitHub is OAuth2 only; use it through Dex.
- Admins add providers in the UI or API: issuer URL, client ID, client secret
  (encrypted with `LOCCI_SECRET_KEY`, like environment keys), scopes, whether
  to create users on first login, allowed email domains, and a group that
  grants admin.
- Sign-in uses the authorization code flow with PKCE. After the provider
  confirms the user, locci issues its normal `lc_` token, so the API, CLI and
  audit log work exactly as with passwords.
- The CLI will get `locci login --sso <provider>` (a browser flow with a local
  callback) and later `--device` for headless machines.

## Provider setup

Step-by-step guides for Keycloak, Authentik, Zitadel, Google and Entra ID will
be added here when the feature lands. Each will need the same three things
from the provider: an issuer URL, a client ID and secret, and the redirect URI
`https://<your-locci>/api/v1/auth/oidc/<provider>/callback`.
