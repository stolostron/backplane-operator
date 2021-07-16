# Build the backplane-operator binary
FROM golang:1.16 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/

COPY bin/ bin/
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o backplane-operator main.go

# Use ubi minimal base image to package the backplane-operator binary
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
WORKDIR /

COPY --from=builder workspace/bin/ bin/
COPY --from=builder workspace/backplane-operator .
USER 65532:65532

ENTRYPOINT ["/backplane-operator"]
