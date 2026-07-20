# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e

FROM golang:1.26.5-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off

COPY core/go.mod core/go.sum ./core/
COPY proto/gen/go.mod proto/gen/go.sum ./proto/gen/
COPY server/go.mod server/go.sum ./server/
RUN --mount=type=cache,target=/go/pkg/mod \
    cd server && go mod download

COPY core/ ./core/
COPY proto/gen/ ./proto/gen/
COPY server/ ./server/

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd server && go build -trimpath \
      -ldflags="-s -w -X main.buildVersion=${VERSION}" \
      -o /out/kave-server .

FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="Kave" \
      org.opencontainers.image.description="Tenant-scoped AI admission, accounting, and provider gateway" \
      org.opencontainers.image.source="https://github.com/kave-io/kave" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.licenses="Apache-2.0"
COPY --from=build --chown=nonroot:nonroot /out/kave-server /usr/local/bin/kave-server
USER nonroot:nonroot
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/kave-server", "healthz"]
ENTRYPOINT ["/usr/local/bin/kave-server"]
