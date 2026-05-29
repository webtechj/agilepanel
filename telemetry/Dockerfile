# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o telemetryd .

# Run stage
FROM scratch
WORKDIR /
COPY --from=builder /app/telemetryd /telemetryd
EXPOSE 8080
VOLUME ["/data"]
ENV DATA_DIR=/data/servers
ENTRYPOINT ["/telemetryd"]
