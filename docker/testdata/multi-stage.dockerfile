# Build stage
FROM golang:1.22 AS builder
WORKDIR /src
COPY . .
RUN go build -o /app ./cmd/api

# Runtime stage
FROM alpine:3.19 AS runtime
COPY --from=builder /app /app
EXPOSE 8080
CMD ["/app"]
