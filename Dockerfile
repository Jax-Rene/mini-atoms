FROM golang:1.24-bookworm AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends nodejs npm ca-certificates && rm -rf /var/lib/apt/lists/*
RUN corepack enable

COPY package.json pnpm-lock.yaml tailwind.config.js ./
RUN pnpm install --frozen-lockfile

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY web ./web

RUN pnpm run build:css
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/mini-atoms ./cmd/server

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates sqlite3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app

COPY --from=builder /out/mini-atoms /app/mini-atoms

ENV APP_ENV=production
ENV APP_ADDR=:8080
ENV DATABASE_PATH=/data/mini-atoms-gorm.db

EXPOSE 8080

CMD ["/app/mini-atoms"]
