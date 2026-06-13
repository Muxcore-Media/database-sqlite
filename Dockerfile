FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY database-sqlite/ /build/database-sqlite/
WORKDIR /build/database-sqlite
RUN go mod download
RUN CGO_ENABLED=0 go build -o /database-sqlite ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /database-sqlite /
ENTRYPOINT ["/database-sqlite"]
