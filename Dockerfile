# First stage of multi-stage build: build the Go binary
FROM golang:1.19-alpine AS builder

# Create directory for build context
WORKDIR /build

# Get the required files
COPY go.mod .
COPY go.sum .
COPY *.go ./
COPY VERSION .

# Download all dependencies
RUN go mod download

# Build the Go app
RUN CGO_ENABLED=0 go build -ldflags="-X main.AppVersion=$(cat VERSION) -s -w" -trimpath -o sessionprobe .

# Second stage of multi-stage build: run the Go binary
FROM alpine:latest

# Create required directories and adjust working directory
RUN mkdir /app /app/files
WORKDIR /app/files

# Running as a non-root user
RUN adduser -D local
USER local

# Copy binary from first stage
COPY --from=builder /build/sessionprobe /app

# This command runs the app
ENTRYPOINT ["/app/sessionprobe"]
