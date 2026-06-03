# Rinha de Backend 2026: Fraud Detection (Go + C)

Fraud-detection backend for the [Rinha de Backend 2026](https://github.com/zanfranceschi/rinha-de-backend-2026). For each card transaction it turns the payload into a 14-dimension vector, finds the 5 nearest neighbors among 3,000,000 reference vectors (euclidean, k=5), and approves when `fraud_score = frauds / 5 < 0.6`.

All of it under **1 CPU and 350 MB of RAM** combined, which is the whole point of the edition.

Two endpoints on port `9999`: `GET /ready` and `POST /fraud-score`.

## Architecture

```
client ──TCP :9999──▶ lb (C) ──SCM_RIGHTS fd──▶ api1 (Go)
                         │     ──SCM_RIGHTS fd──▶ api2 (Go)
                         └─ round-robin, then steps out      both mmap the same index.bin
```

- **lb (C)** accepts the TCP connection on 9999, sets `TCP_NODELAY`, and hands the client file descriptor to a worker over a Unix socket via `SCM_RIGHTS`. After that it is out of the path: the worker talks to the client directly, no byte proxying. The rules forbid the LB from inspecting or transforming the payload anyway, and dropping the proxy is what removes a whole network hop.
- **api (Go)** receives the fds and runs one edge-triggered epoll loop. Hand-written HTTP/1.1 + JSON parser (zero allocation), vectorizes to a 14-dim int16 query, searches the index, and writes one of 6 pre-rendered responses. `GOMAXPROCS=1`, GC off.
- **preprocess** runs at Docker build time: downloads the official `references.json.gz`, quantizes the 3M vectors to int16, builds the partitioned index, writes `index.bin`. At startup the worker only `mmap`s it read-only, so it is ready in about zero seconds.

## How the decision is made

The search is **exact**, not approximate. ANN (HNSW/IVF) is faster but misses a few neighbors, and in this edition every detection error is expensive, so I went with a cheap exact search instead:

1. The 3M vectors are partitioned by a 4-bit key over the categorical dims (`is_online`, `card_present`, `unknown_merchant`, and whether there is a previous transaction). 16 partitions, 12 populated, one of them holding a third of the data.
2. A query lands in its partition and walks a **KD-tree** with bounding-box pruning. Across partitions, a stop rule backed by a proven lower bound (`gap`) guarantees no true neighbor is skipped.
3. `fraud_score = frauds among the 5 / 5`, and `approved = fraud_score < 0.6`.

Validated against the official labels (all 54,100 transactions, including the 645 edge cases): **0 false positives, 0 false negatives**. The `gap`-stop returns the same neighbors as a full sweep on every one of the 54,100.

## What makes it fast

- **C load balancer with fd passing** (`SCM_RIGHTS`): no proxy, the worker owns the connection end to end.
- **int16 quantization** (×10000): the whole index fits in ~115 MB, `mmap`ped read-only and shared between the workers.
- **AVX2 SIMD** distance kernel via `simd/archsimd` (Go 1.26, `GOEXPERIMENT=simd`): `VPMADDWD` for the squared euclidean distance, behind a build-tagged `sqDist` with a scalar fallback.
- **Allocation-free hot path** with GC off (`SetGCPercent(-1)` plus a `SetMemoryLimit` backstop), so no GC pause steals the single core.
- **Kernel busy-poll** (`SO_BUSY_POLL` plus the epoll `EPIOCSPARAMS` ioctl) and **index warmup** + `mlock` at startup to flatten the cold tail.

## Scoring

The official score is `score_p99 + score_det`, each capped at 3000:

```
score_p99 = 1000 · log10(1000 / max(p99_ms, 1))           -> 3000 iff p99 <= 1 ms
score_det = 1000 · log10(1 / max(eps, 0.001)) - 300 · log10(1 + E)
            E = 1*FP + 3*FN + 5*errors                    -> 3000 iff E = 0
```

Maxing it (6000) needs both at once: `E = 0` and `p99 <= 1 ms`. Under the official k6 this submission scores **6000**, with `0 FP / 0 FN / 0 HTTP errors` and p99 under 1 ms.

## Stack

| Piece | What |
|---|---|
| Workers | Go 1.26, hand-rolled epoll, `simd/archsimd` (AVX2) |
| Load balancer | C, `accept4` + `sendmsg(SCM_RIGHTS)` |
| Index | partitioned KD-tree, int16, mmap |
| Images | distroless (api), scratch (lb), `linux/amd64` |

## Run

```sh
docker compose up --build -d        # 1 lb + 2 api on port 9999
curl localhost:9999/ready
```

The build downloads the official `references.json.gz` and generates the index. Exactness suite and local benchmark:

```sh
go test ./...                       # vectorization, search, exactness fuzz
go run ./cmd/bench -rate 900        # load generator: measures p99, estimates the score
```

## Layout

```
cmd/api          worker: mmap the index + epoll loop
cmd/preprocess   builds index.bin from references.json.gz
cmd/bench        local load generator (p99 + estimated score)
cmd/validate*    offline validation against the official labels
internal/vectorize   14 dimensions, normalization, zero-alloc parser
internal/index       partitioned index, KD-tree, search, mmap, SIMD kernels
internal/server      epoll, fd receive, HTTP parser, responses
lb/lb.c          load balancer
```

## Resource budget

| Service | CPU | Memory |
|---|---|---|
| api1 | 0.45 | 160 MB |
| api2 | 0.45 | 160 MB |
| lb | 0.10 | 20 MB |

## Compliance

Test and preview payloads are never used as references, lookup keys, training data, or decision tables. The only source for the index is the official `references.json.gz`. The transaction `id` is ignored by the parser. The decision path is only: payload, 14-dim vector, 5 nearest neighbors in the index, `frauds / 5`.

Licensed under MIT.
