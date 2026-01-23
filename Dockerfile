# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./switchAILocal ./cmd/server/

# Runtime stage

FROM alpine:3.23.2

RUN apk add --no-cache tzdata ca-certificates nodejs npm

# Install Gemini CLI in the container
RUN npm install -g @google/gemini-cli

# Create a non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/switchAILocal /app/switchAILocal
COPY config.example.yaml /app/config.example.yaml
COPY static /app/static

# Ensure the non-root user owns the application files
RUN chown -R appuser:appgroup /app

EXPOSE 18080

# Default to UTC, can be overridden via environment variable
ENV TZ=UTC

USER appuser

CMD ["./switchAILocal"]