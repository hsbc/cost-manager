FROM gcr.io/distroless/static-debian12:nonroot

COPY ./bin/cost-manager /
ENTRYPOINT ["/cost-manager"]
