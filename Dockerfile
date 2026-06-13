FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY database-sqlite/ /build/database-sqlite/
WORKDIR /build/database-sqlite
RUN go mod download && CGO_ENABLED=0 go build -o /database-sqlite ./cmd/module
FROM alpine:3.21
RUN adduser -D -h /data db
USER db
WORKDIR /app
COPY --from=builder /database-sqlite .
ENTRYPOINT ["./database-sqlite"]
