# JustixAuto: one image with the API, the migration and bootstrap commands and
# the four built web apps (served by the API from WEB_DIR).
# Base images are pinned by digest (infra/AGENTS.md: immutable references).

FROM node:24.21.0-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS web
WORKDIR /src
COPY package.json package-lock.json ./
COPY web ./web
RUN npm ci --ignore-scripts --no-audit --no-fund && npm run build:apps

FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
WORKDIR /app
COPY --from=go /out/ /app/
COPY --from=web /src/web/apps/realization/dist /app/web/apps/realization/dist
COPY --from=web /src/web/apps/financing/dist /app/web/apps/financing/dist
COPY --from=web /src/web/apps/insurance/dist /app/web/apps/insurance/dist
COPY --from=web /src/web/apps/admin/dist /app/web/apps/admin/dist
ENV HTTP_ADDR=:8080 WEB_DIR=/app/web/apps
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]
