# Build stage
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy the go.mod and go.sum files to cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project directory, including subdirectories
COPY . .

# Build the Go application with verbose output for the target architecture
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -v -o main .

# Run stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/main .
COPY --from=builder /app/templates/ /app/templates/ 
EXPOSE 8080
CMD ["./main"]