# Stage 1: Build the React frontend
FROM node:24-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build the Go binary (with embedded frontend)
FROM golang:1.26.11-alpine AS builder
WORKDIR /app
# Copy Go source
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy the built frontend into the right location for go:embed
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/myfi ./cmd/server

# Stage 3: Minimal production image, running as an unprivileged user
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/myfi /bin/myfi
EXPOSE 8080
ENTRYPOINT ["/bin/myfi"]
