# locci tasks. Run `just` to see everything, grouped.
#
#   just setup            first-time setup
#   just dev              run the server from source
#   just cli ps           run any CLI command from source
#
# Each recipe has a one-line description (shown by `just`) and a short
# comment above every command it runs.

# load variables from .env into every recipe
set dotenv-load

# don't echo the comments inside recipes when they run
set ignore-comments

bin := "locci"
image := "locci/docker"

# list recipes
default:
    # print every recipe with its description, in file order
    @{{ just_executable() }} --list --unsorted

# ---------------------------------------------------------------- setup

# first-time setup: .env, Go modules, web dependencies
[group('setup')]
setup:
    # create .env from the sample unless it already exists
    test -f .env || cp .env.sample .env
    # fetch Go dependencies
    go mod download
    # install the web UI's packages (bun.lock pins the versions)
    cd web && bun install --frozen-lockfile

# check the tools this project needs
[group('setup')]
doctor:
    # Go toolchain
    @go version
    # Bun, for installing and building the web UI
    @bun --version
    # C compiler, needed by go-libsql (cgo)
    @gcc --version | head -1
    # Docker daemon the server will manage
    @docker info >/dev/null 2>&1 && echo "docker: reachable" || echo "docker: not reachable"

# ---------------------------------------------------------------- develop

# run the API server from source
[group('develop')]
dev:
    # compile and start `locci serve` (reads .env)
    go run ./cmd/locci serve

# run the web UI with hot reload (in a second terminal, next to `just dev`)
[group('develop')]
dev-web:
    # vite dev server; proxies /api to the server from `just dev`
    cd web && bun run dev

# format Go code
[group('develop')]
fmt:
    # rewrite Go files in the standard format
    gofmt -w ./cmd ./internal

# vet + tests
[group('develop')]
test:
    # static checks for common mistakes
    go vet ./...
    # run all Go tests
    go test ./...

# tidy go.mod / go.sum
[group('develop')]
tidy:
    # add missing and drop unused dependencies
    go mod tidy

# ---------------------------------------------------------------- build

# build the web UI into internal/ui/dist (embedded by go build)
[group('build')]
web:
    # SvelteKit build straight into internal/ui/dist
    cd web && bun run build
    # the build wipes dist; restore the tracked placeholder
    touch internal/ui/dist/.gitkeep

# build the binary (cgo is required by go-libsql)
[group('build')]
build:
    # compile ./cmd/locci into ./locci with cgo enabled
    CGO_ENABLED=1 go build -o {{ bin }} ./cmd/locci

# build the web UI, then the binary
[group('build')]
all: web build

# build and run the server binary
[group('build')]
run: build
    # start the freshly built binary
    ./{{ bin }} serve

# remove build output
[group('build')]
clean:
    # delete the binary
    rm -f {{ bin }}
    # delete the built UI but keep the placeholder file
    find internal/ui/dist -mindepth 1 ! -name .gitkeep -delete

# ---------------------------------------------------------------- cli

# run any CLI command from source, e.g. `just cli ps` or `just cli logs -f web`
[group('cli')]
cli *args:
    # pass every argument straight to the locci CLI
    go run ./cmd/locci {{ args }}

# create an admin user on this host (prompts for a password)
[group('cli')]
admin name:
    # writes the user directly into the local database (DATABASE_URL)
    go run ./cmd/locci user add {{ name }} --admin

# log in to a server, e.g. `just login admin` or `just login admin https://host`
[group('cli')]
login user url="http://localhost:8998":
    # prompts for the password and saves the token for later commands
    go run ./cmd/locci login --url {{ url }} -u {{ user }}

# list containers
[group('cli')]
ps:
    # same as `locci ps`
    go run ./cmd/locci ps

# follow a container's logs
[group('cli')]
logs container:
    # stream logs until Ctrl+C
    go run ./cmd/locci logs -f {{ container }}

# install the CLI to $GOBIN (~/go/bin) so `locci` works everywhere
[group('cli')]
install:
    # build and copy the binary into Go's bin directory
    CGO_ENABLED=1 go install ./cmd/locci

# ---------------------------------------------------------------- docker

# build the Docker image
[group('docker')]
image tag="latest":
    # multi-stage build: web UI, Go binary, slim runtime
    docker build -t {{ image }}:{{ tag }} .

# run the image with the host's Docker socket and a data volume
[group('docker')]
up tag="latest":
    # socket = manage host containers, volume = keep the database
    docker run -d --name {{ bin }} -p 8998:8998 \
      -v /var/run/docker.sock:/var/run/docker.sock \
      -v locci-data:/data \
      --env-file .env \
      -e DATABASE_URL=file:/data/locci.db \
      -e DOCKER_HOST=unix:///var/run/docker.sock \
      -e SERVER_PORT=8998 -e GIN_MODE=release \
      {{ image }}:{{ tag }}

# stop and remove the container
[group('docker')]
down:
    # force-remove stops it first; the data volume is kept
    docker rm -f {{ bin }}

# follow the container's logs
[group('docker')]
docker-logs:
    # stream logs until Ctrl+C
    docker logs -f {{ bin }}

# build the image from the current source and start it with compose.yaml
[group('docker')]
compose-up tag="latest":
    # --build keeps the container in step with the code (cached layers make it quick)
    LOCCI_IMAGE={{ image }}:{{ tag }} docker compose up -d --build
    @echo "open http://localhost:${LOCCI_PORT:-8998}"

# stop the compose service (the data volume is kept)
[group('docker')]
compose-down:
    # removes the container and network, not the volume
    docker compose down

# follow the compose service's logs
[group('docker')]
compose-logs:
    # stream logs until Ctrl+C
    docker compose logs -f

# ---------------------------------------------------------------- project

# branch, uncommitted changes and recent commits
[group('project')]
status:
    # current branch
    @git branch --show-current
    # changed files
    @git status --short
    # last five commits
    @git log --oneline -5

# start a new branch, e.g. `just branch feat/stacks`
[group('project')]
branch name:
    # create the branch and switch to it
    git switch -c {{ name }}

# run tests, then commit everything, e.g. `just commit "feat(api): add stacks"`
[group('project')]
commit message: test
    # stage every change (new, modified, deleted files)
    git add -A
    # commit with the given message
    git commit -m "{{ message }}"

# push the current branch
[group('project')]
push:
    # push and set upstream so plain `git push` works afterwards
    git push -u origin HEAD

# commits since the last tag
[group('project')]
changes:
    # from the latest tag (or the first commit if there are no tags) to HEAD
    @git log --oneline $(git describe --tags --abbrev=0 2>/dev/null || git rev-list --max-parents=0 HEAD)..HEAD

# tag a release and push the tag, e.g. `just release v0.1.0`
[group('project')]
release version: test
    # annotated tag on the current commit
    git tag -a {{ version }} -m "{{ version }}"
    # publish the tag
    git push origin {{ version }}

# rename the Go module, e.g. `just rename github.com/MikeTeddyOmondi/bandari`
[group('project')]
rename module:
    #!/usr/bin/env bash
    set -euo pipefail
    # current module path from go.mod
    old=$(go list -m)
    # update go.mod
    go mod edit -module "{{ module }}"
    # rewrite import paths in every .go file
    grep -rl "$old" --include='*.go' . | xargs perl -pi -e "s#\Q$old\E#{{ module }}#g"
    # make sure everything still compiles
    go build ./...
    echo "renamed $old -> {{ module }}"
