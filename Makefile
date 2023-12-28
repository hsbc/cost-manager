test:
	go test -race ./...

build:
	go build -o ./bin/cost-manager

run: build
	./bin/cost-manager

image:
	docker build -t cost-manager .
