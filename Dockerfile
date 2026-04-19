FROM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS builder
WORKDIR /app
RUN apk add --no-cache gcc musl-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/internal/api/web/dist ./internal/api/web/dist
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o emerald ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/emerald .
EXPOSE 8080
ENV EMERALD_PORT=8080
ENV EMERALD_DB_PATH=/data/emerald.db
CMD ["./emerald"]
