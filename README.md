# tide

Realtime **serve + train** framework for Welvet — find which **dtype × quant ×
training path × arch** adapts best under live load on the **SIMD** backend.

Tide is **dataset-agnostic**. A host supplies a `runner.Dataset` and a `chain.Spec`;
the dashboard, Lucy metrics, permute matrix, and checkpoints stay the same.
[`live_mnist`](../live_mnist) is one host (MNIST 80/20 classification).

Aligned with [`test41_w_sine_ada_perm`](../loom/arcagitesting/test41_w_sine_ada_perm).

Lucy Score is **live-fit**: can the net **learn while it still serves**, in a small
box. That is the synthetic-organism metric. SoftAcc is serve-confidence, **not**
the Acc pillar.

Measuring math lives in [`welvet/lucy`](../welvet/lucy) (`Finalize`, `BuildLPD`).
Tide dash / Lucy PDF only display it. A new host does not copy these formulas.

```
Score = T × Avail × Acc / 10,000
```

---

## What you are measuring

In-house **consciousness benchmark**: what can **run and train at the same time**,
then how far that live-fit can be **memory-condensed** (dtype / quant) without
falling into a trap.

| View | Metrics | Meaning |
|------|---------|---------|
| Pure Acc | Hard Acc (`avg_accuracy`) | Argmax learning. Acc champ is the RAM reference. |
| Throughput | T = outputs / second | Actions per second while the sweep is live. |
| Availability | `InferMs / (InferMs + TrainMs) × 100` | Duty cycle: can you still talk to the model while it trains. |
| Lucy Score | `T × Availability × Acc / 10,000` | Live-fit. Acc is argmax. Availability dies when SGD blocks serve. SoftAcc is **not** this term. |
| Consciousness Q | geomean of Acc/Thru/Avail keep vs **learner** peaks | Learner = Acc keep ≥70% of Acc champ. Chance-Acc tiny dtypes do **not** set Thru/Avail. |
| Lucy density (LPD) | `Q × shrink` vs Acc-champ RAM | Memory intelligence. **0** unless Acc keep ≥70% (weeds Score/MiB traps). |
| Gold | all 3 pillars ≥80% and RAM ≤20% of Acc champ | Trifecta in a small box. |
| Gold-std | Acc ≥80% plus Thru or Avail ≥80%, then smallest then fastest | Two-or-more of the trifecta. |
| Trap | RAM ≤20% of Acc champ and Acc keep &lt;70% | Binary / chance Acc looking dense. |

SoftAcc, AdaptPct, Stability, Consistency remain Welvet adaptation traces. MobileScore (`Score / WeightMiB`) is the binary trap — use LPD.

### Pareto front

A **Pareto front** is the set of options where improving one goal forces you to
hurt another (e.g. Acc ↔ RAM, Acc ↔ Availability). Dominated cells fall off;
goldilocks sits on the undominated edge of **learning vs size vs live duty**.

---

## Lucy / test41 score formulas

| Symbol | Formula |
|--------|---------|
| SoftAcc | SoftAcc formula on **true-class softmax prob vs 1.0** (scale 1 → ≈100×p); sine uses scale 0.10 on continuous targets. Serve-confidence — **not** the Acc pillar. |
| Hard Acc | argmax accuracy % (`avg_accuracy`) — the Acc pillar |
| Availability | `InferMs / (InferMs + TrainMs) × 100` |
| AdaptPct | Mean SoftAcc in the first few pulse windows after each phase switch |
| Throughput (T) | `TotalOutputs / duration_seconds` |
| Score | `T × Availability × Acc / 10_000` (hard Acc; SoftAcc is diagnostic) |
| ZeroDowntime | `Acc × Availability / 100` |
| MobileScore | `Score / WeightMiB` (trap — do not use for goldilocks) |
| Q | geomean(RelAcc, RelThru, RelAvail) vs Acc champ and learner Thru/Avail peaks |
| LPD | `Q × min(AccChampRAM / thisRAM, 32)` if RelAcc ≥ 70%, else 0 |

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
| Arch | `Config.Cams` (Welvet Parallel branches) or named `single`/`bicameral`/`tricameral` for 1/2/3. Hosts pass `-cams 4-15` etc. |

**Removed:** `tween_head` / `*_simd` twin modes / CPU-tiled backends.

### Architectures

**single / bicameral / tricameral** — 1 / 2 / 3 hemispheres (legacy names, live_mnist)  
**cameral×N** — same stem with **N** Welvet Parallel branches (live_gpt default 4–15; any host can set a range)

Old cell IDs used `cnn` for single; checkpoints still resume (`|cnn|` ≡ `|single|`).

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
| `metrics` | Re-export of `welvet/lucy` (SoftAcc, Score, `BuildLPD`) |
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
