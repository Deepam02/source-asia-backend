# Source Asia – Backend Assignment

Two-part HTTP service written in Go (standard library only, except `github.com/google/uuid` for ID generation).

> **AI disclosure:** Claude Code (Anthropic) was used to scaffold and review this implementation.

---

## Table of contents

1. [Quick start](#quick-start)
2. [Part 1 – Rate-limited API](#part-1--rate-limited-api)
3. [Part 2 – Product catalog](#part-2--product-catalog)
4. [API reference](#api-reference)
5. [Seed and test runner](#seed-and-test-runner)
6. [Production notes](#production-notes)

---

## Quick start

### With `go run` (local)

```sh
# requires Go 1.22+
go run ./cmd/server
# → listening on :3000
```

### With Docker Compose

```sh
docker compose up --build -d
# server is available at http://localhost:3000
```

The Dockerfile is a two-stage build: `golang:1.24-alpine` compiles both
binaries (`server` and `seed`); `alpine:3.20` carries only the final
static binaries. `CGO_ENABLED=0` produces a fully static binary with no
libc dependency.

---

## Part 1 – Rate-limited API

### Window design

The rate limiter uses a **rolling (sliding) window** of 60 seconds.
Each user's state stores a slice of `time.Time` timestamps for accepted
requests. On every call to `Allow`:

1. All timestamps older than `now − 60s` are evicted from the slice.
2. If `len(timestamps) >= 5`, the request is rejected (429) and a
   cumulative rejected counter is incremented.
3. Otherwise the current timestamp is appended, an accepted counter is
   incremented, and `Allow` returns `true`.

A rolling window was chosen over a fixed window because it prevents the
boundary burst problem: a fixed window allows up to 10 accepts in the two
seconds straddling a minute boundary, which violates the intent of "5 per
minute".

### Concurrency

A single `sync.Mutex` protects all state inside `Limiter`. Both `Allow`
and `Stats` acquire the lock for their full duration. Parallel calls for
the same `user_id` are serialised at the mutex, so the invariant of at
most 5 accepts per 60-second window holds regardless of concurrency.

### Stats window

`GET /stats` returns:

| field | meaning |
|---|---|
| `accepted` | accepted requests **in the current rolling 60-second window** (eviction runs inside `Stats` before counting `len(timestamps)`) |
| `rejected` | **cumulative** rejected count since the server started (resets on restart) |

### Status codes

| case | code |
|---|---|
| accepted | `201 Created` |
| rate-limited | `429 Too Many Requests` |
| invalid input | `400 Bad Request` |

`201` was chosen for accepted requests because the spec permits either
`201` or `200` and `201` reflects that a new accepted record has been
recorded in the window.

### Production limitations

- **Single instance only.** Window state lives in process memory. A second
  instance has no knowledge of the first's counters, so the 5-per-minute
  limit is not enforced across instances.
- **No persistence.** A restart resets all windows and counters. Any
  request budget a user had consumed is lost.
- **No eviction of inactive users.** `userState` entries accumulate in the
  map for the lifetime of the process. Under a large number of distinct
  `user_id` values this is a slow memory leak.
- **Clock skew.** Relies on the local wall clock (`time.Now()`). NTP jumps
  or clock skew can momentarily distort window boundaries.
- **In production:** use Redis with a sliding-window Lua script (or a
  dedicated rate-limit service) so state is shared across instances and
  survives restarts.

---

## Part 2 – Product catalog

### Storage model

The catalog `Store` holds three separate in-memory structures under one
`sync.RWMutex`:

```
meta       map[string]Product    // id → {id, name, sku, created_at}
imageMedia map[string][]string   // id → ordered image URL slice
videoMedia map[string][]string   // id → ordered video URL slice
order      []string              // insertion-order ID list for stable pagination
```

Images and videos are stored in **separate maps** rather than a single
combined list so that the list query path never touches either media map.

### List vs detail query path

| operation | maps read | media URLs loaded |
|---|---|---|
| `GET /products` | `meta`, `order` | **none** |
| `GET /products/{id}` | `meta`, `imageMedia`, `videoMedia` | all for that product |

With 1,000 products × 10 images each, a `GET /products?limit=20` request
reads exactly 20 `Product` structs from `meta`. None of the 10,000 image
URL strings are allocated or serialised — the list holds a read lock only
long enough to copy 20 small structs.

### URL validation rules

Every URL in `image_urls` or `video_urls` must satisfy:

1. Scheme is `http://` or `https://` (checked with `strings.HasPrefix`).
2. Total byte length ≤ 2048 characters.

Validation runs before any mutation. The first failing URL returns `400`
with the offending value in the error message.

### Per-request media limits

At most **20 URLs per array** per request (applies to `image_urls` and
`video_urls` independently). Exceeding either limit returns `400`.
Duplicate URLs within a batch are silently de-duplicated; a URL already
stored for a product is never stored twice.

### Pagination

| parameter | default | valid range | out-of-range behaviour |
|---|---|---|---|
| `offset` | `0` | `>= 0` | `< 0` returns `400` |
| `limit` | `20` | `>= 1` | `< 1` returns `400`; no upper cap is enforced |

Results are returned in **insertion order** (the order products were
created via `POST /products`), which is stable across requests.

---

## API reference

All endpoints accept and return `application/json`. Every error response
has the shape:

```json
{"error": "human-readable message"}
```

Every request is logged to stdout: `METHOD /path STATUS latency`.

---

### `POST /request`

Submit a rate-limited request.

**Request body**

```json
{
  "user_id": "alice",
  "payload": <any JSON value>
}
```

Both fields are required. `payload` accepts any valid JSON value (object,
array, string, number, boolean). Unknown fields return `400`.

**Responses**

| status | body | condition |
|---|---|---|
| `201 Created` | `{"status":"accepted"}` | within the 5-per-60s window |
| `429 Too Many Requests` | `{"error":"rate limit exceeded"}` | window exhausted |
| `400 Bad Request` | `{"error":"..."}` | missing/empty `user_id`, missing `payload`, or invalid JSON |

```sh
curl -s -X POST http://localhost:3000/request \
  -H "Content-Type: application/json" \
  -d '{"user_id":"alice","payload":{"action":"click"}}'
# → {"status":"accepted"}
```

---

### `GET /stats`

Return rate-limit statistics for a user.

**Query parameters**

| param | required |
|---|---|
| `user_id` | yes |

**Response – `200 OK`**

```json
{
  "user_id": "alice",
  "accepted": 3,
  "rejected": 1
}
```

`accepted` is the count within the current rolling window. `rejected` is
cumulative. A user that has never made a request returns zeros for both.

**Responses**

| status | condition |
|---|---|
| `200 OK` | always (including unknown users, which return zeros) |
| `400 Bad Request` | missing or empty `user_id` query parameter |

```sh
curl -s "http://localhost:3000/stats?user_id=alice"
```

---

### `POST /products`

Create a new product.

**Request body**

```json
{
  "name": "Widget A",
  "sku": "SKU-001",
  "image_urls": [
    "https://cdn.example.com/products/sku-001/img-1.jpg",
    "https://cdn.example.com/products/sku-001/img-2.jpg"
  ],
  "video_urls": [
    "https://cdn.example.com/products/sku-001/demo.mp4"
  ]
}
```

`name` and `sku` are required and must be non-empty after whitespace
trimming. `image_urls` and `video_urls` are optional (omit or send `[]`).
Each array is limited to 20 items. Unknown fields return `400`.

**Responses**

| status | body | condition |
|---|---|---|
| `201 Created` | full product (see below) | success |
| `409 Conflict` | `{"error":"SKU already exists"}` | duplicate `sku` |
| `400 Bad Request` | `{"error":"..."}` | missing name/sku, URL validation failure, array over 20 items, invalid JSON |

**201 response body**

```json
{
  "id": "e3d9f1a2-4b5c-6d7e-8f9a-0b1c2d3e4f5a",
  "name": "Widget A",
  "sku": "SKU-001",
  "created_at": "2025-05-19T10:00:00Z",
  "image_urls": [
    "https://cdn.example.com/products/sku-001/img-1.jpg",
    "https://cdn.example.com/products/sku-001/img-2.jpg"
  ],
  "video_urls": [
    "https://cdn.example.com/products/sku-001/demo.mp4"
  ]
}
```

`image_urls` and `video_urls` are always present as arrays (empty `[]`
when none were supplied).

```sh
curl -s -X POST http://localhost:3000/products \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Widget A",
    "sku": "SKU-001",
    "image_urls": ["https://cdn.example.com/sku-001/img-1.jpg"],
    "video_urls": ["https://cdn.example.com/sku-001/demo.mp4"]
  }'
```

---

### `GET /products`

List products. Returns metadata only — `image_urls` and `video_urls` are
**never included** in list items regardless of what is stored.

**Query parameters**

| param | default | description |
|---|---|---|
| `offset` | `0` | products to skip |
| `limit` | `20` | products to return |

**Response – `200 OK`**

```json
{
  "products": [
    {
      "id": "e3d9f1a2-...",
      "name": "Widget A",
      "sku": "SKU-001",
      "created_at": "2025-05-19T10:00:00Z"
    }
  ]
}
```

`products` is always an array. It is `[]` when `offset` is beyond the
last product.

**Responses**

| status | condition |
|---|---|
| `200 OK` | success |
| `400 Bad Request` | `offset` < 0 or `limit` < 1 |

```sh
curl -s "http://localhost:3000/products?offset=0&limit=20"
```

---

### `GET /products/{id}`

Fetch a single product with all media.

**Responses**

| status | body | condition |
|---|---|---|
| `200 OK` | full product (same shape as `POST /products` 201) | found |
| `404 Not Found` | `{"error":"product not found"}` | unknown id |

```sh
curl -s http://localhost:3000/products/e3d9f1a2-4b5c-6d7e-8f9a-0b1c2d3e4f5a
```

---

### `POST /products/{id}/media`

Append image and/or video URLs to an existing product. Already-stored URLs
are silently de-duplicated; they are not re-added.

**Request body**

```json
{
  "image_urls": ["https://cdn.example.com/products/sku-001/img-3.jpg"],
  "video_urls": []
}
```

At least one of `image_urls` or `video_urls` must be a non-empty array.
Each array independently enforces the 20-item limit and URL validation.

**Responses**

| status | body | condition |
|---|---|---|
| `200 OK` | full updated product | success |
| `404 Not Found` | `{"error":"product not found"}` | unknown id |
| `400 Bad Request` | `{"error":"..."}` | both arrays empty/absent, limit exceeded, URL invalid, invalid JSON |

```sh
curl -s -X POST \
  http://localhost:3000/products/e3d9f1a2-4b5c-6d7e-8f9a-0b1c2d3e4f5a/media \
  -H "Content-Type: application/json" \
  -d '{"image_urls":["https://cdn.example.com/sku-001/img-3.jpg"],"video_urls":[]}'
```

---

## Seed and test runner

`cmd/seed` requires the server to be running on port 3000.

```sh
# terminal 1
go run ./cmd/server

# terminal 2
go run ./cmd/seed
```

Or against the Docker container:

```sh
docker compose up --build -d
docker compose exec server ./seed
```

The runner executes three phases in order:

**Phase 1 – seed 1,000 products**
Creates 1,000 products via `POST /products`. Each has 10 `image_urls` and
2 `video_urls` using invented `cdn.example.com` paths. Progress is printed
every 100 products. Total seed wall time is reported.

**Phase 2 – list latency benchmark**
Calls `GET /products?limit=20` once and prints the HTTP status, the number
of items returned (always 20), and the measured round-trip latency.
Because the list path never reads any media map, latency is O(limit)
regardless of how many URL strings are stored.

**Phase 3 – rate limiter smoke test**
Fires 7 sequential `POST /request` calls for `user_id = "seed-test-user"`.
Prints the HTTP status code and accepted/rejected verdict for each.

Expected output:
```
[1] HTTP 201 — accepted
[2] HTTP 201 — accepted
[3] HTTP 201 — accepted
[4] HTTP 201 — accepted
[5] HTTP 201 — accepted
[6] HTTP 429 — rejected
[7] HTTP 429 — rejected
```

---

## Production notes

### Moving to PostgreSQL

The in-memory split-storage model maps cleanly to a relational schema:

```sql
CREATE TABLE products (
  id         UUID PRIMARY KEY,
  name       TEXT NOT NULL,
  sku        TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE product_images (
  id         UUID PRIMARY KEY,
  product_id UUID NOT NULL REFERENCES products(id),
  url        TEXT NOT NULL,
  position   INT  NOT NULL
);

CREATE TABLE product_videos (
  id         UUID PRIMARY KEY,
  product_id UUID NOT NULL REFERENCES products(id),
  url        TEXT NOT NULL,
  position   INT  NOT NULL
);
```

**List query** – identical isolation as the in-memory version: reads only
`products`, no join:

```sql
SELECT id, name, sku, created_at
FROM products
ORDER BY created_at, id
LIMIT $1 OFFSET $2;
```

For large tables, replace `OFFSET` with a keyset cursor:
`WHERE (created_at, id) > ($last_created_at, $last_id)`.

**Detail query** – two additional selects (or a lateral join) to fetch
image and video rows for the single requested product.

### Moving to a CDN

The API stores and returns URL strings only; it never accepts binary data.
To integrate with a real CDN:

1. Add an upload endpoint that streams the file to S3 (or another object
   store) and returns the resulting public URL.
2. Clients call `POST /products/{id}/media` with that URL — the catalog
   layer is unchanged.
3. Optionally persist the first image URL as `thumbnail_url` on the
   `products` row so the list endpoint can surface it without any join.

### Rate limiter

Replace the in-process mutex + timestamp-slice with a Redis sorted-set
sliding window:

```lua
-- atomic Lua script
local key    = "rl:" .. user_id
local now    = tonumber(ARGV[1])   -- milliseconds
local window = 60000
local limit  = 5
redis.call("ZREMRANGEBYSCORE", key, 0, now - window)
local count = redis.call("ZCARD", key)
if count < limit then
  redis.call("ZADD", key, now, now .. math.random())
  redis.call("PEXPIRE", key, window)
  return 1   -- accepted
end
return 0     -- rejected
```

This is atomic, shared across all server instances, and survives process
restarts with Redis persistence enabled.
