FROM golang:1.21 as build

WORKDIR /go/src/cost-manager
# Configure Go build cache: https://docs.docker.com/build/ci/github-actions/cache/#cache-mounts
RUN go env -w GOMODCACHE=/root/.cache/go-build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download

COPY . .

# Build static cost-manager binary
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -ldflags="-s -w" -o /go/bin/cost-manager

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /go/bin/cost-manager /
ENTRYPOINT ["/cost-manager"]
