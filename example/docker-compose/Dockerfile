FROM golang:1.23.2-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o paywall-app ./example/docker-compose/app/

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /build/paywall-app .
COPY --from=builder /build/example/docker-compose/app/static ./static
COPY --from=builder /build/templates ./templates

EXPOSE 8080
CMD ["./paywall-app"]
