# Stage 2: Create a minimal image for running the application
FROM alpine:latest

# important for cleanly closing connection
STOPSIGNAL SIGINT

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY ./broadcaster .

# Command to run the application
CMD ["./broadcaster"]
