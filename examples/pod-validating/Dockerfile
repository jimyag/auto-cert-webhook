FROM golang:alpine AS builder

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o webhook ./examples/pod-validating

# Runtime image
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/webhook .
USER 65532:65532

ENTRYPOINT ["/webhook"]
