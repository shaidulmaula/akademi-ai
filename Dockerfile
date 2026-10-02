# Multi-stage build for Go Standalone Webapp
FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server .

# Final minimal runner
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

ENV TZ=Asia/Jakarta
ENV PORT=8086
ENV DB_PATH=/app/data/courses.db
ENV UPLOAD_DIR=/app/uploads

COPY --from=builder /app/server /app/server

RUN mkdir -p /app/data /app/uploads

EXPOSE 8086

CMD ["/app/server"]
