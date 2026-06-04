.PHONY: tidy fmt vet build run run-all test check clean migrate-init migrate-up migrate-down migrate-status

CMDS := backplane worker ingester pdp inference-gateway log-siem-transmitter log-dev-projector migrate

tidy:
	go mod tidy

fmt:
	gofmt -s -w .

vet:
	go vet ./...

build:
	mkdir -p bin
	@for c in $(CMDS); do \
		echo "building $$c"; \
		go build -o bin/$$c ./cmd/$$c || exit 1; \
	done

run:
	go run ./cmd/backplane

# Run every binary in the foreground, multiplexed into this terminal.
# Output from all binaries interleaves; Ctrl+C kills the whole group.
# migrate is a one-shot tool, not a long-running process — skipped here.
.ONESHELL:
run-all: build
	@trap 'kill 0' SIGINT SIGTERM
	export AURELION_INGESTER_INSTANCE_ID=$${AURELION_INGESTER_INSTANCE_ID:-ingester-dev}
	export AURELION_PDP_INSTANCE_ID=$${AURELION_PDP_INSTANCE_ID:-pdp-dev}
	for c in backplane worker ingester pdp inference-gateway log-siem-transmitter log-dev-projector; do \
		echo "→ starting $$c"; \
		./bin/$$c & \
	done
	wait

test:
	go test ./...

check: fmt vet test

# bun migrations — secret store comes from the same env vars as backplane.
migrate-init:
	go run ./cmd/migrate init

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

clean:
	rm -rf bin tmp
