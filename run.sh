#!/bin/bash

# Set the environment variable to handle protobuf registration conflicts
export GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn

# Check if port 3000 is already in use
echo "Checking if port 3000 is already in use..."
if lsof -i :3000 > /dev/null 2>&1; then
  echo "WARNING: Port 3000 is already in use by another process!"
  echo "Process using port 3000:"
  lsof -i :3000
  echo "You may need to kill this process before continuing."
  read -p "Do you want to continue anyway? (y/n) " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 1
  fi
fi

# Run the Go application with verbose output
echo "Starting CS2 Inspect Service on 127.0.0.1:3000..."
echo "Using GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn"

# Run with verbose output to see any errors
go run . 2>&1 | tee inspect-service.log

# Keep the script running until manually stopped
echo "Press Ctrl+C to stop the service" 