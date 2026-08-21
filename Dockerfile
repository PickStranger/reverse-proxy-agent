# ---- Build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /rev ./cmd

# ---- Runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /rev /app/rev
COPY --from=builder /app/static /app/static

EXPOSE 8080

ENV AIServerAddr=""
ENV UseMockAI="true"

ENTRYPOINT ["/app/rev"]
