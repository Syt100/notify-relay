# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go test ./...
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -buildvcs=false -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/bridge ./cmd/notify-relay
RUN mkdir -p /out/data && chown 1000:1001 /out/data

FROM scratch
LABEL org.opencontainers.image.title="notify-relay" \
      org.opencontainers.image.description="A lightweight ntfy to WxPusher bridge" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/bridge /bridge
COPY --from=build --chown=1000:1001 /out/data /data
COPY LICENSE THIRD_PARTY_NOTICES.md /licenses/
COPY third_party/ /licenses/third_party/
USER 1000:1001
ENV GOMEMLIMIT=32MiB GOGC=50 GOMAXPROCS=1
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/bridge", "healthcheck"]
ENTRYPOINT ["/bridge"]
