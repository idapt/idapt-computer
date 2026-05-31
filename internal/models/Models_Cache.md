# models — local catalog cache

`internal/models/cache.go` caches the public model catalog (`GET
/api/v1/models`) to disk so the TUI's `/model` picker opens
instantly on repeat visits instead of round-tripping the server every
time.

The cache is read by `internal/tui/picker_data.go::loadModels` and
written transparently whenever a fresh fetch succeeds.

## File

| Path                          | Mode | Notes                                       |
|-------------------------------|------|---------------------------------------------|
| `~/.idapt/models-cache.json`  | 0600 | Created on first save; survives crashes.    |

JSON shape (top-level versioned, per-key entries inside):

```jsonc
{
  "version": 2,
  "entries": {
    "<sha256(baseURL + apiKey)>": {
      "fetched_at": "2026-05-15T10:21:08Z",
      "models": [
        {
          "id": "claude-opus-4-7",
          "display_name": "Claude Opus",
          "provider": "anthropic",
          "context_window": 200000,
          "input_price": 15, "output_price": 75,
          "vision": true
        }
      ]
    }
  }
}
```

The cached `models` array is the CLI's own flattened `Row` projection —
**not** the raw `/api/v1/models` wire row. The wire carries pricing and
capability data as nested `pricing` / `capabilities` objects
(`pricing.input_per_million`, `capabilities.context_length`,
`capabilities.image_input`); `picker_data.go::fetchModelRows` flattens
those into the `Row` fields above at fetch time. The on-disk format is
versioned independently of the wire so a wire-shape change doesn't force
a cache-format bump unless the projection itself changes.

## TTL

`CacheTTL = 15 * time.Minute`. Entries older than this are still
returned by `LoadFromCache` (the `Fresh()` helper is the freshness
predicate the caller decides on), so callers can:

1. Try fresh → return immediately if within TTL.
2. Fetch from the network on miss / stale.
3. **Stale-fallback**: when the fetch fails, return the stale entry
   rather than render an empty modal.

15 minutes is the sweet spot — short enough that a new model shipped
upstream is visible within one coffee break, long enough that
`/model` reopens in the same session never hit the network twice.

## Cache key

`sha256(baseURL + 0x00 + apiKey)`. Two reasons:

1. **Per-key isolation** — two API keys may have different available
   models (RBAC, billing tier). Hashing on the key makes each
   credential's cache independent.
2. **Per-environment isolation** — the same key against prod vs
   localhost dev cluster sees different models. Hashing on the
   base URL keeps environments apart.

A null byte separator prevents the collision between
`("https://a.com", "bkey")` and `("https://a.comb", "key")`.

## Atomic writes

`SaveToCache` uses temp-file + atomic rename (same pattern as
`internal/cliconfig`). Two concurrent `idapt` processes saving the
same cache cannot corrupt each other's writes — readers always see
either the old file or the new file, never an in-progress write.

## Best-effort tolerance

Every failure mode degrades silently rather than blocking:

| Condition                  | Behavior                                         |
|----------------------------|--------------------------------------------------|
| Missing cache file         | `LoadFromCache` returns `ok=false`.              |
| Torn JSON                  | Returns `ok=false`; next `SaveToCache` rewrites. |
| `version > CurrentVersion` | Returns `ok=false` (future build wrote it).      |
| Save errors                | Returned to caller but never propagated to UI.   |

`Invalidate` is provided for the TUI's `Ctrl+R` "force refetch"
flow — drops the entry for one credential without touching the
others.

## Callers

* `internal/tui/picker_data.go::loadModels` — the only consumer
  today. Reads via `LoadFromCache`, writes via `SaveToCache`,
  honors the `modelPickerForceRefetch` flag on `Model` to bypass
  the cache when the user presses Ctrl+R inside the picker.

## Not yet implemented

* `If-Modified-Since` conditional GETs against the v1 endpoint
  would let us refresh for free when nothing changed. Server-side
  `Last-Modified` header on `/api/v1/models` would unlock this —
  not a blocker for v1, fine to defer.
