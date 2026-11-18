# The web UI and the Go binary build on the machine running the build
# (BUILDPLATFORM) and Go cross-compiles for each target, so a multi-arch image
# needs no emulation. The binary is pure Go and static (CGO_ENABLED=0).

# --- web UI ---
FROM --platform=$BUILDPLATFORM oven/bun:1.4.3 AS web
WORKDIR /src/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN mkdir -p ../internal/ui/dist && bun run build

# --- Go binary ---
FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/ui/dist ./internal/ui/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags="-s -w -X github.com/MikeTeddyOmondi/locci-docker/internal/cli.Version=$VERSION" \
      -o /out/locci ./cmd/locci

# --- runtime ---
# No RUN here: this stage is for the target platform, and anything run in it
# would need emulation. The CA bundle comes from the build stage.
FROM debian:bookworm-slim
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/locci /usr/local/bin/locci
ENV DATABASE_URL=file:/data/locci.db GIN_MODE=release
# Only admins may change or open a console in the server and agent containers.
LABEL io.locci.protected="true"
VOLUME /data
EXPOSE 8998
ENTRYPOINT ["locci"]
CMD ["serve"]
