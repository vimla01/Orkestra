.PHONY: build run test lint docker-build clean

build:
	go build -o bin/orkestra ./cmd/orkestra

run:
	go run ./cmd/orkestra --config config.yaml

test:
	go test ./... -v -race

lint:
	go vet ./...

docker-build:
	docker build -t orkestra:dev .

clean:
	rm -rf bin/
