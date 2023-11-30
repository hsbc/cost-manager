test:
	go test ./...

build: test
	go build -o ./bin/cost-manager

run: build
	./bin/cost-manager

image:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags netgo -ldflags="-s -w" -o ./bin/cost-manager.linux
	docker build -t cost-manager .
