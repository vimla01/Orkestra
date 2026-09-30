.PHONY: build run test lint docker-build clean demo-up demo demo-server demo-down

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

# Local multi-cluster demo on kind (requires docker and kind).
demo-up:
	./hack/demo/up.sh

demo:
	./hack/demo/run.sh

demo-server: build
	./bin/orkestra serve --config hack/demo/config.yaml

demo-down:
	./hack/demo/down.sh
