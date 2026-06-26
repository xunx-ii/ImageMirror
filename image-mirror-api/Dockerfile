FROM golang:1.26-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/image-mirror-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/image-mirror-worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/image-mirror-seed ./cmd/seed

FROM alpine:3.21

WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/image-mirror-api /usr/local/bin/image-mirror-api
COPY --from=builder /out/image-mirror-worker /usr/local/bin/image-mirror-worker
COPY --from=builder /out/image-mirror-seed /usr/local/bin/image-mirror-seed
COPY db/migrations ./db/migrations
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN sed -i 's/\r$//' /usr/local/bin/docker-entrypoint.sh \
  && chmod +x /usr/local/bin/docker-entrypoint.sh \
  && mkdir -p /data/images

EXPOSE 8080
ENTRYPOINT ["docker-entrypoint.sh"]
