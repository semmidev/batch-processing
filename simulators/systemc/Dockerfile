FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy files
COPY main.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o systemc main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the built binary
COPY --from=builder /app/systemc .

# Expose port
EXPOSE 8081

# Command to run
CMD ["./systemc"]
