.PHONY: tidy fmt test run start stop restart

tidy:
	go mod tidy

fmt:
	go fmt ./...

test:
	go test ./...

run:
	go run ./cmd/server

start:
	./scripts/start.sh

stop:
	./scripts/stop.sh

restart:
	./scripts/restart.sh
