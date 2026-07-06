FROM golang:1.25-alpine3.23 AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /build/wifi_bot ./cmd/app

FROM alpine:3.23.4

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /build/wifi_bot .
COPY --from=builder /build/configs ./configs

EXPOSE 8080

CMD ["./wifi_bot"]
