# syntax=docker/dockerfile:1

# --- UI ---
FROM node:20-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --omit=optional
COPY web/ ./
RUN npm run build

# --- Fleet UI ---
FROM node:20-alpine AS web-fleet
WORKDIR /src/web-fleet
COPY web-fleet/package.json ./
RUN npm install
COPY web-fleet/ ./
RUN npm run build

# --- API ---
FROM golang:1.27-alpine AS api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
COPY --from=web-fleet /src/web-fleet/dist ./web-fleet/dist
ARG KATANA_VERSION=0.5.0
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/goxray/goxray/internal/version.Version=${KATANA_VERSION}" \
    -o /out/katana ./cmd/katana
RUN CGO_ENABLED=0 GOOS=linux go build \
    -o /out/fleet ./cmd/fleet

# --- Fleet runtime (docker build --target fleet) ---
FROM gcr.io/distroless/static-debian12:nonroot AS fleet
WORKDIR /
COPY --from=api /out/fleet /fleet
COPY --from=api /src/web-fleet/dist /web-fleet/dist
ENV FLEET_ADDR=0.0.0.0:8090 \
    FLEET_DB_PATH=/data/fleet.db \
    FLEET_WEB_DIST=/web-fleet/dist \
    PROMETHEUS_URL=http://prometheus:9090
USER nonroot:nonroot
EXPOSE 8090
ENTRYPOINT ["/fleet"]

# --- Katana runtime (default) ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=api /out/katana /katana
COPY --from=api /src/web/dist /web/dist
ARG KATANA_VERSION=0.5.0
ENV KATANA_ADDR=0.0.0.0:8443 \
    KATANA_DB_PATH=/data/katana.db \
    KATANA_ADMISSION_DRY_RUN=true \
    KATANA_VERSION=${KATANA_VERSION}
USER nonroot:nonroot
EXPOSE 8443
ENTRYPOINT ["/katana"]
