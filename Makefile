test:
	go test ./...

build: test
	go build -o ./bin/cost-manager

run: build
	./bin/cost-manager

image:
	docker build -t cost-manager .
