# GitLab

GitLab.com or a self-managed GitLab as the identity provider.

> Not tested end to end. Written from GitLab's OpenID Connect documentation
> (November 2026).

## What locci needs

| Setting | Value |
|---|---|
| Issuer URL | `https://gitlab.com`, or your instance's URL |
| Client ID / secret | an application with the `openid`, `email` and `profile` scopes, **Confidential** |
| Redirect URI | `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/gitlab/callback` |
| Scopes | `openid email profile` (default) |
| Groups claim | `groups` (from userinfo, full paths such as `my-org/ops`) |
| Admin group | a group path, such as `my-org/locci-admins` |

GitLab's ID token carries `groups_direct`; the userinfo endpoint carries
`groups` with every group the user belongs to, which locci reads when the ID
token has none. GitLab marks confirmed emails as verified.

## Manual setup

1. Choose where the application lives: a group (Settings, Applications) for
   one organization, or your user (Preferences, Applications), or instance-wide
   (Admin area, Applications) on self-managed GitLab.
2. Add an application named `locci`, Redirect URI as above, **Confidential**
   on, scopes `openid`, `email` and `profile`. Copy the application ID and
   secret.
3. In locci add the provider with Admin group set to the group path.

## Scripted setup

Self-managed GitLab, instance-wide application, with an admin's personal
access token (`api` scope):

```sh
#!/bin/sh
set -eu
GITLAB=${GITLAB:-https://gitlab.example.com}
LOCCI_URL=${LOCCI_URL:-https://locci.example.com}

APP=$(curl -fsS -X POST "$GITLAB/api/v4/applications" -H "PRIVATE-TOKEN: $GITLAB_ADMIN_TOKEN" \
  --data-urlencode name=locci \
  --data-urlencode "redirect_uri=$LOCCI_URL/api/v1/auth/oidc/gitlab/callback" \
  --data-urlencode "scopes=openid email profile" \
  --data-urlencode confidential=true)

curl -fsS -X POST "$LOCCI_URL/api/v1/sso/providers" \
  -H "Authorization: Bearer $LOCCI_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"slug\":\"gitlab\",\"display_name\":\"GitLab\",\"issuer_url\":\"$GITLAB\",
       \"client_id\":\"$(echo "$APP" | jq -r .application_id)\",
       \"client_secret\":\"$(echo "$APP" | jq -r .secret)\",
       \"admin_group\":\"my-org/locci-admins\",\"auto_create_users\":true,\"enabled\":true}"
```

## Device flow

Recent GitLab versions support the device authorization grant; if
`$GITLAB/.well-known/openid-configuration` lists a
`device_authorization_endpoint`, `locci login --sso gitlab --device` works.

## Notes

- On GitLab.com anyone with an account can sign in: set Allowed email domains,
  or turn off "Create accounts on first sign-in" and pre-create the users.
- GitHub (not GitLab) is OAuth2 only; use [Dex](DEX.md) in front of it.
