FROM golang:1.21 as build

WORKDIR /go/src/cost-manager

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Ensure Go files are formatted correctly
RUN test -z "$(gofmt -l .)"

# Run unit tests
RUN go test -race ./...

# Build static cost-manager binary
RUN CGO_ENABLED=0 go build -tags netgo -ldflags="-s -w" -o /go/bin/cost-manager

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /go/bin/cost-manager /
ENTRYPOINT ["/cost-manager"]
