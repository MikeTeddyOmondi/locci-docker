# Keycloak

Tested with Keycloak 26.0.7: sign-in, account creation and the admin role from
a group, including demotion when the user leaves the group.

## What locci needs

| Setting | Value |
|---|---|
| Issuer URL | `https://<keycloak>/realms/<realm>` |
| Client ID / secret | a confidential OpenID Connect client |
| Redirect URI | `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/keycloak/callback` |
| Scopes | `openid email profile` (default) |
| Groups claim | `groups`, added by a group membership mapper |

## Manual setup

1. **Realm:** use an existing realm or create one (`locci` below).
2. **Client:** Clients, Create client.
   - Client type OpenID Connect, Client ID `locci`.
   - Client authentication **on** (confidential), Standard flow **on**.
   - Valid redirect URIs: the redirect URI above.
   - Advanced, "Proof Key for Code Exchange Code Challenge Method": `S256`.
   - Credentials tab: copy the client secret.
3. **Groups claim:** the client, Client scopes, `locci-dedicated`, Add mapper,
   By configuration, **Group Membership**:
   - Token Claim Name `groups`, Full group path **off**,
   - Add to ID token, access token and userinfo **on**.
4. **Admin group:** Groups, create `locci-admins`, and add the people who
   should be locci admins.
5. **Users** need an email marked verified (Users, the user, "Email verified")
   for linking by email and for allowed-domain lists.

## Scripted setup

Runs Keycloak in dev mode and configures it with `kcadm.sh` (inside the
container, so nothing to install). For production run Keycloak in production
mode behind HTTPS and only use the configuration part.

```sh
#!/bin/sh
set -eu
LOCCI_URL=${LOCCI_URL:-http://localhost:8998}
KC_PORT=${KC_PORT:-8180}
REDIRECT="$LOCCI_URL/api/v1/auth/oidc/keycloak/callback"

docker run -d --name keycloak -p "$KC_PORT:8080" \
  -e KC_BOOTSTRAP_ADMIN_USERNAME=admin -e KC_BOOTSTRAP_ADMIN_PASSWORD=change-me \
  quay.io/keycloak/keycloak:26.0.7 start-dev

docker exec -e REDIRECT="$REDIRECT" keycloak bash -c '
set -e
K=/opt/keycloak/bin/kcadm.sh
until $K config credentials --server http://localhost:8080 --realm master \
      --user admin --password change-me >/dev/null 2>&1; do sleep 2; done

$K create realms -s realm=locci -s enabled=true
CID=$($K create clients -r locci -i -s clientId=locci -s enabled=true \
  -s publicClient=false -s secret=change-this-client-secret -s standardFlowEnabled=true \
  -s "redirectUris=[\"$REDIRECT\"]" \
  -s "attributes.\"pkce.code.challenge.method\"=S256")
$K create clients/$CID/protocol-mappers/models -r locci \
  -s name=groups -s protocol=openid-connect -s protocolMapper=oidc-group-membership-mapper \
  -s "config.\"claim.name\"=groups" -s "config.\"full.path\"=false" \
  -s "config.\"id.token.claim\"=true" -s "config.\"access.token.claim\"=true" \
  -s "config.\"userinfo.token.claim\"=true"

GID=$($K create groups -r locci -i -s name=locci-admins)
USER=$($K create users -r locci -i -s username=alice -s email=alice@example.com \
  -s emailVerified=true -s firstName=Alice -s enabled=true)
$K set-password -r locci --username alice --new-password change-me-too
$K update users/$USER/groups/$GID -r locci -s realm=locci -s userId=$USER -s groupId=$GID -n
'
```

Then add it to locci (admin token in `LOCCI_TOKEN`):

```sh
curl -fsS -X POST "$LOCCI_URL/api/v1/sso/providers" \
  -H "Authorization: Bearer $LOCCI_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"slug\":\"keycloak\",\"display_name\":\"Keycloak\",
       \"issuer_url\":\"http://127.0.0.1:$KC_PORT/realms/locci\",
       \"client_id\":\"locci\",\"client_secret\":\"change-this-client-secret\",
       \"auto_create_users\":true,\"admin_group\":\"locci-admins\",
       \"allowed_email_domains\":\"example.com\",\"enabled\":true}"
```

## Notes

- Keycloak's issuer is derived from the hostname it is reached at. Use the
  same host for locci's issuer URL as browsers use, or set Keycloak's
  `KC_HOSTNAME`.
- With "Full group path" on, groups arrive as `/locci-admins`; locci accepts
  both forms.
- Roles follow the group on every sign-in. Keycloak keeps a session, so after
  changing groups the next "Continue with Keycloak" is instant and already
  carries the new groups.
