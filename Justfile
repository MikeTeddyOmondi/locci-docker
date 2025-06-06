default:
    just --list

tidy:
    go mod tidy    

build:
    go build -o docker-wrapper ./cmd/main.go

run:
    just build
    ./docker-wrapper

run-bin:
    go run ./cmd/main.go

build-image:
    docker build -t ranckosolutionsinc/bundler-service-go:v1.0.0 .

run-container:
    docker run -dp 8998:8998 \
    --name docker-wrapper \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -e _EXPERIMENTAL_DAGGER_RUNNER_HOST=docker-container://dagger-engine-v0.14.0 \
    ranckosolutionsinc/docker-wrapper:v1.0.0

log:
    docker logs -f docker-wrapper

stop-container:
    docker stop docker-wrapper

rm-container:
    docker stop docker-wrapper
    docker rm docker-wrapper

compose:
    docker-compose up -d

compose-down:
    docker-compose down

compose-build:
    docker-compose up -d --build
