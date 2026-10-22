# syntax=docker/dockerfile:1

# ---- frontend ----
FROM node:24-slim AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---- backend ----
FROM golang:1.27-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/homeops ./cmd/homeops

# ---- runtime: ~15 MB, no shell, runs as non-root ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=api /out/homeops /app/homeops
COPY --from=web /web/dist /app/web
ENV HOMEOPS_STATIC_DIR=/app/web \
    HOMEOPS_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD ["/app/homeops", "healthcheck"]
ENTRYPOINT ["/app/homeops"]
