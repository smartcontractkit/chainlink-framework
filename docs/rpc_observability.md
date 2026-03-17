# RPC Observability (Beholder)

RPC client metrics are published to Beholder and surface in Prometheus/Grafana when `RPCClientBase` is constructed with a non-nil `metrics.RPCClientMetrics`.

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `rpc_request_latency_ms` | Histogram | RPC call latency in milliseconds (per call) |
| `rpc_request_errors_total` | Counter | Total number of failed RPC requests |

Labels: `env`, `network`, `chain_id`, `rpc_provider`, `call` (e.g. `latest_block`, `latest_finalized_block`).

## Example Prometheus / Grafana Queries

### Latency over time

- **p99 latency by env and chain:**
  ```promql
  histogram_quantile(0.99, sum(rate(rpc_request_latency_ms_bucket[5m])) by (le, env, network, chain_id))
  ```
- **p50 latency for a given environment:**
  ```promql
  histogram_quantile(0.5, sum(rate(rpc_request_latency_ms_bucket{env="staging"}[5m])) by (le, network, chain_id))
  ```

### Error rate over time

- **Errors per second by env and chain:**
  ```promql
  sum(rate(rpc_request_errors_total[5m])) by (env, network, chain_id, rpc_provider)
  ```
- **Error rate for a specific RPC provider:**
  ```promql
  sum(rate(rpc_request_errors_total{rpc_provider="primary"}[5m])) by (env, network, chain_id)
  ```

### Request rate

- **Requests per second by call type:**
  ```promql
  sum(rate(rpc_request_latency_ms_count[5m])) by (call, env, network)
  ```

## Enabling metrics

Create `RPCClientMetrics` with `metrics.NewRPCClientMetrics(metrics.RPCClientMetricsConfig{...})` and pass it as the last argument to `multinode.NewRPCClientBase(...)`. The follow-up interface refactor will make it easier for multinode/chain integrations to supply `env`, `network`, `chain_id`, and `rpc_provider`.

## Follow-up: multinode integration (PR 2)

After the metrics module changes are merged, a second PR will:

1. Update `multinode/go.mod`: bump `github.com/smartcontractkit/chainlink-framework/metrics` to the new version that includes `RPCClientMetrics`.
2. Add RPC metrics support in multinode: add optional `rpcMetrics metrics.RPCClientMetrics` to `RPCClientBase` and `NewRPCClientBase`, and call `RecordRequest` in `LatestBlock` and `LatestFinalizedBlock` (with latency and error recording).
