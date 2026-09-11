# Reference SSO plugin — standalone image, no platform code inside.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/sso-oidc .

FROM alpine:3.20
RUN adduser -D -u 1000 plugin && apk add --no-cache ca-certificates
COPY --from=build /out/sso-oidc /usr/local/bin/sso-oidc
USER plugin
EXPOSE 8080
ENTRYPOINT ["sso-oidc"]
