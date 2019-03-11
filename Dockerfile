# Build the manager binary
FROM golang:latest as builder

# Copy in the go src
WORKDIR /go/src/github.com/awesomenix/azk
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager github.com/awesomenix/azk/cmd/manager

# Copy the controller-manager into a thin image
FROM ubuntu:latest
WORKDIR /
ENV TERM=xterm
RUN sed -i -e 's/^deb-src/#deb-src/' /etc/apt/sources.list && \
    export DEBIAN_FRONTEND=noninteractive && \
    apt-get update && \
    apt-get upgrade -y --no-install-recommends && \
    apt-get install -y --no-install-recommends \
    bash ca-certificates curl gnupg2 jq wget && \
    rm -rf /var/cache/apt
COPY --from=builder /go/src/github.com/awesomenix/azk/manager .
ENTRYPOINT ["/manager"]
