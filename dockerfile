FROM golang:1.24.0-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -v -o main .

FROM alpine:latest
WORKDIR /app

# Install wget for healthcheck
RUN apk add --no-cache wget

COPY --from=builder /app/main .
COPY --from=builder /app/templates/ /app/templates/
COPY --from=builder /app/static/ /app/static/
COPY --from=builder /app/uploads/ /app/uploads/
COPY --from=builder /app/.env /app/.env

# Create a non-root user
RUN adduser -D -g '' appuser
RUN chown -R appuser:appuser /app
USER appuser

ENV GIN_MODE=release
ENV SESSION_SECRET=your-session-secret-key

EXPOSE 8080
CMD ["./main"]