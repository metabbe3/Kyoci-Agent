# Build stage
FROM golang:1.23-alpine AS builder

LABEL name="Kyoci Agent"
LABEL description="Production-grade autonomous AI agent engine"

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /ai-agent .

# Runtime stage
FROM alpine:3.20

LABEL name="Kyoci Agent"
LABEL description="Production-grade autonomous AI agent engine"

RUN apk --no-cache add ca-certificates
WORKDIR /app

COPY --from=builder /ai-agent /usr/local/bin/ai-agent
COPY config/config.yaml /app/config/config.yaml

EXPOSE 8080

ENTRYPOINT ["ai-agent"]
CMD ["--serve"]
