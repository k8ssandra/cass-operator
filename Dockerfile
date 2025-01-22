# Build the manager binary
FROM golang:1.23 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY apis/ apis/
COPY pkg/ pkg/
COPY internal/ internal/

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Build the UBI image
FROM redhat/ubi9-micro:latest

ARG VERSION

LABEL maintainer="DataStax, Inc <info@datastax.com>"
LABEL name="cass-operator"
LABEL vendor="DataStax, Inc"
LABEL release="${VERSION}"
LABEL version="${VERSION}"
LABEL summary="DataStax Kubernetes Operator for Apache Cassandra "
LABEL description="The DataStax Kubernetes Operator for Apache Cassandra®. This operator handles the provisioning and day to day management of Apache Cassandra based clusters. Features include configuration deployment, node remediation, and automatic upgrades."

WORKDIR /
COPY --from=builder /workspace/manager /manager
COPY ./LICENSE.txt /licenses/

USER 65532:65532

ENTRYPOINT ["/manager"]
