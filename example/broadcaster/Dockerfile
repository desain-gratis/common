# Stage 2: Create a minimal image for running the application
FROM alpine:latest

# important for cleanly closing connection
STOPSIGNAL SIGINT

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY ./broadcaster .

# Expose the port your application listens on (if applicable)
EXPOSE 9090

# Command to run the application
CMD ["./broadcaster"]
