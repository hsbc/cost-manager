BIN_DIR = ./bin

test:
	go test -race ./...

build:
	go build -o $(BIN_DIR)/cost-manager

run: build
	$(BIN_DIR)/cost-manager

image:
	docker build -t cost-manager .

generate: deepcopy-gen
	$(BIN_DIR)/deepcopy-gen \
		--go-header-file ./hack/boilerplate.go.txt \
		--input-dirs ./pkg/api/v1alpha1 \
		--output-file-base zz_generated.deepcopy

verify: deepcopy-gen
	$(BIN_DIR)/deepcopy-gen \
		--go-header-file ./hack/boilerplate.go.txt \
		--input-dirs ./pkg/api/v1alpha1 \
		--output-file-base zz_generated.deepcopy \
		--verify-only

deepcopy-gen:
	go build -o $(BIN_DIR)/deepcopy-gen k8s.io/code-generator/cmd/deepcopy-gen
