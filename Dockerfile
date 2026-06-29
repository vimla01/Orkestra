# Stage 1: Build
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /orkestra ./cmd/orkestra

# Stage 2: Runtime
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /orkestra /orkestra

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/orkestra"]
