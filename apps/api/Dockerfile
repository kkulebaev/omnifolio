FROM golang:1.23-alpine AS build
WORKDIR /app
RUN apk add --no-cache git

COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download

COPY apps/api ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/api /usr/local/bin/api
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
