FROM golang:1.26.1-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /agentregistry-server ./cmd/server

FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /agentregistry-server /agentregistry-server

EXPOSE 9000 8080

ENTRYPOINT ["/agentregistry-server"]
