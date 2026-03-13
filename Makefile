.PHONY: tidy fmt vet build run run-all test check clean

CMDS := backplane worker log-siem-transmitter log-dev-projector

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
# Output from all four interleaves; Ctrl+C kills the whole group.
.ONESHELL:
run-all: build
	@trap 'kill 0' SIGINT SIGTERM
	for c in $(CMDS); do \
		echo "→ starting $$c"; \
		./bin/$$c & \
	done
	wait

test:
	go test ./...

check: fmt vet test

clean:
	rm -rf bin tmp
