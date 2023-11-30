FROM scratch

COPY ./bin/cost-manager.linux /cost-manager

ENTRYPOINT ["/cost-manager"]
