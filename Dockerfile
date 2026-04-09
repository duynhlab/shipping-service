FROM docker.io/library/golang:1.25.8-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/shipping-service ./cmd/main.go

FROM alpine:latest
RUN apk upgrade --no-cache && apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/shipping-service .
EXPOSE 8080
CMD ["./shipping-service"]
