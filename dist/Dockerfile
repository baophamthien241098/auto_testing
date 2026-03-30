FROM alpine:latest

WORKDIR /app

# Copy the pre-compiled binary
COPY main_linux main
RUN apk add --no-cache tzdata
RUN chmod +x main

# Expose port
EXPOSE 8080

CMD ["./main"]
