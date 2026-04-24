FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -o /bin/crawler ./cmd/crawler

FROM gcr.io/distroless/static-debian12 AS api
COPY --from=builder /bin/api /api
ENTRYPOINT ["/api"]

FROM gcr.io/distroless/static-debian12 AS crawler
COPY --from=builder /bin/crawler /crawler
ENTRYPOINT ["/crawler"]
