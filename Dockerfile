# Build stage
FROM golang:1.26.10-alpine@sha256:87c65a688975c0b1d4585d204874c45ca78354670e3a5f11368c5a492303d696 AS builder

WORKDIR /app

RUN apk add --no-cache gcc musl-dev linux-headers

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./switchAILocal ./cmd/server/

# Runtime stage

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache tzdata ca-certificates nodejs npm

# Install Gemini CLI in the container
RUN npm install -g @google/gemini-cli

# Create a non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/switchAILocal /app/switchAILocal
COPY config.example.yaml /app/config.example.yaml
COPY static /app/static
COPY plugins /app/plugins

# Ensure the non-root user owns the application files
RUN chown -R appuser:appgroup /app

EXPOSE 18080

# Default to UTC, can be overridden via environment variable
ENV TZ=UTC

USER appuser

CMD ["./switchAILocal"]