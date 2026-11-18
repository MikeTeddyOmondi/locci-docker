# Google

Sign in with Google accounts or a Google Workspace domain.

> Not tested end to end (no Google Cloud project was available). Written from
> Google's OpenID Connect and device flow documentation (November 2026).

## What locci needs

| Setting | Value |
|---|---|
| Issuer URL | `https://accounts.google.com` |
| Client ID / secret | an OAuth client of type **Web application** |
| Redirect URI | `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/google/callback` (must be HTTPS, except `localhost`) |
| Scopes | `openid email profile` (default) |
| Allowed email domains | **your domain**, for example `example.com`. Without it, anyone with a Google account can sign in |

Google sends no group claim, so leave Admin group empty and give roles in
locci (Users page). Google marks emails verified, so new users are named after
their email, and an admin can pre-create `alice@example.com` to link an
existing account.

## Setup

1. In the [Google Cloud console](https://console.cloud.google.com/), pick or
   create a project.
2. **Google Auth Platform, Branding:** set the app name and support email.
   For a Workspace domain choose **Audience: Internal**, so only your domain's
   accounts can consent at all.
3. **Clients, Create client**, type **Web application**, name `locci`.
   Authorized redirect URI: the redirect URI above. Copy the client ID and
   secret.
4. In locci, Sign-in, Add provider: slug `google`, issuer
   `https://accounts.google.com`, the client ID and secret, Allowed email
   domains `example.com`, then enable it.

There is no scripted setup: Google only creates OAuth web clients in the
console (`gcloud` can't).

```sh
curl -fsS -X POST "$LOCCI_URL/api/v1/sso/providers" \
  -H "Authorization: Bearer $LOCCI_TOKEN" -H 'Content-Type: application/json' \
  -d '{"slug":"google","display_name":"Google","issuer_url":"https://accounts.google.com",
       "client_id":"<id>.apps.googleusercontent.com","client_secret":"<secret>",
       "auto_create_users":true,"allowed_email_domains":"example.com","enabled":true}'
```

## Device flow

Google only allows the device flow for clients of type **TVs and Limited
Input devices**, and only with the `openid`, `email` and `profile` scopes. To
offer `locci login --device`, create a second client of that type and add it
as another provider (for example slug `google-cli`, same issuer and domain
list). Its button on the login page won't work in a browser; the browser
sign-in uses the `google` provider.

## Notes

- Personal Gmail accounts can sign in unless you set allowed domains or use
  an Internal audience.
- Workspace groups aren't in Google's tokens; reading them needs the
  Directory API, which locci doesn't use.
