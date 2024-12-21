FROM golang:1.23.4 as build

WORKDIR /go/src/cost-manager

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build static cost-manager binary
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /go/bin/cost-manager

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /go/bin/cost-manager /
ENTRYPOINT ["/cost-manager"]
