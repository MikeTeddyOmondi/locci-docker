# Microsoft Entra ID

Sign in with work or school accounts from your Entra ID (Azure AD) tenant.

> Not tested end to end (no Entra tenant was available). Written from
> Microsoft's identity platform documentation (November 2026).

## What locci needs

| Setting | Value |
|---|---|
| Issuer URL | `https://login.microsoftonline.com/<tenant-id>/v2.0` (your tenant's ID, not `common`) |
| Client ID / secret | an app registration with a client secret |
| Redirect URI | `<LOCCI_PUBLIC_URL>/api/v1/auth/oidc/entra/callback`, platform **Web** |
| Scopes | `openid email profile` (default) |
| Admin group | the group's **Object ID** (a GUID), not its name |

Use the tenant-specific issuer: with `common` or `organizations` the ID
token's issuer contains each user's tenant ID and doesn't match. The tenant
issuer also limits sign-in to your tenant.

**Emails:** Entra's `email` claim is optional, unverified and can change, so
locci treats it as unverified. New users get their user principal name
(`alice@contoso.com`) as username, existing accounts aren't linked by email,
and the allowed-domains list would refuse everyone, so leave it empty and rely
on the tenant issuer.

## Manual setup

1. Microsoft Entra admin center, **App registrations, New registration**.
   - Name `locci`, supported account types: this organizational directory
     only.
   - Redirect URI: platform **Web**, the redirect URI above.
2. **Certificates & secrets, New client secret**; copy the value (shown once)
   and note when it expires.
3. **Token configuration, Add groups claim**: Security groups. The ID token's
   `groups` claim then lists group object IDs.
4. Optional: **Token configuration, Add optional claim**, ID, `email`.
5. Copy the **Directory (tenant) ID** and **Application (client) ID** from
   Overview.

## Scripted setup

With the Azure CLI (`az login` as someone who can create app registrations):

```sh
#!/bin/sh
set -eu
LOCCI_URL=${LOCCI_URL:-https://locci.example.com}
TENANT=$(az account show --query tenantId -o tsv)

APP=$(az ad app create --display-name locci --sign-in-audience AzureADMyOrg \
  --web-redirect-uris "$LOCCI_URL/api/v1/auth/oidc/entra/callback" \
  --query appId -o tsv)
az ad app update --id "$APP" --set groupMembershipClaims=SecurityGroup
SECRET=$(az ad app credential reset --id "$APP" --display-name locci --years 1 --query password -o tsv)
az ad sp create --id "$APP" >/dev/null   # the enterprise app users sign in to

ADMINS=$(az ad group create --display-name locci-admins --mail-nickname locci-admins --query id -o tsv)
echo "Add admins with: az ad group member add --group $ADMINS --member-id <user object id>"

curl -fsS -X POST "$LOCCI_URL/api/v1/sso/providers" \
  -H "Authorization: Bearer $LOCCI_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"slug\":\"entra\",\"display_name\":\"Microsoft\",
       \"issuer_url\":\"https://login.microsoftonline.com/$TENANT/v2.0\",
       \"client_id\":\"$APP\",\"client_secret\":\"$SECRET\",
       \"auto_create_users\":true,\"admin_group\":\"$ADMINS\",\"enabled\":true}"
```

## Device flow

Entra treats the device flow as a public-client flow and refuses requests
that carry a client secret for public clients. To offer
`locci login --device`, create a second app registration with **Allow public
client flows: Yes** and no secret, and add it as another provider without a
client secret (for example slug `entra-cli`, same tenant issuer and admin
group). Its button on the login page won't work in a browser.

## Notes

- Client secrets expire (at most 2 years). Put a reminder in your calendar,
  then **Edit** the provider in locci with the new secret.
- Users in more than 200 groups get a group overage marker instead of the
  list, and the admin group can't be read; assign the app only the groups
  that matter (Token configuration, "Groups assigned to the application").
