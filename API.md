# API Reference

All endpoints accept and return `application/json`. Every request is logged to stdout as `METHOD /path STATUS latency`.

Every error response has the shape:

```json
{"error": "human-readable message"}
```

Base URL: `http://localhost:3000`

---

## POST /request

Submit a rate-limited request.

**Request body**

```json
{
  "user_id": "alice",
  "payload": {"action": "click"}
}
```

Both fields are required. `user_id` must be a non-empty string after whitespace trimming. `payload` accepts any valid JSON value (object, array, string, number, boolean). Unknown fields return 400.

**Responses**

| Status | Body | Condition |
|---|---|---|
| `201 Created` | `{"status":"accepted"}` | within the 5-per-60s window |
| `429 Too Many Requests` | `{"error":"rate limit exceeded"}` | window exhausted |
| `400 Bad Request` | `{"error":"..."}` | missing/empty `user_id`, missing `payload`, or invalid JSON |

---

## GET /stats

Return rate-limit statistics for a user.

**Query parameters**

| Parameter | Required | Description |
|---|---|---|
| `user_id` | yes | the user to query |

**Response: 200 OK**

```json
{
  "user_id": "alice",
  "accepted": 3,
  "rejected": 1
}
```

`accepted` is the count within the current rolling 60-second window (stale timestamps are evicted before counting). `rejected` is cumulative since server start. A user that has never made a request returns zeros for both fields.

**Responses**

| Status | Condition |
|---|---|
| `200 OK` | always (including unknown users, which return zeros) |
| `400 Bad Request` | missing or empty `user_id` query parameter |

---

## POST /products

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

`name` and `sku` are required and must be non-empty after whitespace trimming. `image_urls` and `video_urls` are optional (omit or send `[]`). Each array is limited to 20 items independently. Unknown fields return 400.

**URL validation** (applies to both `image_urls` and `video_urls`):
- Scheme must be `http://` or `https://`.
- Total byte length must be at most 2048 characters.
- Validation runs before any mutation; the first failing URL returns 400 with the offending value in the message.

**Responses**

| Status | Body | Condition |
|---|---|---|
| `201 Created` | full product (see below) | success |
| `409 Conflict` | `{"error":"SKU already exists"}` | duplicate `sku` |
| `400 Bad Request` | `{"error":"..."}` | missing `name`/`sku`, URL validation failure, array over 20 items, or invalid JSON |

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

`image_urls` and `video_urls` are always present as arrays (empty `[]` when none were supplied).

---

## GET /products

List products. Returns metadata only. `image_urls` and `video_urls` are never included in list items regardless of what is stored.

**Query parameters**

| Parameter | Default | Valid range | Out-of-range behaviour |
|---|---|---|---|
| `offset` | `0` | `>= 0` | `< 0` returns 400 |
| `limit` | `20` | `>= 1` | `< 1` returns 400; no upper cap |

Results are returned in insertion order (the order products were created via `POST /products`), which is stable across requests.

**Response: 200 OK**

```json
{
  "products": [
    {
      "id": "e3d9f1a2-4b5c-6d7e-8f9a-0b1c2d3e4f5a",
      "name": "Widget A",
      "sku": "SKU-001",
      "created_at": "2025-05-19T10:00:00Z"
    }
  ]
}
```

`products` is always an array. It is `[]` when `offset` is beyond the last product.

**Responses**

| Status | Condition |
|---|---|
| `200 OK` | success |
| `400 Bad Request` | `offset < 0` or `limit < 1` |

---

## GET /products/{id}

Fetch a single product with all media URLs.

**Path parameters**

| Parameter | Description |
|---|---|
| `id` | UUID of the product |

**Responses**

| Status | Body | Condition |
|---|---|---|
| `200 OK` | full product (same shape as `POST /products` 201 body) | found |
| `404 Not Found` | `{"error":"product not found"}` | unknown id |

---

## POST /products/{id}/media

Append image and/or video URLs to an existing product. Already-stored URLs are silently de-duplicated and never re-added.

**Path parameters**

| Parameter | Description |
|---|---|
| `id` | UUID of the product |

**Request body**

```json
{
  "image_urls": ["https://cdn.example.com/products/sku-001/img-3.jpg"],
  "video_urls": []
}
```

At least one of `image_urls` or `video_urls` must be a non-empty array. Each array independently enforces the 20-item limit and URL validation rules (same as `POST /products`). Unknown fields return 400.

**Responses**

| Status | Body | Condition |
|---|---|---|
| `200 OK` | full updated product | success |
| `404 Not Found` | `{"error":"product not found"}` | unknown id |
| `400 Bad Request` | `{"error":"..."}` | both arrays empty/absent, limit exceeded, URL invalid, or invalid JSON |
