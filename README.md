# tide

Realtime **serve + train** framework for Welvet — find which **dtype × quant ×
training path × arch** adapts best under live load on the **SIMD** backend.

Aligned with [`test41_w_sine_ada_perm`](../loom/arcagitesting/test41_w_sine_ada_perm)
Lucy measuring (SoftAcc, duty-cycle Availability, AdaptPct, WeightBytes).

---

## What you are measuring

Three axes at once (same story as test41-w perm):

| Axis | Metrics | Meaning |
|------|---------|---------|
| Adaptation quality | SoftAcc, AdaptPct, Stability, Consistency | How well / how fast the net tracks after mid-stream label flips |
| Duty-cycle availability | Availability, ZeroDowntime | Share of **busy** time spent inferring vs training |
| Cost | WeightBytes, HeapBytes, MobileScore | How small the model is, and Score per MiB |

### Pareto front

A **Pareto front** is the set of options where improving one goal forces you to
hurt another (e.g. Score ↔ WeightBytes, SoftAcc ↔ Availability). Dominated
cells fall off; the interesting winners sit on the undominated edge.

---

## Lucy / test41 score formulas

| Symbol | Formula |
|--------|---------|
| SoftAcc | SoftAcc formula on **true-class softmax prob vs 1.0** (scale 1 → ≈100×p); sine uses scale 0.10 on continuous targets |
| Hard Acc | argmax accuracy % (still recorded as `avg_accuracy`) |
| Availability | `InferMs / (InferMs + TrainMs) × 100` |
| AdaptPct | Mean SoftAcc in the first few pulse windows after each phase switch |
| Throughput (T) | `TotalOutputs / duration_seconds` |
| Score | `T × Availability × SoftAcc / 10_000` |
| ZeroDowntime | `SoftAcc × Availability / 100` |
| MobileScore | `Score / WeightMiB` |

Task (MNIST): classify while serving. Mid-stream flip phases **A → B
(`label=(label+5)%10`) → A2** force re-adaptation (same role as sine frequency
switches in test41).

---

## Matrix

| Dimension | Values |
|-----------|--------|
| Backend | **SIMD only** |
| DType | `core.AllDTypes` (full) |
| Quant | `quant.AllFormats` |
| Modes | `sgd`, `step_sgd`, `tween`, `tween_chain`, `step_tween`, `step_tween_chain` |
| Arch | `cnn`, `bicameral` |

**Removed:** `tween_head` / `*_simd` twin modes / CPU-tiled backends.

### Architectures

**cnn** — `CNN2 → CNN2 → Dense → 10`  
**bicameral** — `CNN2 → CNN2 → Dense → Parallel(Dense∥Dense, add) → Dense → 10`

| Mode | Lucy / test41 analog |
|------|----------------------|
| `sgd` | NormalBP |
| `step_sgd` | StepBP (3 warm forwards) |
| `tween` | Tween (layerwise gaps) |
| `tween_chain` | TweenChain |
| `step_tween` | StepTween |
| `step_tween_chain` | StepTweenChain |

---

## Packages

| Package | Role |
|---------|------|
| `metrics` | SoftAcc + duty-cycle Availability + Score |
| `permute` | dtype × format × mode × arch @ SIMD |
| `pulse` | live run state for the dashboard |
| `dash` | HTTP + HTML charts (1s poll) |
| `runner` | concurrent serve + train pulses |
| `chain` | CNN / Bicameral Welvet models |

## Quick start

```bash
cd ../live_mnist
go run . -addr :8080 -mode smoke
# open http://127.0.0.1:8080
```

## Epoch default

Each permutation trains **one full epoch** over the dataset train split.  
Finish the matrix → re-run starts **epoch N+1**. Ctrl+C resumes mid-epoch.

## Checkpoint / resume

`checkpoint.Store` + runner `CheckpointEvery` persist scores, bests, inflight
weights + train offset.

## Learning speed

| Metric | Meaning |
|--------|---------|
| `time_to_acc25_sec` / `time_to_acc50_sec` | Wall seconds until a 1s **hard Acc** window hit ≥25% / ≥50% |
| `acc_per_sec` | Final SoftAcc ÷ duration |
| `mobile_acc_per_sec` | Acc/sec ÷ model MiB |
