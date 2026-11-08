.PHONY: run build test fmt tidy compose-up compose-down

run:
	go run ./cmd/server

build:
	go build -o bin/sonichub ./cmd/server

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

tidy:
	GOPROXY=https://goproxy.cn,direct go mod tidy

compose-up:
	docker compose -f deploy/docker-compose.yml up --build

compose-down:
	docker compose -f deploy/docker-compose.yml down
