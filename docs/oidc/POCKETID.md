# Pocket ID

[Pocket ID](https://pocket-id.org) is a small, passkey-only OIDC provider.
Tested with Pocket ID 1.16.0: sign-in, account creation and the admin role
from a group.

## What locci needs

| Setting | Value |
|---|---|
| Issuer URL | Pocket ID's `APP_URL`, such as `https://id.example.com` (no path) |
| Client ID / secret | from an OIDC client (Pocket ID generates the ID) |
| Redirect URI | `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/pocketid/callback` |
| Scopes | **`openid email profile groups`**: Pocket ID only sends groups for the `groups` scope |

## Manual setup

1. **OIDC client:** Administration, OIDC Clients, Add.
   - Name `locci`, Callback URL: the redirect URI above.
   - PKCE on. Leave "Public client" off and copy the client ID and secret.
2. **Admin group:** Administration, User Groups, create `locci-admins` and add
   people.
3. **In locci**, set Scopes to `openid email profile groups` and Admin group
   to `locci-admins`.

## Scripted setup

Pocket ID signs in with passkeys, which a script can't create. This script
uses the first-run setup endpoint (it creates the initial admin and returns a
session, only on a fresh instance) to make an API key, then the API for the
rest, and Pocket ID's own CLI for a one-time login link. Add a passkey for the
admin afterwards.

```sh
#!/bin/bash
set -euo pipefail
LOCCI_URL=${LOCCI_URL:-http://localhost:8998}
PID_URL=${PID_URL:-http://127.0.0.1:1411}
REDIRECT="$LOCCI_URL/api/v1/auth/oidc/pocketid/callback"

docker run -d --name pocket-id -p 1411:1411 -e APP_URL="$PID_URL" \
  -e ENCRYPTION_KEY="$(openssl rand -base64 32)" ghcr.io/pocket-id/pocket-id:v1
until curl -fsS "$PID_URL/.well-known/openid-configuration" >/dev/null 2>&1; do sleep 1; done

# 1. Initial admin (fresh instance only) and an API key from its session.
JAR=$(mktemp)
curl -fsS -c "$JAR" -X POST "$PID_URL/api/signup/setup" -H 'Content-Type: application/json' \
  -d '{"username":"admin","email":"admin@example.com","firstName":"Admin","lastName":"User"}' >/dev/null
KEY=$(curl -fsS -b "$JAR" -X POST "$PID_URL/api/api-keys" -H 'Content-Type: application/json' \
  -d "{\"name\":\"setup\",\"expiresAt\":\"$(date -u -v+1d +%FT%TZ 2>/dev/null || date -u -d tomorrow +%FT%TZ)\"}" | jq -r .token)
api() { curl -fsS -H "X-API-KEY: $KEY" -H 'Content-Type: application/json' "$@"; }

# 2. A user, the admin group, and the OIDC client.
USER=$(api -X POST "$PID_URL/api/users" -d '{"username":"bob","email":"bob@example.com",
  "firstName":"Bob","lastName":"Ops","displayName":"Bob Ops","isAdmin":false}' | jq -r .id)
GROUP=$(api -X POST "$PID_URL/api/user-groups" -d '{"name":"locci-admins","friendlyName":"locci admins"}' | jq -r .id)
api -X PUT "$PID_URL/api/user-groups/$GROUP/users" -d "{\"userIds\":[\"$USER\"]}" >/dev/null
CLIENT=$(api -X POST "$PID_URL/api/oidc/clients" -d "{\"name\":\"locci\",
  \"callbackURLs\":[\"$REDIRECT\"],\"isPublic\":false,\"pkceEnabled\":true}" | jq -r .id)
SECRET=$(api -X POST "$PID_URL/api/oidc/clients/$CLIENT/secret" | jq -r .secret)

# 3. A one-time login link for bob (no passkey yet).
docker exec pocket-id /app/pocket-id one-time-access-token bob

echo "client id: $CLIENT"
echo "client secret: $SECRET"
```

Then add it to locci:

```sh
curl -fsS -X POST "$LOCCI_URL/api/v1/sso/providers" \
  -H "Authorization: Bearer $LOCCI_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"slug\":\"pocketid\",\"display_name\":\"Pocket ID\",\"issuer_url\":\"$PID_URL\",
       \"client_id\":\"$CLIENT\",\"client_secret\":\"$SECRET\",
       \"scopes\":\"openid email profile groups\",
       \"auto_create_users\":true,\"admin_group\":\"locci-admins\",\"enabled\":true}"
```

## Notes

- **Device flow:** `locci login --device` works when Pocket ID's discovery
  document lists a `device_authorization_endpoint`; otherwise use the
  browser sign-in.
- **Emails aren't marked verified** unless Pocket ID's email verification is
  on. So new locci users get the Pocket ID username (`bob`) rather than the
  email, existing accounts aren't linked by email, and an allowed-domains list
  refuses everyone. Leave the domain list empty, or turn on
  `EMAIL_VERIFICATION_ENABLED` in Pocket ID.
- Pocket ID shows a consent screen the first time; the user approves it once.
- `POST /api/users/:id/one-time-access-token` with an API key failed with a
  foreign-key error in 1.16.0; the `pocket-id one-time-access-token <user>`
  CLI works. Newer versions also accept a `STATIC_API_KEY` environment
  variable for declarative setups, which replaces step 1.
