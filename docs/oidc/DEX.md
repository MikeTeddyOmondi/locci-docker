# Dex

[Dex](https://dexidp.io) is a small federating OIDC provider: it fronts
LDAP, GitHub, GitLab, SAML and others. It is also the way to sign in to locci
with **GitHub**, which is OAuth2 only. Tested with Dex 2.41.1 (web and CLI
sign-in, account creation).

## What locci needs

| Setting | Value |
|---|---|
| Issuer URL | Dex's `issuer`, such as `https://dex.example.com/dex` |
| Client ID / secret | a `staticClients` entry |
| Redirect URI | `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/dex/callback` |
| Scopes | `openid email profile`, plus `groups` with connectors that provide groups |

## Setup

Dex is configured with one file. A minimal one with a local test user:

```yaml
issuer: https://dex.example.com/dex
storage:
  type: sqlite3
  config:
    file: /var/dex/dex.db
web:
  http: 0.0.0.0:5556
oauth2:
  skipApprovalScreen: true
staticClients:
  - id: locci
    name: locci
    secret: change-this-client-secret
    redirectURIs:
      - https://locci.example.com/api/v1/auth/oidc/dex/callback
enablePasswordDB: true
staticPasswords:
  - email: admin@example.com
    # bcrypt of "password"; make your own with: htpasswd -bnBC 10 "" 'secret' | tr -d ':\n'
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: admin
    userID: 08a8684b-db88-4b73-90a9-3cd1661f5466
```

For GitHub, add a connector instead of (or next to) the password database:

```yaml
connectors:
  - type: github
    id: github
    name: GitHub
    config:
      clientID: $GITHUB_CLIENT_ID
      clientSecret: $GITHUB_CLIENT_SECRET
      redirectURI: https://dex.example.com/dex/callback
      orgs:
        - name: my-org          # members of these orgs only; teams become groups
```

## Scripted setup

```sh
#!/bin/sh
set -eu
LOCCI_URL=${LOCCI_URL:-http://localhost:8998}
cat > dex.yaml <<EOF
issuer: http://127.0.0.1:5556/dex
storage: {type: memory}
web: {http: 0.0.0.0:5556}
oauth2: {skipApprovalScreen: true}
staticClients:
  - id: locci
    name: locci
    secret: change-this-client-secret
    redirectURIs: ["$LOCCI_URL/api/v1/auth/oidc/dex/callback"]
enablePasswordDB: true
staticPasswords:
  - email: admin@example.com
    hash: "\$2a\$10\$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: admin
    userID: 08a8684b-db88-4b73-90a9-3cd1661f5466
EOF
# docker cp instead of a bind mount: works with Colima/remote Docker too.
docker create --name dex -p 5556:5556 ghcr.io/dexidp/dex:v2.41.1 dex serve /tmp/dex.yaml
docker cp dex.yaml dex:/tmp/dex.yaml
docker start dex

curl -fsS -X POST "$LOCCI_URL/api/v1/sso/providers" \
  -H "Authorization: Bearer $LOCCI_TOKEN" -H 'Content-Type: application/json' \
  -d '{"slug":"dex","display_name":"Dex","issuer_url":"http://127.0.0.1:5556/dex",
       "client_id":"locci","client_secret":"change-this-client-secret",
       "auto_create_users":true,"enabled":true}'
```

Sign in as `admin@example.com` / `password`.

## Notes

- Dex's static password users have no groups, so the admin-group mapping only
  applies with connectors that return groups (GitHub teams, LDAP, SAML); add
  `groups` to the scopes then.
- Emails from the password database are verified, so new users are named
  after their email.
