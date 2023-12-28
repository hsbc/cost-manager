test:
	go test -race ./...

build:
	CGO_ENABLED=0 go build -tags netgo -ldflags="-s -w" -o ./bin/cost-manager

run: build
	./bin/cost-manager

image: build
	docker build -t cost-manager .
