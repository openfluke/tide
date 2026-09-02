# River (in Tide)

Compare / Acc-keep / LPD / throughput analytics — now part of Tide on the same port.

```go
import (
    "github.com/openfluke/tide/dash"
    "github.com/openfluke/tide/river"
)

st := river.NewStore(path, host, "near80", trainN, seed, lrs)
st.SetPlan(cells)

srv := &dash.Server{
    Tracker: tr,
    Cells:   cells,
    Addr:    *addr,
    Task:    "MNIST near80",
    River:   st,
    RiverOpts: river.Options{
        Title:       "MNIST near80",
        Subtitle:    "Acc-keep · full 80/20",
        PDFFilename: "near80_compare.pdf",
    },
}
srv.ListenAndServe()
```

| Route | Purpose |
|-------|---------|
| `/compare` | Compare charts (lean, mode×dtype grids, dense score) |
| `/near` | Acc-keep band (default ≥70% of champ) |
| `/lpd` | LPD search |
| `/thru` | Throughput ranking |
| `/api/river/*` | JSON + full-site PDF (compare, near, LPD, thru) |

Standalone `river.Start(addr, st, opts)` still works for a separate port (legacy hosts).
