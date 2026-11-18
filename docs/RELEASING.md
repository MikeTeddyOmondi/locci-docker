# Releasing

Two GitHub Actions workflows live in `.github/workflows`:

- **`ci.yml`**, on every push to `main` and every pull request:
  - Go: `gofmt`, `go vet`, `go test` and `scripts/govulncheck.sh`
  - web UI: `bun run check` and `bun run build`
  - the Docker image built for `linux/amd64` and `linux/arm64` (not pushed)
- **`release.yml`**, on a pushed tag `v*`:
  - binaries for Linux, macOS and Windows on amd64 and arm64
  - the multi-arch image `docker.io/locci/docker:<version>`, plus `:<major>.<minor>`
    and `:latest` (not for pre-releases such as `v1.0.0-rc.1`)
  - a GitHub release with the archives, `SHA256SUMS`, the signatures and the
    notes from `CHANGELOG.md`

## Secrets

Set these in the repository's settings (Secrets and variables, Actions):

| Secret | What |
|---|---|
| `DOCKERHUB_USERNAME` | Docker Hub user that can push `locci/docker` |
| `DOCKERHUB_TOKEN` | a Docker Hub access token for it (Read & Write) |
| `RELEASE_GPG_KEY` | the ASCII-armoured private key that signs the binaries (`gpg --armor --export-secret-keys KEYID`) |
| `RELEASE_GPG_PASSPHRASE` | its passphrase |

The image signature and the provenance attestations need no secret: they use
the workflow's own GitHub identity through Sigstore (keyless).

Publish the public half of the GPG key where users can find it, for example
as `KEYS` in the repository or on a keyserver:
`gpg --armor --export KEYID > KEYS`.

## Cutting a release

1. In `CHANGELOG.md`, rename `## [Unreleased]` to `## [0.2.0] - 2026-11-18`
   and start a new empty `## [Unreleased]` above it. The release notes are
   that section.
2. Commit, then tag and push:
   ```sh
   git tag v0.2.0
   git push origin v0.2.0
   ```
3. Watch the run under Actions. A tag with a dash (`v0.2.0-rc.1`) is marked as
   a pre-release and doesn't move `:latest`.

`locci --version` in the binaries and the image prints the tag.

## What is signed, and how to verify it

**Binaries.** Each archive has a GPG signature (`.asc`), `SHA256SUMS` lists
them all and is signed too, and GitHub holds a build provenance attestation
for every archive.

```sh
gpg --import KEYS
gpg --verify SHA256SUMS.asc SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
gh attestation verify locci-v0.2.0-linux-amd64.tar.gz --repo MikeTeddyOmondi/locci-docker
```

**Image.** It is signed with cosign (keyless) by digest, and carries a
provenance attestation and an SBOM.

```sh
cosign verify docker.io/locci/docker:0.2.0 \
  --certificate-identity-regexp '^https://github.com/MikeTeddyOmondi/locci-docker/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify oci://docker.io/locci/docker:0.2.0 --repo MikeTeddyOmondi/locci-docker
```

## Notes

- The binary is pure Go (`CGO_ENABLED=0`), so one Linux runner cross-compiles
  every target, and the Dockerfile cross-compiles per platform instead of
  emulating one.
- The macOS binaries aren't notarized; macOS quarantines downloaded
  binaries, so run `xattr -d com.apple.quarantine locci` once, or install
  from source.
- `scripts/govulncheck.sh` fails CI on any reachable vulnerability not listed
  in `.govulncheck-ignore`, where each entry says why it doesn't apply.
