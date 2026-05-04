FROM golang:1.26-alpine AS build
WORKDIR /app
RUN apk add --no-cache git

COPY apps/cron/go.mod apps/cron/go.sum* ./
RUN go mod download

COPY apps/cron ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/cron ./cmd/cron

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
COPY --from=build /out/cron /usr/local/bin/cron
ENTRYPOINT ["/usr/local/bin/cron"]
