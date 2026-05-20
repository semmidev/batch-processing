FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy files
COPY main.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o systema main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the built binary
COPY --from=builder /app/systema .

# Expose port
EXPOSE 8082

# Command to run
CMD ["./systema"]
