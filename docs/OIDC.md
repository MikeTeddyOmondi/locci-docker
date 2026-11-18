# Single sign-on (OIDC)

locci can sign people in through any OpenID Connect provider with discovery
(`/.well-known/openid-configuration`). It uses the authorization code flow with
PKCE; after the provider confirms the user, locci issues its normal `lc_`
token, so the API, CLI and audit log work exactly as with passwords.

Step-by-step guides, each with a manual and a scripted setup, tested against
the real provider:

| Provider | Guide | Device flow (`--device`) |
|---|---|---|
| Keycloak | [oidc/KEYCLOAK.md](oidc/KEYCLOAK.md) | yes, enable it on the client (tested) |
| Authentik | [oidc/AUTHENTIK.md](oidc/AUTHENTIK.md) | yes, set a device code flow on the brand |
| Pocket ID | [oidc/POCKETID.md](oidc/POCKETID.md) | check your version's discovery document |
| Dex | [oidc/DEX.md](oidc/DEX.md) | yes, register its `/device/callback` redirect |

Others with discovery (Zitadel, Authelia, Kanidm, Ory, Google, Microsoft Entra
ID, GitLab, Okta, Auth0) use the same settings: an issuer URL, a client ID and
secret, and the redirect URI. GitHub is OAuth2 only; put it behind Dex.

## What stays the default

Username and password login (and `AUTH_TOKEN`) stay the default and need no
setup. OIDC is off until an admin adds **and enables** a provider:

- With no provider enabled the login page shows only the password form,
  `GET /api/v1/auth/providers` returns an empty list and the OIDC routes
  answer 404.
- Turning password login off is a separate admin choice, refused while no
  provider is enabled. The last enabled provider can't be disabled or removed
  while it is off.
- Break-glass: on the server host, `locci auth password-login on` turns it
  back on directly in the database. `AUTH_TOKEN` keeps working regardless.

## Before you start

- Set `LOCCI_SECRET_KEY` (client secrets are encrypted with it, and it seals
  the sign-in cookie). Without it, adding a provider is refused with HTTP 412.
- Set `LOCCI_PUBLIC_URL` to the address people open locci at, such as
  `https://locci.example.com`. Redirect URIs are built from it; without it they
  come from each request (`X-Forwarded-Proto` and `X-Forwarded-Host` are
  honoured behind a proxy).
- The redirect URI to register at the provider is
  `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/<slug>/callback`. The Sign-in page
  shows it for each provider.

## Adding a provider

**Web UI:** Harbour master, **Sign-in**, **Add provider**. Fill in the issuer
URL and press **Discover** to check it, then the client ID and secret. Enable
it to show "Continue with ..." on the login page.

**API** (admin token):

```sh
curl -fsS -X POST "$LOCCI_URL/api/v1/sso/providers" \
  -H "Authorization: Bearer $LOCCI_TOKEN" -H 'Content-Type: application/json' \
  -d '{
    "slug": "keycloak",
    "display_name": "Keycloak",
    "issuer_url": "https://id.example.com/realms/main",
    "client_id": "locci",
    "client_secret": "...",
    "scopes": "openid email profile",
    "auto_create_users": true,
    "default_role": "user",
    "admin_group": "locci-admins",
    "allowed_email_domains": "example.com",
    "enabled": true
  }'
```

`POST /api/v1/sso/providers/test` with `{"issuer_url": "..."}` runs discovery
without saving. See [API.md](API.md#single-sign-on) for every route.

## How users are matched

On every sign-in, in this order:

1. **A linked identity:** the provider's subject was seen before, so it's the
   same locci user.
2. **An existing account named after the verified email:** if the ID token's
   `email` is verified and a locci user has exactly that username, the
   identity is linked to it. An admin can pre-create `alice@example.com` and
   let Alice sign in with SSO. Unverified emails never link.
3. **A new account** when "Create accounts on first sign-in" is on. Its
   username is the verified email, else the preferred username, else
   `<provider>-<subject>` (with `-2`, `-3`... on clashes). It has no
   password, signs in only through SSO, and shows an "SSO" badge on the Users
   page. `locci user passwd NAME` can give it one later.
4. Otherwise the sign-in is refused.

**Allowed email domains** (comma separated) admit only verified emails in
those domains, for linking and for new accounts.

**Admin group:** when set, members of that group (read from the `groups`
claim, or the claim you name) become admins and everyone else gets the
default role, on every sign-in, so removing someone from the group demotes
them at their next sign-in. Leave it empty to manage roles in locci instead.
Group names may be given with or without a leading `/` (Keycloak full paths).

Only enable providers you trust to verify emails: linking by email relies on
it.

## Signing in

- **Web:** "Continue with ..." on the login page. The token comes back in the
  URL fragment (`/login#token=...`), which browsers never send to servers,
  and is removed from the address bar at once.
- **CLI:** `locci login --url https://locci.example.com --sso keycloak` opens
  the browser and listens on `127.0.0.1` for the result.
- **CLI without a browser** (servers, SSH sessions):
  `locci login --url https://locci.example.com --sso keycloak --device` prints
  a URL and a code to enter on any other device (a phone works), then waits.
  This is the OAuth device flow (RFC 8628); the provider must offer it (its
  discovery has a `device_authorization_endpoint`) and it often has to be
  enabled on the client. The locci server talks to the provider on the CLI's
  behalf, so the client secret never leaves the server.

Every sign-in and failed attempt is in the audit log as `LOGIN`.

## Troubleshooting

| Message | Cause |
|---|---|
| `sign-in state doesn't match` or `session is missing` | the browser lost the `locci_sso` cookie (third-party cookie blocking on a different site, or more than 10 minutes passed). Start again; serve locci and the provider over HTTPS. |
| `only admits verified emails from allowed domains` | the provider doesn't mark the email verified (Authentik and Pocket ID by default), or it's another domain. |
| `no locci account is linked` | "Create accounts on first sign-in" is off and no user is named after the verified email. |
| `oidc: issuer URL provided to client ... did not match the issuer URL returned by provider` | the issuer URL differs from the provider's own (often a trailing slash or `localhost` versus `127.0.0.1`). Use exactly what the provider's discovery document says. |
| provider says `redirect_uri` is invalid | register the exact redirect URI shown on the Sign-in page. |
| `doesn't offer the device flow` | the provider has no device endpoint, or the device grant isn't enabled for the client. See the provider guide. |
