# tide

Realtime **serve + train** framework for Welvet — find which **numerical type × quant × training path** adapts best under live load.

Inspired by loom/lucy dense mid-stream adaptation:

```
Score = Throughput × Availability% × AvgAccuracy% / 10_000
```

- **Throughput** — serve inferences / second  
- **Availability%** — `(duration − blocked_train_time) / duration × 100`  
- **AvgAccuracy%** — rolling window accuracy while serving  

## Packages

| Package | Role |
|---------|------|
| `metrics` | Lucy Score + window rolls |
| `permute` | dtype × format × train-mode × backend matrix |
| `pulse` | live run state for the dashboard |
| `dash` | HTTP + HTML charts (1s poll) |
| `runner` | concurrent serve + train pulses |
| `chain` | CNN2 → CNN2 → flatten → Dense Welvet model |

## Quick start

Used by [`live_mnist`](../live_mnist):

```bash
cd ../live_mnist
go run . -addr :8080 -mode smoke
# open http://127.0.0.1:8080
```

## Train modes

| Mode | Lucy analog |
|------|-------------|
| `sgd` / `sgd_simd` | NormalBP |
| `step_sgd` / `step_sgd_simd` | Step+BP (3 warm forwards) |
| `tween` / `tween_simd` | Tween (layerwise gaps) |
| `tween_chain` / `tween_chain_simd` | TweenChain |
| `step_tween` / `step_tween_simd` | StepTween |
| `step_tween_chain` / `step_tween_chain_simd` | StepTweenChain |
| `tween_head` / `tween_head_simd` | head-only gap baseline |

SIMD rows honor `welvet/simd.Enabled()` (AVX2/NEON when linked).

Dashboard `/api/live` includes a **server-side `history`** cache (also `history.json` on disk) so refresh / other machines see the same timeline; scrubber goes back in time.

## Checkpoint / resume

`checkpoint.Store` + runner `CheckpointEvery` (default 1m) persist:

- Lucy scores / completed cells / **best Score · Throughput · Availability · Accuracy**
- inflight model weights so Ctrl+C → restart continues mid-cell
