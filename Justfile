# locci: `just` lists recipes

default:
    just --list

# install web dependencies (first time)
web-install:
    cd web && npm install

# build the web UI into internal/ui/dist (embedded by go build)
web:
    cd web && npm run build
    touch internal/ui/dist/.gitkeep

# run the UI with hot reload (proxies /api to `just run`)
web-dev:
    cd web && npm run dev

# build the single binary (cgo is required by go-libsql)
build:
    CGO_ENABLED=1 go build -o locci ./cmd/locci

# build UI + binary
all: web build

run: build
    ./locci serve

test:
    go vet ./...
    go test ./...

tidy:
    go mod tidy

build-image:
    docker build -t ranckosolutionsinc/locci:dev .

run-container:
    docker run -d --name locci -p 8998:8998 \
      -v /var/run/docker.sock:/var/run/docker.sock \
      -v locci-data:/data \
      --env-file .env \
      ranckosolutionsinc/locci:dev

log:
    docker logs -f locci

rm-container:
    docker rm -f locci
