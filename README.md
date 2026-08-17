# tide

Realtime **serve + train** framework for Welvet — find which **dtype × quant ×
training path × arch** adapts best under live load on the **SIMD** backend.

Tide is **dataset-agnostic**. A host supplies a `runner.Dataset` and a `chain.Spec`;
the dashboard, Lucy metrics, permute matrix, and checkpoints stay the same.
[`live_mnist`](../live_mnist) is one host (MNIST 80/20 classification).

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

Task (host-defined): classify while serving. MNIST host uses mid-stream flip
phases **A → B (`label=(label+5)%10`) → A2** to force re-adaptation (same role as
sine frequency switches in test41).

---

## Matrix

| Dimension | Values |
|-----------|--------|
| Backend | **SIMD only** |
| DType | `core.AllDTypes` (full) |
| Quant | `quant.AllFormats` |
| Modes | Lucy 6 (`sgd`…`step_tween_chain`) **plus** every other named Welvet TrainMode (Split / Alt / FastProxy / Sparse / Mesh*). Old Lucy tokens stay frozen so checkpoints resume. |
| Arch | `cnn` (single×1), `bicameral` (×2), `tricameral` (×3) |

**Removed:** `tween_head` / `*_simd` twin modes / CPU-tiled backends.

### Architectures

**cnn** — `CNN2 → CNN2 → Dense → 10` (single, cams=1)  
**bicameral** — `CNN2 → CNN2 → Dense → Parallel(2×Dense, add) → Dense → 10`  
**tricameral** — same with **3** hemispheres

Credit modes (FastProxy, Sparse, …) run `TrainStackMSE` on the Dense sandwich; CNN stem gets Tween-style local gaps. Mesh* on this bench collapses to the family (no volumetric grid).

| Mode | Lucy / test41 analog |
|------|----------------------|
| `sgd` | NormalBP |
| `step_sgd` | StepBP (3 warm forwards) |
| `tween` | Tween (layerwise gaps) |
| `tween_chain` | TweenChain |
| `step_tween` | StepTween |
| `step_tween_chain` | StepTweenChain |
| `TweenSplit` / `FastProxy` / `Sparse` / … | Welvet `parallel.AllNamedTrainModes()` (new IDs only) |

---

## Packages

| Package | Role |
|---------|------|
| `metrics` | SoftAcc + duty-cycle Availability + Score |
| `permute` | dtype × format × mode × arch @ SIMD |
| `pulse` | live run state for the dashboard |
| `dash` | HTTP + HTML charts (1s poll). JSON: `/api/live`, `/api/board`, `/api/meta`, `/api/winners`, `/api/start` (CORS `*`) |
| `ocean` | tide-of-tides: poll many dashboards, consolidate best mode/dtype |
| `runner` | concurrent serve + train pulses (`Config.Build` optional; nil keeps `chain.Model`) |
| `chain` | CNN / Bi / Tri Welvet models |

## Quick start

Any host that builds a `[]permute.Cell`, a `runner.Dataset`, and a `dash.Server`:

```bash
cd ../live_mnist
go run . -addr :8080 -mode smoke
# open http://127.0.0.1:8080
```

Set `dash.Server.Task` / `Subtitle` so the page names the workload (MNIST, sine, …).

Ocean mode (another tide that **does not train**) links any running dashboards:

```bash
# watch live_mnist + a layer sprint together
cd ../quick_sprint
go run . -ocean-only -peers http://127.0.0.1:8080,http://127.0.0.1:8101
# open http://127.0.0.1:8090
```

See [`quick_sprint`](../quick_sprint) for one tide per Welvet layer.

## Epoch default

Each permutation trains **one pass** over the train split (`-train-n`, default 8000 on the MNIST host).  
Finish the matrix → re-run starts **epoch N+1**. Ctrl+C or dashboard **Resume** continues; **DoneIDs skip finished cells**, so adding new Welvet modes does not replay epoch-1 work.

## Checkpoint / resume

`checkpoint.Store` + runner `CheckpointEvery` persist scores, bests, inflight
weights + train offset.

## Learning speed

| Metric | Meaning |
|--------|---------|
| `time_to_acc25_sec` / `time_to_acc50_sec` | Wall seconds until a 1s **hard Acc** window hit ≥25% / ≥50% |
| `acc_per_sec` | Final SoftAcc ÷ duration |
| `mobile_acc_per_sec` | Acc/sec ÷ model MiB |
