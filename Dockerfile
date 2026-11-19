# Build stage
FROM golang:1.26.11-alpine@sha256:ecb67f987aa1a57794c91d19ade7ae0209f2f0f5496aaef9e6ae85ddf92d02ab AS builder

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

FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

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