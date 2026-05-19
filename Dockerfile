# ── build stage ────────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS build

WORKDIR /src

# Copy dependency manifests first so this layer is cached when only source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build both binaries. CGO_ENABLED=0 produces a fully static binary that runs
# on scratch/alpine without libc. -trimpath removes local build paths.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server  ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/seed    ./cmd/seed

# ── final stage ────────────────────────────────────────────────────────────────
FROM alpine:3.20

# ca-certificates lets the seed runner make outbound HTTPS calls if needed.
RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /out/server ./server
COPY --from=build /out/seed   ./seed

EXPOSE 3000

ENTRYPOINT ["./server"]
