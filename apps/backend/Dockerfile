# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build-base
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .

FROM build-base AS build-api
ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -mod=readonly -trimpath -ldflags="-s -w -buildid=" -o /out/api ./cmd/api

FROM build-base AS build-migrate
ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -mod=readonly -trimpath -ldflags="-s -w -buildid=" -o /out/migrate ./cmd/migrate

FROM build-base AS build-bootstrap
ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -mod=readonly -trimpath -ldflags="-s -w -buildid=" -o /out/bootstrap ./cmd/bootstrap

# Multi-platform index digest resolved from the official registry on 2026-07-29.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6 AS runtime
WORKDIR /app
ENV TZ=Asia/Jakarta
USER 65532:65532

FROM runtime AS migrate
COPY --from=build-migrate --chown=65532:65532 --chmod=0555 /out/migrate /app/migrate
ENTRYPOINT ["/app/migrate"]

FROM runtime AS bootstrap
COPY --from=build-bootstrap --chown=65532:65532 --chmod=0555 /out/bootstrap /app/bootstrap
ENTRYPOINT ["/app/bootstrap"]
CMD ["-manifest", "/run/secrets/bootstrap-users.json"]

# Keep the API last so a plain `docker build` still produces the serving image.
FROM runtime AS api
COPY --from=build-api --chown=65532:65532 --chmod=0555 /out/api /app/api
EXPOSE 8080
ENTRYPOINT ["/app/api"]
