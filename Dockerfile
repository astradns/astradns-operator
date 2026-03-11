# Build the manager binary
FROM golang:1.26.1 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY astradns-operator/go.mod astradns-operator/go.mod
COPY astradns-operator/go.sum astradns-operator/go.sum
COPY astradns-types/go.mod astradns-types/go.mod
COPY astradns-types/go.sum astradns-types/go.sum

WORKDIR /workspace/astradns-operator
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the Go source (relies on .dockerignore to filter)
COPY astradns-operator /workspace/astradns-operator
COPY astradns-types /workspace/astradns-types

# Build
# the GOARCH has no default value to allow the binary to be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/astradns-operator/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
