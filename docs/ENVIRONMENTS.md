# Environments (multiple Docker hosts)

An **environment** is one Docker host that locci manages. Every page, API
route and CLI command works the same against any of them; you only choose
which one.

On first start locci creates `local`: the Docker host from `DOCKER_HOST`, or
the local socket. It is the **default** environment, which the original API
routes, deploy jobs without a `host` and new browsers use.

## Connection kinds

| Kind | URL | Credentials | Use when |
|---|---|---|---|
| `local` | empty, `unix:///path.sock` or `tcp://host:2375` | none | the socket is reachable from the server itself |
| `tcp+tls` | `tcp://host:2376` | CA certificate, client certificate, client key | the host exposes Docker's TLS port |
| `ssh` | `ssh://user@host[:port][/socket/path]` | private key | you'd rather not expose any Docker port |

Keys (the TLS client key, the SSH private key) are encrypted with
`LOCCI_SECRET_KEY` before they are stored and are never returned by the API.
Set that variable before adding a remote host; see
[CONFIGURATION.md](CONFIGURATION.md#locci_secret_key).

## Adding a host

**Web UI:** Environments, then **Add environment**. Pick the kind, fill in the
URL and paste (or load from a file) the certificates or key. **Test
connection** shows the Docker version, OS, CPUs, memory, container count and,
for SSH, the host key fingerprint. Save when it looks right.

**CLI:**

```sh
locci env add edge --kind ssh --url ssh://deploy@10.0.0.5 --ssh-key ~/.ssh/locci_deploy
locci env add lab --kind tcp+tls --url tcp://10.0.0.6:2376 \
  --tls-ca ca.pem --tls-cert cert.pem --tls-key key.pem --label "Lab box"
```

`env add` tests first and refuses an unreachable host unless you pass
`--no-test`.

Names are lowercase letters, digits and dashes, are used in URLs
(`/e/edge/containers`) and in the CLI (`-E edge`), and can't be changed later.
`test` is reserved.

## Preparing an SSH host

locci speaks SSH itself (`golang.org/x/crypto/ssh`); it doesn't need an `ssh`
binary, `~/.ssh` or an agent, so it works the same inside the container image.

1. Create a dedicated, **unencrypted** key pair for locci:
   `ssh-keygen -t ed25519 -N '' -f locci_deploy -C locci`
2. On the host, add `locci_deploy.pub` to the `authorized_keys` of a user who
   can use the Docker socket (in the `docker` group, or root).
3. Make sure sshd allows forwarding to a Unix socket. In `/etc/ssh/sshd_config`:

   ```
   AllowStreamLocalForwarding yes
   AllowTcpForwarding yes
   ```

   Both default to yes in OpenSSH, but many hardened or minimal images
   (Alpine, for one) set `AllowTcpForwarding no`. Reload sshd after changing
   them. When forwarding is refused, locci's error says so.
4. Add the environment with the private key `locci_deploy`.

**Rootless Docker or a non-standard socket:** put the socket path after the
host, for example `ssh://me@host/run/user/1000/docker.sock`. The default is
`/var/run/docker.sock`.

### Host key pinning

The first successful connection (a test, or saving when the host is
reachable) records the host's SSH key; every later connection must present the
same key, or it fails with `ssh host key changed`. The UI and `env add` show
the fingerprint as `ecdsa-sha2-nistp256 SHA256:...`, the same form as
`ssh-keygen -lf /etc/ssh/ssh_host_ecdsa_key.pub` on the host, so you can
compare them.

- Changing an environment's URL drops the pin; the new host's key is pinned
  next.
- If a host was rebuilt on purpose, forget the pin: in the UI's Edit dialog
  ("Pinned host key" switch) or `locci env edit NAME --reset-host-key`.

## Preparing a TLS host

The host's dockerd must listen on TLS with client verification
(`--tlsverify --tlscacert ca.pem --tlscert server-cert.pem --tlskey
server-key.pem -H tcp://0.0.0.0:2376`), and you need a client certificate
signed by the same CA. Docker's guide "Protect the Docker daemon socket" walks
through creating them with `openssl`.

The server certificate must be valid for the name or IP in the URL you give
locci.

**Trying it locally with Docker-in-Docker:**

```sh
docker run -d --privileged --name dind -p 23760:2376 \
  -e DOCKER_TLS_CERTDIR=/certs -e DOCKER_TLS_SAN=DNS:localhost,IP:127.0.0.1 \
  docker:28-dind
docker cp dind:/certs/client ./dind-certs     # ca.pem, cert.pem, key.pem
locci env add lab --kind tcp+tls --url tcp://127.0.0.1:23760 \
  --tls-ca dind-certs/ca.pem --tls-cert dind-certs/cert.pem --tls-key dind-certs/key.pem
```

## Choosing a host

- **Web UI:** the switcher at the top of the sidebar. The choice is in the URL
  (`/e/<name>/...`), so links and reloads keep it; the browser also remembers
  the last one. Old links such as `/containers/<id>` open in that remembered
  (or the default) environment.
- **CLI:** `-E NAME` on any command, `LOCCI_ENV=NAME`, or `locci env use NAME`
  to save it (`locci env use --clear` to follow the server default again).
- **API:** prefix any Docker route with `/environments/<name>`; see
  [API.md](API.md).

## Health

Every 30 seconds the server pings every environment and records `up` or
`down` and when it was last seen. The sidebar dots, the Environments page and
`locci env ls` show it. A host that fails is reconnected from scratch on the
next request.

## Editing, the default, removing

- **Edit** (UI menu or `locci env edit`) changes the label, URL, certificates
  or key. Leave a key blank to keep the stored one; Test connection uses it.
  The name and kind are fixed.
- **Set as default** (or `locci env default NAME`) moves the original API
  routes and deploy jobs to that host.
- **Remove** forgets the host and its stored keys; nothing on the host is
  touched. The default can't be removed until another one is the default.

Only admins can add, edit, remove or change the default. Every signed-in user
can see and use every environment (per-environment permissions are in the
backlog).

## Deploy jobs

RabbitMQ deploy jobs may name an environment in `deployConfig.host`:

```json
{ "projectId": "p1", "deployConfig": { "id": "42", "action": "start", "target": "web", "host": "edge" } }
```

Without `host` the job uses the current default environment, as before. The
job's existing `environment` field is the sender's own label (such as
`production`) and is not used to pick a host.
