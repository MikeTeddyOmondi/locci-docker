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
| `DOCKER_USERNAME` | Docker Hub user that can push `locci/docker` |
| `DOCKER_TOKEN` | a Docker Hub access token for it (Read & Write) |
| `RELEASE_GPG_KEY` | the ASCII-armoured private key that signs the binaries (`gpg --armor --export-secret-keys KEYID`) |
| `RELEASE_GPG_PASSPHRASE` | its passphrase |

The image signature and the provenance attestations need no secret: they use
the workflow's own GitHub identity through Sigstore (keyless).

The public half of the GPG key is committed as `docs/release-key.asc`; the
workflow attaches it to each release and checks every signature against it,
so a release fails if the secret key and the published key don't match.
After changing the key, export it again:
`gpg --armor --export KEYID > docs/release-key.asc`.

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

The signing key is "Locci Release Signing" `<admin@locci.cloud>`, with this
fingerprint:

```
2B9D 1A57 F9B7 7E2E EDFF  0F05 B5E0 0DB4 AAE7 15C3
```

Its public half is [`docs/release-key.asc`](release-key.asc) in this
repository, and every release carries the same file as `release-key.asc`.
Import it once and check the fingerprint matches the one above:

```sh
gpg --import release-key.asc
gpg --fingerprint B5E00DB4AAE715C3
```

Then verify a download:

```sh
gpg --verify SHA256SUMS.asc SHA256SUMS          # the checksums are ours
sha256sum -c SHA256SUMS --ignore-missing        # and match your download
# or one archive directly:
gpg --verify locci-v0.2.0-linux-amd64.tar.gz.asc locci-v0.2.0-linux-amd64.tar.gz
gh attestation verify locci-v0.2.0-linux-amd64.tar.gz --repo MikeTeddyOmondi/locci-docker
```

gpg reports "Good signature" and, until you certify the key yourself, a
warning that it isn't certified with a trusted signature. That warning is
normal; the fingerprint check above is what establishes trust.

The key expires on 2028-08-18. Before then, extend it
(`gpg --quick-set-expire B5E00DB4AAE715C3 2y`), export the public key again
to `docs/release-key.asc`, and update the `RELEASE_GPG_KEY` secret. A new key
means a new fingerprint here too.

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
