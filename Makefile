.PHONY: build run test clean db-up db-down db-migrate

build:
	go build -o bin/server cmd/server/main.go

run:
	go run cmd/server/main.go

test:
	go test -v ./...

clean:
	rm -rf bin/

db-up:
	docker-compose up -d sqlserver

db-down:
	docker-compose down

db-migrate:
	go run cmd/server/main.go -migrate
