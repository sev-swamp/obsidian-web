# --- Stage 1: frontend ---------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /src/apps/web
COPY apps/web/package.json apps/web/package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm npm install
COPY apps/web/ ./
RUN npm run build

# --- Stage 2: backend ----------------------------------------------------
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# Embed the freshly built frontend.
COPY --from=web /src/apps/web/dist ./apps/web/dist
# Cache mounts keep module and build caches between builds — rebuilds
# take ~1 min instead of ~10 on a small VPS.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/obsidianweb ./apps/server \
    && CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/obsidianweb-cli ./apps/cli

# --- Stage 3: runtime ----------------------------------------------------
FROM alpine:3.20
RUN adduser -D -u 1000 obsidianweb
COPY --from=build /out/obsidianweb /usr/local/bin/obsidianweb
COPY --from=build /out/obsidianweb-cli /usr/local/bin/obsidianweb-cli
USER obsidianweb
EXPOSE 8787
VOLUME ["/vault"]
ENV OBSIDIANWEB_VAULT=/vault
ENTRYPOINT ["obsidianweb"]
CMD ["-config", "/config/config.yaml"]
