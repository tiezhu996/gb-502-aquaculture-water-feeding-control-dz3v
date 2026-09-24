FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /aquaculture-api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app
USER app
COPY --from=builder /aquaculture-api /usr/local/bin/aquaculture-api
EXPOSE 8080
ENTRYPOINT ["aquaculture-api"]
