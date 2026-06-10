# Kyoci Agent - Production-grade autonomous AI agent engine
.PHONY: build run serve test clean docker proto grpc

# Build the binary
build:
	go build -o bin/ai-agent .

# Run interactively
run: build
	./bin/ai-agent

# Start API server
serve: build
	./bin/ai-agent --serve

# Run with specific provider
run-ollama: build
	./bin/ai-agent --provider ollama

run-openai: build
	./bin/ai-agent --provider openai

# Single prompt
ask: build
	./bin/ai-agent --prompt "$(MSG)"

# Generate protobuf code
proto:
	cd proto && PATH=$$PATH:~/go/bin protoc --go_out=. --go-grpc_out=. agent.proto

# Build and start gRPC server
grpc: build proto
	./bin/ai-agent --grpc

# Tests
test:
	go test ./...

# Clean
clean:
	rm -rf bin/

# Docker build
docker:
	docker build -t ai-agent .
	docker run -p 8080:8080 \
		-e OPENAI_API_KEY=$(OPENAI_API_KEY) \
		-e ANTHROPIC_API_KEY=$(ANTHROPIC_API_KEY) \
		ai-agent

# Tidy dependencies
tidy:
	go mod tidy