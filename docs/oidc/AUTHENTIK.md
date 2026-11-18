# Authentik

Tested with Authentik 2026.8.5: sign-in, account creation and the admin role
from a group.

## What locci needs

| Setting | Value |
|---|---|
| Issuer URL | `https://<authentik>/application/o/<application-slug>/` (with the trailing slash) |
| Client ID / secret | from an OAuth2/OpenID provider, client type Confidential |
| Redirect URI | `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/authentik/callback` |
| Scopes | `openid email profile` (default); `profile` carries `groups` |

## Manual setup

1. **Provider:** Applications, Providers, Create, **OAuth2/OpenID Provider**.
   - Authorization flow: `default-provider-authorization-implicit-consent`
     (or the explicit-consent one).
   - Client type **Confidential**; note the client ID and secret.
   - Redirect URIs: the redirect URI above, matching mode Strict.
   - Signing key: **authentik Self-signed Certificate** (or your own). locci
     verifies ID tokens with the provider's public keys, so a signing key is
     required.
   - Advanced: scopes `openid`, `email`, `profile`. Allowed grant types must
     include **authorization_code**.
2. **Application:** Applications, Create, slug `locci`, provider the one above.
   The issuer is `https://<authentik>/application/o/locci/`.
3. **Admin group:** Directory, Groups, create `locci-admins` and add people.
4. **Verified emails (optional):** Authentik's default `email` scope reports
   `email_verified: false`. To use locci's allowed-domain list or link
   existing accounts by email, add a Scope Mapping with scope name `email`:

   ```python
   return {"email": request.user.email, "email_verified": True}
   ```

   and use it on the provider instead of the default email mapping. Only do
   this if you control how emails get into Authentik.

## Scripted setup

Starts Authentik (Postgres, server, worker) and configures it through its API
with the bootstrap token. Recent Authentik versions no longer need Redis.

```sh
#!/bin/bash
set -euo pipefail
LOCCI_URL=${LOCCI_URL:-http://localhost:8998}
AK_PORT=${AK_PORT:-9100}
AK_TOKEN=change-this-bootstrap-token
REDIRECT="$LOCCI_URL/api/v1/auth/oidc/authentik/callback"

cat > authentik.env <<EOF
AUTHENTIK_POSTGRESQL__HOST=authentik-db
AUTHENTIK_POSTGRESQL__NAME=authentik
AUTHENTIK_POSTGRESQL__USER=authentik
AUTHENTIK_POSTGRESQL__PASSWORD=change-this-db-password
AUTHENTIK_SECRET_KEY=$(openssl rand -base64 48)
AUTHENTIK_BOOTSTRAP_PASSWORD=change-this-admin-password
AUTHENTIK_BOOTSTRAP_TOKEN=$AK_TOKEN
AUTHENTIK_BOOTSTRAP_EMAIL=admin@example.com
EOF

docker network create authentik
docker run -d --name authentik-db --network authentik -e POSTGRES_DB=authentik \
  -e POSTGRES_USER=authentik -e POSTGRES_PASSWORD=change-this-db-password postgres:16-alpine
docker run -d --name authentik-server --network authentik --shm-size 512m \
  -p "$AK_PORT:9000" --env-file authentik.env ghcr.io/goauthentik/server:2026.8.5 server
docker run -d --name authentik-worker --network authentik --shm-size 512m \
  --env-file authentik.env ghcr.io/goauthentik/server:2026.8.5 worker

A="http://127.0.0.1:$AK_PORT/api/v3"
api() { curl -fsS -H "Authorization: Bearer $AK_TOKEN" -H 'Content-Type: application/json' "$@"; }

# The worker applies the bootstrap token and default mappings shortly after start.
until api "$A/propertymappings/provider/scope/?page_size=100" 2>/dev/null \
      | jq -e '[.results[] | select(.scope_name=="profile")] | length > 0' >/dev/null; do sleep 3; done

flow() { api "$A/flows/instances/?slug=$1" | jq -r '.results[0].pk'; }
AUTHZ=$(flow default-provider-authorization-implicit-consent)
INVAL=$(flow default-provider-invalidation-flow)
KEY=$(api "$A/crypto/certificatekeypairs/?has_key=true" | jq -r '.results[] | select(.name|test("Self-signed")) | .pk' | head -1)
OPENID_PROFILE=$(api "$A/propertymappings/provider/scope/?page_size=100" \
  | jq -c '[.results[] | select(.managed and (.scope_name=="openid" or .scope_name=="profile")) | .pk]')
EMAIL=$(api -X POST "$A/propertymappings/provider/scope/" -d '{"name":"locci: email (verified)","scope_name":"email",
  "expression":"return {\"email\": request.user.email, \"email_verified\": True}"}' | jq -r .pk)

PROVIDER=$(api -X POST "$A/providers/oauth2/" -d "{\"name\":\"locci\",\"authorization_flow\":\"$AUTHZ\",
  \"invalidation_flow\":\"$INVAL\",\"client_type\":\"confidential\",\"client_id\":\"locci\",
  \"client_secret\":\"change-this-client-secret\",\"signing_key\":\"$KEY\",
  \"grant_types\":[\"authorization_code\",\"refresh_token\"],
  \"redirect_uris\":[{\"matching_mode\":\"strict\",\"url\":\"$REDIRECT\"}],
  \"property_mappings\":$(jq -c --arg e "$EMAIL" '. + [$e]' <<<"$OPENID_PROFILE")}" | jq -r .pk)
api -X POST "$A/core/applications/" -d "{\"name\":\"locci\",\"slug\":\"locci\",\"provider\":$PROVIDER}" >/dev/null

GROUP=$(api -X POST "$A/core/groups/" -d '{"name":"locci-admins"}' | jq -r .pk)
USER=$(api -X POST "$A/core/users/" -d "{\"username\":\"carol\",\"name\":\"Carol\",
  \"email\":\"carol@example.com\",\"groups\":[\"$GROUP\"],\"is_active\":true}" | jq -r .pk)
api -X POST "$A/core/users/$USER/set_password/" -d '{"password":"change-me-too"}'
```

Then add it to locci:

```sh
curl -fsS -X POST "$LOCCI_URL/api/v1/sso/providers" \
  -H "Authorization: Bearer $LOCCI_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"slug\":\"authentik\",\"display_name\":\"Authentik\",
       \"issuer_url\":\"http://127.0.0.1:$AK_PORT/application/o/locci/\",
       \"client_id\":\"locci\",\"client_secret\":\"change-this-client-secret\",
       \"auto_create_users\":true,\"admin_group\":\"locci-admins\",
       \"allowed_email_domains\":\"example.com\",\"enabled\":true}"
```

## Notes

- **Device flow:** Authentik needs a device code flow set on the brand
  (System, Brands, the brand, "Device code flow", for example
  `default-provider-authorization-implicit-consent`). Users then enter the
  code at `https://<authentik>/device`.
- **Allowed grant types:** a provider created through the API in recent
  versions starts with none, and the sign-in fails with "The request is
  otherwise malformed" (Authentik logs "Invalid grant_type for provider").
  Include `authorization_code`.
- **Without the verified-email mapping** and with an allowed-domain list,
  locci refuses the sign-in with "only admits verified emails". Without a
  domain list, new users get their preferred username instead of the email.
- The issuer ends with `/`. Use exactly what **Discover** reports.
- Authentik's own port 9000 often clashes with other tools; any host port
  works.
