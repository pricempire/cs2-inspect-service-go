# Build stage
FROM golang:1.23-alpine AS builder

# Install required packages
RUN apk add --no-cache gcc musl-dev git

WORKDIR /app

# First, copy the entire repository including .git directory
COPY . .

# Initialize and update git submodules
RUN if [ -d ".git" ]; then \
        git submodule update --init --recursive; \
    else \
        echo "Warning: .git directory not found, cannot update submodules"; \
    fi

# Download dependencies
RUN go mod download

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -o cs2-inspect-service -ldflags "-X google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn"

# Final stage
FROM alpine:latest

WORKDIR /app

# Install required runtime packages
RUN apk add --no-cache ca-certificates tzdata

# Copy the binary from builder
COPY --from=builder /app/cs2-inspect-service .

# Create directory for session files
RUN mkdir -p sessions

# Create directory for logs
RUN mkdir -p logs

# Expose port
EXPOSE 3000

# Run the service
CMD ["./cs2-inspect-service"] 