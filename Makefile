.PHONY: build run test lint docker-build clean dashboard dashboard-dev demo-up demo demo-server demo-down

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
	rm -rf bin/ dashboard/dist

# Web dashboard (requires Node.js). The control plane serves dashboard/dist.
dashboard:
	cd dashboard && npm ci && npm run build

# Dashboard dev server with hot reload on :5173, proxying /api to :8080.
dashboard-dev:
	cd dashboard && npm install && npm run dev

# Local multi-cluster demo on kind (requires docker and kind).
demo-up:
	./hack/demo/up.sh

demo:
	./hack/demo/run.sh

demo-server: build
	./bin/orkestra serve --config hack/demo/config.yaml

demo-down:
	./hack/demo/down.sh
