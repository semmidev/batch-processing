# Build stage — runs from the REPO ROOT context (docker-compose sets context: .)
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy the systema source
COPY simulators/systema/main.go ./simulators/systema/

# Build
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o systema \
    ./simulators/systema/

# Final stage — minimal image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

COPY --from=builder /app/systema .

EXPOSE 8082

CMD ["./systema"]
