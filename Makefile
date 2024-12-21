IMAGE = cost-manager
BIN_DIR = ./bin

test:
	go test $(shell go list ./... | grep -v github.com/hsbc/cost-manager/e2e) -race

build:
	go build -o $(BIN_DIR)/cost-manager

.PHONY: e2e
e2e:
	go test ./e2e --test.image=$(IMAGE) --parallel=4 --shuffle=on -race -v

image:
	docker build -t $(IMAGE) .

generate: controller-gen
	$(BIN_DIR)/controller-gen object \
		paths=./pkg/api/...

# https://book.kubebuilder.io/reference/controller-gen
controller-gen:
	go build -o $(BIN_DIR)/controller-gen sigs.k8s.io/controller-tools/cmd/controller-gen
