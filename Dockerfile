# Use the official Golang image to create a build artifact.
# This is known as a multi-stage build.
FROM golang:1.26-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app
# CGO_ENABLED=0 is important for a static build
# -ldflags="-w -s" strips debugging information, reducing binary size
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-w -s" -o bbscope main.go

# Start a new stage from scratch for a smaller, more secure image
FROM alpine:3.24

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create non-root user for security
RUN adduser -D -g '' -u 1000 bbscope

# Set the Current Working Directory inside the container
WORKDIR /home/bbscope

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/bbscope .

# Change ownership to non-root user
RUN chown -R bbscope:bbscope /home/bbscope

# Switch to non-root user
USER bbscope

# Command to run the executable
ENTRYPOINT ["./bbscope"]
