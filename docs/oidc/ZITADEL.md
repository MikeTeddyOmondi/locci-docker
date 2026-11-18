# Zitadel

Zitadel Cloud or a self-hosted Zitadel.

> Not tested end to end. Written from Zitadel's documentation (November 2026);
> locci's handling of Zitadel's role claim is covered by unit tests.

## What locci needs

| Setting | Value |
|---|---|
| Issuer URL | your instance, such as `https://my-instance.zitadel.cloud` or your custom domain |
| Client ID / secret | a **Web** application with authentication method **Basic** (or **Post**) |
| Redirect URI | `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/zitadel/callback` |
| Scopes | `openid email profile` (default) |
| Groups claim | `urn:zitadel:iam:org:project:roles` |
| Admin group | a **project role** key, such as `locci-admin` |

Zitadel sends roles as an object keyed by role name
(`{"locci-admin": {"<org id>": "<org domain>"}}`); locci reads the keys.
Zitadel marks emails verified once users confirm them.

## Setup

1. **Project:** Projects, Create, name `locci`. In its General settings tick
   **Assert Roles on Authentication**.
2. **Roles:** in the project, Roles, add `locci-admin` (and any others).
3. **Application:** in the project, New, type **Web**, authentication method
   **Code** with **Basic** (or **Post**). Redirect URIs: the redirect URI above
   (for plain `http://` during testing turn on Development mode). Copy the
   client ID and secret.
4. In the application's **Token settings**, tick **User roles inside ID
   Token**. (Alternatively locci reads them from userinfo, which Assert Roles
   on Authentication enables.)
5. **Authorizations:** give the people who should be locci admins the
   `locci-admin` role on the project.
6. In locci add the provider with Groups claim
   `urn:zitadel:iam:org:project:roles` and Admin group `locci-admin`.

```sh
curl -fsS -X POST "$LOCCI_URL/api/v1/sso/providers" \
  -H "Authorization: Bearer $LOCCI_TOKEN" -H 'Content-Type: application/json' \
  -d '{"slug":"zitadel","display_name":"Zitadel","issuer_url":"https://my-instance.zitadel.cloud",
       "client_id":"<client id>","client_secret":"<secret>",
       "groups_claim":"urn:zitadel:iam:org:project:roles","admin_group":"locci-admin",
       "auto_create_users":true,"enabled":true}'
```

For repeatable setups Zitadel has an official Terraform provider
(`zitadel/zitadel`) with `zitadel_project`, `zitadel_project_role`,
`zitadel_application_oidc` and `zitadel_user_grant` resources.

## Device flow

Zitadel supports the device authorization grant. Enable the **Device Code**
grant type on the application, then use `locci login --sso zitadel --device`.

## Notes

- Project-scoped roles only appear for users with an authorization on that
  project.
- Without Assert Roles on Authentication (or roles in the ID token) the claim
  is missing and nobody becomes admin through the group.
