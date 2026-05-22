# Source Asia Backend

Go HTTP service (stdlib + `github.com/google/uuid`): a rolling-window rate limiter and an in-memory product catalog. Listens on `:3000`.

## Quick Start

**Option 1: Go** (requires Go 1.22+)

```sh
go run ./cmd/server
```

**Option 2: Docker**

```sh
docker compose up --build -d
```

## Seeding

The server must be running first. Then, in a second terminal:

```sh
go run ./cmd/seed                  # Go
docker compose exec server ./seed  # Docker
```

This seeds 1,000 products (10 images, 2 videos each), benchmarks `GET /products?limit=20`, then fires 7 `/request` calls to verify the 429 cutoff.

## Architecture and Design Decisions

**Part 1: Rate-limited API.** A per-user slice of timestamps behind a single `sync.Mutex`. Each `POST /request` evicts entries older than 60 seconds, returns 429 if 5 or more remain, otherwise appends now and returns 201. The mutex serialises concurrent callers for the same user, so the 5-per-60s limit holds under parallel load.

**Part 2: Product catalog.** Three separate maps (`meta`, `imageMedia`, `videoMedia`) plus an insertion-order slice. `GET /products` reads only `meta` and the order slice and never touches the media maps, so listing 20 of 1,000 products serialises zero image URLs and stays O(limit).

## Try It (reviewer walkthrough)

A reviewer would create a product, list it, fetch it by id, then trip the rate limiter. Each block is copy-paste standalone.

**macOS / Linux**

```sh
# 1. create a product (copy the returned id)
curl -s -X POST http://localhost:3000/products -H "Content-Type: application/json" \
  -d '{"name":"Widget A","sku":"SKU-001","image_urls":["https://cdn.example.com/img-1.jpg"],"video_urls":[]}'

# 2. list products (metadata only, no media URLs)
curl -s "http://localhost:3000/products?offset=0&limit=5"

# 3. fetch one product by id (full media)
curl -s http://localhost:3000/products/PASTE_ID_HERE

# 4. trip the rate limiter: 5 accepted, then 429
for i in $(seq 7); do curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:3000/request \
  -H "Content-Type: application/json" -d '{"user_id":"bob","payload":1}'; done
```

**Windows (PowerShell)**

```powershell
# 1. create a product (copy the returned id)
Invoke-RestMethod -Method Post -Uri http://localhost:3000/products -ContentType "application/json" `
  -Body '{"name":"Widget A","sku":"SKU-001","image_urls":["https://cdn.example.com/img-1.jpg"],"video_urls":[]}'

# 2. list products (metadata only, no media URLs)
Invoke-RestMethod -Uri "http://localhost:3000/products?offset=0&limit=5"

# 3. fetch one product by id (full media)
Invoke-RestMethod -Uri http://localhost:3000/products/PASTE_ID_HERE

# 4. trip the rate limiter: 5 accepted, then 429
1..7 | ForEach-Object { try { Invoke-WebRequest -Method Post -Uri http://localhost:3000/request `
  -ContentType "application/json" -Body '{"user_id":"bob","payload":1}' | Select-Object -Expand StatusCode } `
  catch { $_.Exception.Response.StatusCode.value__ } }
```

## Postman

The repo includes `SourceAsia.postman_collection.json`, an importable collection with every endpoint plus a **429 Demo** folder of 7 `/request` calls.

To import:

1. Open Postman (signing in may be required) and click **Import** (top left).
2. Drag in `SourceAsia.postman_collection.json` and confirm.
3. The `baseUrl` variable defaults to `http://localhost:3000`, so the requests work as soon as the server is running.

For `GET /products/{id}` and `POST /products/{id}/media`, paste a real id (from a `POST /products` response) into the request's `id` path variable.

## Reference

See [API.md](API.md) for full endpoint contracts, status codes, and request/response shapes.

## Known Limitations

Single-instance and in-memory only. A restart resets all rate-limit counters and the product catalog, and the 5-per-60s limit is not enforced across multiple server instances.
