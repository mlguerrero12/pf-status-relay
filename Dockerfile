FROM golang:1.21 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-w' -o pf-status-relay

FROM quay.io/centos/centos:stream9
COPY --from=builder /src/pf-status-relay /usr/bin/pf-status-relay
CMD ["/usr/bin/pf-status-relay"]
