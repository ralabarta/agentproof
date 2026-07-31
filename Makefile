.PHONY: build test lint verify demo clean

build:
	go build -o bin/agentproof ./cmd/agentproof

test:
	go test ./...

lint:
	go vet ./...

verify: build
	./bin/agentproof verify

demo: build
	./bin/agentproof init
	./bin/agentproof record --objective "AgentProof self-check" --agent codex -- sh -c "true"
	./bin/agentproof verify --test-result testdata/go-test.jsonl

clean:
	rm -rf bin dist
