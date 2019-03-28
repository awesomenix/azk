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
FROM gcr.io/distroless/base:latest
WORKDIR /
COPY --from=builder /go/src/github.com/awesomenix/azk/manager .
ENTRYPOINT ["/manager"]
