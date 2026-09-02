const PAGE = document.body.dataset.page || 'live';
const has = (id) => !!document.getElementById(id);

const pieCanvas = document.getElementById('pieChart');
const pieCtx = pieCanvas ? pieCanvas.getContext('2d') : null;
const modeCanvas = document.getElementById('modePie');
const modeCtx = modeCanvas ? modeCanvas.getContext('2d') : null;
const chartPulse = document.getElementById('chart-pulse');
const rankChart = document.getElementById('rankChart');
const learnTo50Chart = document.getElementById('learnTo50Chart');
const learnRateChart = document.getElementById('learnRateChart');
const scrub = document.getElementById('scrub');
const scrubLabel = document.getElementById('scrubLabel');
const liveBtn = document.getElementById('livebtn');

let history = [];
let followLive = true;
let seeded = false;
let lastLive = null;
let chartRev = 0;
let liveETag = '';
const SERVER_CHARTS = [
  'pulse', 'radar-live', 'radar-dens', 'lpd-ram', 'lpd-acc', 'lpd-shrink',
  'heat-score', 'heat-acc', 'heat-soft', 'heat-avail', 'heat-arch', 'heat-arch-avail',
  'scatter-avail-acc', 'scatter-soft-score', 'scatter-acc-score', 'scatter-soft-acc',
  'scatter-thru-acc', 'scatter-thru-avail', 'scatter-adapt-acc',
];
function refreshCharts(rev, pulseEnd) {
  if (!rev) return;
  const pulseOnly = pulseEnd != null && rev === chartRev;
  if (!pulseOnly) chartRev = rev;
  SERVER_CHARTS.forEach(name => {
    const el = document.querySelector(`[data-chart="${name}"]`);
    if (!el) return;
    if (name === 'scatter-adapt-acc') {
      const vs = (lastLive && lastLive.heat && lastLive.heat.vs) || {};
      const show = !!(vs.modes && vs.modes.some(m => Math.abs(m.acc_delta || 0) > 0.05));
      el.style.display = show ? '' : 'none';
      if (!show) return;
    }
    if (pulseOnly && name !== 'pulse') return;
    let url = '/api/charts/' + name + '.svg?v=' + rev;
    if (name === 'pulse' && pulseEnd != null) url += '&end=' + pulseEnd;
    el.src = url;
  });
}
function refreshPulseChart(end) {
  refreshCharts(chartRev || Date.now(), end);
}
let filterRaw = '';
let filterMobile = '';
let filterLearn = '';
let filterLearnMobile = '';
let etaSec = NaN;
let sweepStartMs = Date.now();

function sweepDurations() {
  const rows = (lastLive?.leaderboard || []).filter(r => r.started && r.ended);
  const ms = rows.map(r => new Date(r.ended) - new Date(r.started)).filter(x => x > 0);
  if (!ms.length) return { avg: 0, med: 0, n: 0 };
  ms.sort((a, b) => a - b);
  const avg = ms.reduce((a, b) => a + b, 0) / ms.length;
  const med = ms[Math.floor(ms.length / 2)];
  return { avg, med, n: ms.length };
}

function runningN(j) {
  if (!j) return 0;
  if (j.running_n != null && j.running_n > 0) return j.running_n;
  if ((j.inflight || []).length) return j.inflight.length;
  return j.running ? 1 : 0;
}

function updateETA(j) {
  const total = j.plan || j.cell_total || 0;
  const done = j.epoch_done != null ? j.epoch_done : 0;
  const left = Math.max(0, total - Math.min(done, total) - runningN(j));
  const p = j.pace || {};
  let avg = (p.avg_cell_sec || 0) * 1000;
  let med = (p.med_cell_sec || 0) * 1000;
  let n = p.pace_samples || 0;
  // Prefer server wall-clock farm rate (parallel workers + queue).
  if (p.wall_sec_per_cell > 0) {
    const wallMs = p.wall_sec_per_cell * 1000;
    if (n === 0 || wallMs > med) {
      med = wallMs;
      avg = wallMs;
      if (n === 0) n = 1;
    }
  }
  if (n === 0) {
    const fb = sweepDurations();
    avg = fb.avg;
    med = fb.med;
    n = fb.n;
  }
  const useMs = med > 0 ? med : avg;
  const epochEtaSec = p.eta_epoch_sec > 0 ? p.eta_epoch_sec : (useMs > 0 ? (left * useMs) / 1000 : NaN);
  const sweepEtaSec = p.eta_sweep_sec > 0 ? p.eta_sweep_sec : NaN;
  etaSec = epochEtaSec;
  const rate = p.cells_per_hour > 0 ? p.cells_per_hour : (useMs > 0 ? 3600000 / useMs : 0);
  return { total, done, left, avg, med, n, rate, epochEtaSec, sweepEtaSec };
}

const BANDS = ['#152428','#151c2a','#241c14','#1c1424','#14241c','#241814'];
const METRICS = [
  { key: 'accuracy', snap: 'avg_accuracy', best: 'accuracy', label: 'Hard Acc %', color: '#e6b35a', fixed: 100, digits: 1 },
  { key: 'throughput', snap: 'throughput', best: 'throughput', label: 'throughput /s', color: '#7aa2f7', fixed: null, digits: 1 },
  { key: 'availability', snap: 'availability', best: 'availability', label: 'availability %', color: '#c3a6ff', fixed: 100, digits: 1 },
  { key: 'score', snap: 'score', best: 'score', label: 'lucy score', color: '#3dd6c6', fixed: null, digits: 3 },
];

function fitCanvas(c) {
  if (!c) return false;
  const w = Math.max(280, Math.round(c.clientWidth || c.width || 480));
  const h = Math.max(160, Math.round(c.clientHeight || c.height || 220));
  if (c.width !== w) c.width = w;
  if (c.height !== h) c.height = h;
  return true;
}
function axisName(k) {
  return ({
    availability: 'Avail %', avg_accuracy: 'Hard Acc %', soft_acc: 'SoftAcc',
    score: 'Score', ram: 'RAM KiB', qpct: 'Q %', accpct: 'Acc keep %',
    shrink: 'shrink ×', throughput: 'Thru /s', adapt_pct: 'AdaptPct %',
  })[k] || k;
}
function strokeAxisLabels(ctx, w, h, padL, padB, xk, yk) {
  ctx.fillStyle = '#8aa0ad';
  ctx.font = '12px sans-serif';
  ctx.textAlign = 'center';
  ctx.fillText(axisName(xk), padL + (w - padL - 12) / 2, h - 8);
  ctx.save();
  ctx.translate(14, h / 2);
  ctx.rotate(-Math.PI / 2);
  ctx.fillText(axisName(yk), 0, 0);
  ctx.restore();
}

function snapMetric(row, snapKey) {
  if (!row || !row.snapshot) return null;
  const v = row.snapshot[snapKey];
  return typeof v === 'number' ? v : null;
}

function champForMetric(m) {
  const b = lastLive?.best || {};
  return b[m.best] || null; // Result for this axis
}

function scoreChamp() {
  return lastLive?.best?.score || null;
}

const filterRawEl = document.getElementById('filterRaw');
if (filterRawEl) filterRawEl.addEventListener('input', (e) => {
  filterRaw = (e.target.value || '').toLowerCase();
  renderBoards();
});
const filterMobileEl = document.getElementById('filterMobile');
if (filterMobileEl) filterMobileEl.addEventListener('input', (e) => {
  filterMobile = (e.target.value || '').toLowerCase();
  renderBoards();
});
const filterLearnEl = document.getElementById('filterLearn');
if (filterLearnEl) filterLearnEl.addEventListener('input', (e) => {
  filterLearn = (e.target.value || '').toLowerCase();
  renderBoards();
});
const filterLearnMobileEl = document.getElementById('filterLearnMobile');
if (filterLearnMobileEl) filterLearnMobileEl.addEventListener('input', (e) => {
  filterLearnMobile = (e.target.value || '').toLowerCase();
  renderBoards();
});

function fmtDur(sec) {
  if (!isFinite(sec) || sec < 0) return '—';
  if (sec < 1) return sec < 0.05 ? '—' : '<1s';
  if (sec < 60) return Math.round(sec) + 's';
  if (sec < 3600) return (sec/60).toFixed(1) + 'm';
  const h = Math.floor(sec/3600);
  const m = Math.round((sec%3600)/60);
  return h + 'h ' + m + 'm';
}

function drawDonut(ctx, canvas, slices, centerLines) {
  fitCanvas(canvas);
  const w = canvas.width, h = canvas.height;
  ctx.clearRect(0,0,w,h);
  const cx = w/2, cy = h/2 - 2;
  const R = Math.min(w,h) * 0.38;
  const r = R * 0.58;
  const total = slices.reduce((a,s) => a + s.value, 0) || 1;
  let a0 = -Math.PI/2;
  slices.forEach(s => {
    const a1 = a0 + (s.value/total) * Math.PI*2;
    ctx.beginPath();
    ctx.moveTo(cx, cy);
    ctx.arc(cx, cy, R, a0, a1);
    ctx.closePath();
    ctx.fillStyle = s.color;
    ctx.fill();
    a0 = a1;
  });
  ctx.beginPath();
  ctx.arc(cx, cy, r, 0, Math.PI*2);
  ctx.fillStyle = '#0e1418';
  ctx.fill();
  ctx.textAlign = 'center';
  (centerLines||[]).forEach((line, i) => {
    ctx.font = i === 0 ? '600 14px sans-serif' : '11px sans-serif';
    ctx.fillStyle = i === 0 ? '#3dd6c6' : '#8aa0ad';
    ctx.fillText(line, cx, cy - 4 + i*15);
  });
}

function renderModeQueue(modes) {
  const el = document.getElementById('modeQueue');
  if (!el) return;
  if (!Array.isArray(modes) || !modes.length) {
    el.innerHTML = '<tr><td colspan="6">plan unknown — host did not pass sweep Cells to the dashboard</td></tr>';
    return;
  }
  el.innerHTML = modes.map(m => {
    const pct = m.total > 0 ? Math.round(100 * m.done / m.total) : 0;
    const st = m.running ? 'running' : (m.left === 0 ? 'ok' : '');
    const bar = `<div style="height:6px;background:var(--grid);margin-top:2px"><div style="height:6px;width:${pct}%;background:${m.running ? 'var(--warn)' : 'var(--accent)'}"></div></div>`;
    return `<tr class="${st}">
      <td>${modeChip(m.mode)}</td>
      <td>${m.done}</td>
      <td>${m.running || '—'}</td>
      <td><b>${m.left}</b></td>
      <td>${m.total}</td>
      <td style="min-width:72px">${bar}</td>
    </tr>`;
  }).join('');
}

function fmtSecShort(s) {
  if (s == null || s <= 0) return '—';
  if (s < 60) return s.toFixed(1) + 's';
  return (s/60).toFixed(1) + 'm';
}

function renderWinnerRows(tbodyId, rows, cols) {
  const el = document.getElementById(tbodyId);
  if (!el) return;
  if (!Array.isArray(rows) || !rows.length) {
    el.innerHTML = `<tr><td colspan="${cols}">no finished ok cells yet</td></tr>`;
    return;
  }
  el.innerHTML = rows.map(w => {
    const score = (w.score||0).toFixed(3);
    const soft = (w.soft_acc||0).toFixed(1) + '%';
    const acc = (w.avg_accuracy||0).toFixed(1) + '%';
    const thru = (w.throughput||0).toFixed(1);
    const avail = (w.availability||0).toFixed(1) + '%';
    const t50 = fmtSecShort(w.time_to_acc50_sec);
    const aps = (w.acc_per_sec||0).toFixed(3);
    const id = w.cell_id || w.winner || '';
    if (tbodyId === 'winCellMode') {
      return `<tr class="clickable" data-cell="${escAttr(id)}">
        <td>${modeChip(w.mode || w.group)} ${archBadge(null, id)}</td>
        <td>${score}</td><td>${soft}</td><td>${acc}</td><td>${t50}</td><td>${aps}</td>
        <td>${w.n||0}</td>
        <td class="cellid">${formatCellHTML(null, id)}</td>
      </tr>`;
    }
    return `<tr class="clickable" data-cell="${escAttr(id)}">
      <td>${(w.mode && w.mode === w.group) ? modeChip(w.group) : `<span class="dtype">${w.group||'—'}</span>`}</td>
      <td>${(w.mode && w.mode === w.winner) ? modeChip(w.winner) : `<b>${w.winner||'—'}</b>`}</td>
      <td>${score}</td><td>${soft}</td><td>${acc}</td><td>${thru}</td><td>${avail}</td>
      <td>${w.n||0}</td>
      <td class="cellid">${formatCellHTML(null, id)}</td>
    </tr>`;
  }).join('');
  bindRowClicks(el);
}

function renderSettingsRows(rows) {
  const el = document.getElementById('winSettingsMode');
  if (!el) return;
  if (!Array.isArray(rows) || !rows.length) {
    el.innerHTML = '<tr><td colspan="12">no finished ok cells yet</td></tr>';
    return;
  }
  el.innerHTML = rows.map(w => {
    const id = w.cell_id || '';
    const fmt = !w.format || w.format === 'none' ? 'native' : w.format;
    return `<tr class="clickable" data-cell="${escAttr(id)}">
      <td>${modeChip(w.mode || w.group)}</td>
      <td>${archBadge(null, id)}</td>
      <td class="dtype">${w.dtype || '—'}</td>
      <td><span class="fmt">${fmt}</span></td>
      <td class="num">${(w.score||0).toFixed(3)}</td>
      <td class="num">${(w.soft_acc||0).toFixed(1)}%</td>
      <td class="num">${(w.avg_accuracy||0).toFixed(1)}%</td>
      <td class="num">${(w.availability||0).toFixed(1)}%</td>
      <td class="num">${fmtSecShort(w.time_to_acc50_sec)}</td>
      <td class="num">${(w.acc_per_sec||0).toFixed(3)}</td>
      <td class="num">${(w.weight_kib||0).toFixed(1)}K</td>
      <td class="num">${w.n||0}</td>
    </tr>`;
  }).join('');
  bindRowClicks(el);
}

function renderWinners(w) {
  w = w || {};
  renderSettingsRows(w.best_settings_per_mode);
  renderWinnerRows('winDTypeMode', w.best_dtype_per_mode, 9);
  renderWinnerRows('winFmtMode', w.best_format_per_mode, 9);
  renderWinnerRows('winModeDType', w.best_mode_per_dtype, 9);
  renderWinnerRows('winModeFmt', w.best_mode_per_format, 9);
  renderWinnerRows('winCellMode', w.best_cell_per_mode, 8);
  renderWinnerRows('winFmtDType', w.best_format_per_dtype, 9);
}

function drawProgressPie(eta) {
  if (!pieCanvas || !pieCtx || !lastLive) return;
  eta = eta || updateETA(lastLive);
  const total = lastLive.plan || lastLive.cell_total || 0;
  const done = lastLive.epoch_done != null ? lastLive.epoch_done : 0;
  const ok = lastLive.epoch_ok != null ? lastLive.epoch_ok : done;
  const gap = lastLive.epoch_gap != null ? lastLive.epoch_gap : 0;
  const fail = lastLive.epoch_fail != null ? lastLive.epoch_fail : 0;
  const running = runningN(lastLive);
  const left = Math.max(0, total - Math.min(done, total) - fail - running);
  const barPct = total > 0 ? Math.min(100, 100 * done / total) : 0;
  const barEl = document.getElementById('sweepBar');
  if (barEl) barEl.style.width = barPct.toFixed(1) + '%';
  const slices = [
    { value: ok, color: '#3dd6c6' },
    { value: gap, color: '#7aa2f7' },
    { value: running, color: '#e6b35a' },
    { value: fail, color: '#e06c75' },
    { value: left, color: '#3a4a55' },
  ].filter(s => s.value > 0);
  const pct = total > 0 ? (100*Math.min(done,total)/total).toFixed(1) + '%' : '—';
  const ep = lastLive.epoch || 1;
  const epMax = lastLive.epoch_max || 0;
  const epLeft = lastLive.epochs_left != null ? lastLive.epochs_left
    : (epMax > 0 ? Math.max(0, epMax - ep + 1) : 0);
  const overall = lastLive.epoch_overall_pct != null
    ? Number(lastLive.epoch_overall_pct).toFixed(0) + '%'
    : null;
  const epLabel = epMax > 0 ? `epoch ${ep}/${epMax}` : `epoch ${ep}`;
  const epochEta = eta.sweepEtaSec > 0 ? fmtDur(eta.sweepEtaSec)
    : (epMax > 0 && eta.epochEtaSec > 0 && epLeft > 0 ? fmtDur(eta.epochEtaSec * epLeft) : fmtDur(eta.epochEtaSec));
  const centerLines = epMax > 0
    ? [pct + ' this ep', fmtDur(eta.epochEtaSec) + ' ETA']
    : [pct + ' done', fmtDur(eta.epochEtaSec) + ' ETA'];
  drawDonut(pieCtx, pieCanvas, slices, centerLines);
  const rec = lastLive.recorded || 0;
  const elapsed = (Date.now() - sweepStartMs) / 1000;
  const rateStr = eta.rate > 0 ? eta.rate.toFixed(1) + ' cells/hr' : '—';
  const cellStr = eta.med > 0 ? fmtDur(eta.med / 1000) + ' med' : (eta.avg > 0 ? fmtDur(eta.avg / 1000) + ' avg' : '—');
  document.getElementById('pieAside').innerHTML =
    `<div><span class="swatch" style="background:#3dd6c6"></span>done <b>${done}</b> · ok ${ok}${gap ? ` · gap ${gap}` : ''}</div>` +
    `<div><span class="swatch" style="background:#e6b35a"></span>running <b>${running}</b> · left <b>${left}</b> / ${total}</div>` +
    `<div>${epLabel}${epMax > 0 ? ` · ${epLeft} epoch(s) left · ${overall || '—'} overall` : ''}</div>` +
    (fail ? `<div><span class="swatch" style="background:#e06c75"></span>fail <b>${fail}</b></div>` : '') +
    (rec > total ? `<div>recorded ${rec} across epochs</div>` : '') +
    `<div class="eta">epoch ETA ~ ${fmtDur(eta.epochEtaSec)} · sweep ~ ${epochEta}</div>`;
  const statsEl = document.getElementById('sweepStats');
  if (statsEl) {
    const cur = (() => {
      const inf = lastLive.inflight || [];
      if (inf.length > 1) {
        return `${inf.length} in-flight · ` + inf.slice(0, 3).map(r => prettyCellId(r.cell?.id || '')).join(', ') + (inf.length > 3 ? '…' : '');
      }
      if (inf.length === 1) return prettyCellId(inf[0].cell?.id || lastLive.message || '');
      return lastLive.message ? prettyCellId(lastLive.message) : '—';
    })();
    statsEl.innerHTML = [
      ['Progress', pct + ' · ' + done + '/' + total],
      ['Queue left', String(left)],
      ['Cell pace', cellStr + (eta.n ? ` (${eta.n} samples)` : '')],
      ['Throughput', rateStr],
      ['Elapsed', fmtDur(elapsed)],
      ['Epoch ETA', fmtDur(eta.epochEtaSec)],
      ['Sweep ETA', epochEta],
      ['Phase', lastLive.phase || '—'],
      ['In-flight', cur.length > 64 ? cur.slice(0, 62) + '…' : cur],
      ['Status', runningN(lastLive) > 0 ? `RUNNING ×${runningN(lastLive)}` : (lastLive.awaiting_start ? 'PAUSED' : 'idle')],
      ['Recorded', String(rec)],
      ['History', String(lastLive.history_len || 0) + ' pts'],
    ].map(([k, v]) => `<div class="stat"><span>${k}</span><b>${v}</b></div>`).join('');
  }
}

function modeOf(id) {
  const parts = (id||'').split('|');
  return parts[2] || 'unknown';
}

function drawModePie() {
  if (!modeCanvas || !modeCtx) return;
  const counts = {};
  (lastLive?.mode_progress || []).forEach(m => {
    if (m.done > 0) counts[m.mode] = m.done;
  });
  // Prefer plan order (mode queue) so every train mode stays visible even when counts tie.
  const plan = (lastLive?.mode_progress || []).map(m => m.mode).filter(Boolean);
  const names = plan.length
    ? plan.filter(m => counts[m] != null).concat(Object.keys(counts).filter(m => !plan.includes(m)))
    : Object.keys(counts).sort((a,b) => counts[b]-counts[a] || a.localeCompare(b));
  const entries = names.map(m => [m, counts[m]]);
  const slices = entries.map(e => ({ value: e[1], color: modeColor(e[0]) }));
  if (!slices.length) {
    modeCtx.clearRect(0,0,modeCanvas.width, modeCanvas.height);
    document.getElementById('modeAside').textContent = 'no finished cells yet';
    return;
  }
  drawDonut(modeCtx, modeCanvas, slices, [String(slices.reduce((a,s)=>a+s.value,0)), 'finished']);
  const aside = document.getElementById('modeAside');
  aside.style.maxHeight = '280px';
  aside.style.overflow = 'auto';
  aside.innerHTML = entries.map(e =>
    `<div>${modeChip(e[0])} <b>${e[1]}</b></div>`
  ).join('');
}

function drawBands(ctx, x0, y0, x1, y1, slice) {
  let band = 0, prev = slice[0]?.cell_id, seg = 0;
  const flush = (to) => {
    const xa = x0 + seg * ((x1-x0) / Math.max(slice.length-1,1));
    const xb = x0 + to * ((x1-x0) / Math.max(slice.length-1,1));
    ctx.fillStyle = BANDS[band % BANDS.length];
    ctx.fillRect(xa, y0, Math.max(1, xb-xa), y1-y0);
  };
  for (let i = 1; i < slice.length; i++) {
    if (slice[i].cell_id !== prev) {
      flush(i); band++; prev = slice[i].cell_id; seg = i;
    }
  }
  if (slice.length) flush(slice.length-1);
}

function drawPulse(viewEnd) {
  fitCanvas(pulseCanvas);
  const w = pulseCanvas.width, h = pulseCanvas.height;
  pulseCtx.clearRect(0,0,w,h);
  if (history.length < 2) {
    pulseCtx.fillStyle = '#8aa0ad';
    pulseCtx.font = '13px sans-serif';
    pulseCtx.fillText('waiting for pulses…', 20, 30);
    return;
  }
  const end = Math.min(viewEnd, history.length - 1);
  const start = Math.max(0, end - 240);
  const slice = history.slice(start, end + 1);
  const padL = 52, padR = 12, padTop = 8, gap = 8;
  const panelH = (h - padTop - gap * (METRICS.length - 1)) / METRICS.length;
  const scoreC = scoreChamp();

  METRICS.forEach((m, mi) => {
    const y0 = padTop + mi * (panelH + gap);
    const y1 = y0 + panelH;
    const plotT = y0 + 16;
    const plotB = y1 - 4;

    pulseCtx.fillStyle = 'rgba(0,0,0,0.2)';
    pulseCtx.fillRect(padL, y0, w - padL - padR, panelH);
    drawBands(pulseCtx, padL, plotT, w - padR, plotB, slice);

    pulseCtx.strokeStyle = '#24313a';
    for (let g = 0; g < 3; g++) {
      const y = plotT + g * ((plotB-plotT)/2);
      pulseCtx.beginPath(); pulseCtx.moveTo(padL, y); pulseCtx.lineTo(w-padR, y); pulseCtx.stroke();
    }

    const vals = slice.map(p => p[m.key] || 0);
    const metricC = champForMetric(m);
    const metricChampV = snapMetric(metricC, m.snap);
    const scoreChampV = snapMetric(scoreC, m.snap);
    const maxCand = [Math.max(...vals, 1e-9)];
    if (metricChampV != null) maxCand.push(metricChampV);
    if (scoreChampV != null) maxCand.push(scoreChampV);
    const maxV = m.fixed != null ? m.fixed : Math.max(...maxCand);

    const yAt = (v) => plotB - (v / maxV) * (plotB - plotT);
    const href = (v, style, color) => {
      if (v == null || !isFinite(v)) return;
      const y = yAt(v);
      pulseCtx.save();
      pulseCtx.strokeStyle = color;
      pulseCtx.lineWidth = 1.5;
      if (style === 'dash') pulseCtx.setLineDash([7, 5]);
      else if (style === 'dot') pulseCtx.setLineDash([2, 4]);
      pulseCtx.beginPath();
      pulseCtx.moveTo(padL, y);
      pulseCtx.lineTo(w - padR, y);
      pulseCtx.stroke();
      pulseCtx.restore();
    };

    // Champions first (under live line)
    href(metricChampV, 'dash', 'rgba(255,255,255,0.75)');
    // Only draw score-champ line when it's a different model than metric champ
    if (scoreC && metricC && scoreC.cell?.id !== metricC.cell?.id) {
      href(scoreChampV, 'dot', 'rgba(61,214,198,0.9)');
    } else if (scoreC && !metricC) {
      href(scoreChampV, 'dot', 'rgba(61,214,198,0.9)');
    }

    pulseCtx.strokeStyle = m.color;
    pulseCtx.lineWidth = 2;
    pulseCtx.beginPath();
    slice.forEach((p, i) => {
      const x = padL + i * ((w - padL - padR) / Math.max(slice.length - 1, 1));
      const y = yAt(p[m.key]||0);
      if (i === 0) pulseCtx.moveTo(x, y); else pulseCtx.lineTo(x, y);
    });
    pulseCtx.stroke();

    const tip = vals[vals.length - 1] || 0;
    pulseCtx.fillStyle = m.color;
    pulseCtx.font = '11px sans-serif';
    pulseCtx.textAlign = 'left';
    let label = `${m.label}  live ${tip.toFixed(m.digits)}`;
    if (metricChampV != null) {
      label += `  │ champ ${metricChampV.toFixed(m.digits)}`;
    }
    if (scoreChampV != null && scoreC && (!metricC || scoreC.cell?.id !== metricC.cell?.id)) {
      label += `  │ score-champ ${scoreChampV.toFixed(m.digits)}`;
    }
    pulseCtx.fillText(label, padL + 4, y0 + 12);
  });

  const tip = slice[slice.length - 1];
  if (tip?.cell_id) {
    pulseCtx.fillStyle = '#e6eef2';
    pulseCtx.font = '11px ui-monospace, monospace';
    pulseCtx.textAlign = 'right';
    pulseCtx.fillText(prettyCellId(tip.cell_id), w - padR, 12);
  }
}

const MODE_PALETTE = {
  sgd: '#3dd6c6',
  step_sgd: '#7aa2f7',
  tween: '#e6b35a',
  tween_chain: '#c3a6ff',
  step_tween: '#e06c75',
  step_tween_chain: '#8fd19e',
};
const FALLBACK_PALETTE = [
  '#9ad0f5','#d4a574','#6bcb8a','#ff9ecd','#a8b4c0','#5ec8e6','#c9d66b','#f0a0a0',
];
function hashHue(s) {
  let h = 2166136261;
  for (let i = 0; i < (s||'').length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}
function modeColor(mode) {
  if (MODE_PALETTE[mode]) return MODE_PALETTE[mode];
  return FALLBACK_PALETTE[hashHue(mode || '') % FALLBACK_PALETTE.length];
}
function modeChip(mode) {
  const c = modeColor(mode || '');
  return `<span class="mode-chip" style="color:${c};border-color:${c}66;background:${c}18">${prettyMode(mode) || '—'}</span>`;
}
function escAttr(s) {
  return String(s||'').replace(/\\/g,'\\\\').replace(/"/g,'&quot;').replace(/'/g, '&#39;');
}
function resultById(id) {
  if (!id || !lastLive) return null;
  const pool = [lastLive.current, ...(lastLive.leaderboard || []), ...(lastLive.leaderboard_mobile || [])].filter(Boolean);
  return pool.find(r => r.cell?.id === id) || null;
}
function bindRowClicks(root) {
  if (!root) return;
  root.querySelectorAll('[data-cell]').forEach(el => {
    el.onclick = () => openCellDetail(el.getAttribute('data-cell'));
  });
}

function prettyMode(s) {
  s = String(s || '');
  return s.replace(/FastProxy/g, '[FP]').replace(/HeadProxy/g, '[HP]').replace(/Linear/g, '[L]').replace(/Split/g, '[S]').replace(/Tween/g, '[T]');
}
function prettyCellId(id) {
  id = String(id||'').replace(/\|cnn\|/g, '|single|');
  const p = id.split('|');
  // Classic permute: dtype|format|mode|arch|…
  // test53 funny-LR: layer|dtype|mode|lr=…
  // mixcam: …|NormalBP|arch|…|bm=m0+m1+|pat=alt — show BranchModes as the mode.
  const bmTok = p.find(x => /^bm=/i.test(x));
  const patTok = p.find(x => /^pat=/i.test(x));
  if (bmTok) {
    const modes = bmTok.replace(/^bm=/i, '').split('+').filter(Boolean).map(prettyMode).join('+');
    const pat = patTok ? patTok.replace(/^pat=/i, '') + '·' : '';
    if (p.length >= 3) p[2] = pat + modes;
  } else if (p.length >= 3 && !isLRToken(p[p.length - 1])) {
    p[2] = prettyMode(p[2]);
  } else if (p.length >= 3 && isLRToken(p[p.length - 1])) {
    p[2] = prettyMode(p[2]);
  }
  return p.filter(x => !/^bm=/i.test(x) && !/^pat=/i.test(x)).join('|');
}
function isLRToken(s) {
  return /^lr=/i.test(String(s || '').trim());
}
function isArchToken(s) {
  const t = String(s || '').toLowerCase().replace(/×/g, 'x').trim();
  if (!t || isLRToken(t)) return false;
  if (/^(single|cnn|bi|bicameral|tri|tricameral)$/.test(t)) return true;
  if (/^(cnn|single|cameral|cam|cams)?x?\d+$/.test(t)) return true;
  if (/^\d+-?cameral$/.test(t)) return true;
  return false;
}
function parseCamCount(arch) {
  const s = String(arch || '').toLowerCase().replace(/×/g, 'x').trim();
  if (!s || isLRToken(s)) return 0;
  if (s === 'tricameral' || s === 'tri') return 3;
  if (s === 'bicameral' || s === 'bi') return 2;
  if (s === 'single' || s === 'cnn' || s === 'cnnx1' || s === '') return 1;
  // Only accept explicit cameral/arch suffixes — never raw trailing digits
  // from lr=200 / lr=2 / layer names like cnn2.
  const m = s.match(/^(?:cnn|single|cameral|cam|cams)?x?(\d+)$/);
  if (m) {
    const n = parseInt(m[1], 10);
    return n >= 1 ? n : 0;
  }
  const m2 = s.match(/^(\d+)-?cameral$/);
  if (m2) {
    const n = parseInt(m2[1], 10);
    return n >= 1 ? n : 0;
  }
  return 0;
}
function archTagFor(n, arch) {
  const a = String(arch || '');
  if (n <= 1) return 'single×1';
  if (n === 2 && /^(bicameral|bi)?$/i.test(a)) return 'bicameral×2';
  if (n === 3 && /^(tricameral|tri)?$/i.test(a)) return 'tricameral×3';
  return 'cameral×' + n;
}
function parseCell(cell, id) {
  const raw = String(id || cell?.id || '').replace(/\|cnn\|/g, '|single|');
  const p = raw.split('|').filter((x, i, a) => !(i === a.length - 1 && x === ''));
  const lrTok = p.find(isLRToken) || '';
  const lr = lrTok ? lrTok.replace(/^lr=/i, '') : '';
  const bmTok = p.find(x => /^bm=/i.test(x)) || '';
  const patTok = p.find(x => /^pat=/i.test(x)) || '';
  const branchModes = bmTok ? bmTok.replace(/^bm=/i, '').split('+').filter(Boolean) : [];
  const mixPat = patTok ? patTok.replace(/^pat=/i, '') : '';
  // test53: layer|dtype|mode|lr=…  — arch is always single×1 from the host bridge
  const funnyLR = !!lrTok || (p.length >= 4 && isLRToken(p[p.length - 1]));
  let arch = (cell && cell.arch) || '';
  let mode = (cell && cell.mode) || '';
  let dtype = '';
  let format = '';
  let layer = '';
  if (funnyLR) {
    layer = p[0] || '';
    dtype = p[1] || '';
    mode = mode || p[2] || '';
    if (!arch || isLRToken(arch) || !isArchToken(arch)) arch = 'single';
  } else {
    dtype = p[0] || '';
    format = p[1] || '';
    mode = mode || p[2] || '';
    if (!arch) arch = p[3] || 'single';
    if (arch === 'cnn') arch = 'single';
    // Never treat lr=… as arch even in mixed IDs
    if (isLRToken(arch)) arch = 'single';
  }
  if (branchModes.length) {
    mode = (mixPat ? mixPat + '·' : '') + branchModes.map(prettyMode).join('+');
  }
  let cams = (cell && cell.cams) || 0;
  if (!cams) cams = parseCamCount(arch);
  if (!cams) cams = 1;
  // Guard: if we only had an ID and arch looked like a number from lr=, force 1
  if ((!cell || !cell.cams) && isLRToken(p[p.length - 1])) cams = 1;
  const tag = archTagFor(cams, arch);
  return {
    id: prettyCellId(raw), arch, cams, mode, dtype, format, tag, layer, lr,
    branchModes, mixPat,
  };
}
function archBadge(cell, id) {
  const c = parseCell(cell, id);
  const cls = c.cams <= 1 ? 'arch single' : 'arch';
  return `<span class="${cls}">${c.tag}</span>`;
}
function compactSub(cell, id) {
  const c = parseCell(cell, id);
  const bits = [];
  const task = (lastLive && lastLive.task) ? String(lastLive.task) : '';
  if (task) bits.push(task);
  if (c.layer) bits.push(c.layer);
  bits.push(c.dtype);
  if (c.format && c.format !== 'none') bits.push(c.format);
  bits.push(c.tag);
  if (c.branchModes && c.branchModes.length) {
    bits.push('cams ' + c.branchModes.map(prettyMode).join('+'));
  }
  if (c.lr) bits.push('lr=' + c.lr);
  return bits.join(' · ');
}
function shortId(id, cell) {
  const c = parseCell(cell, id);
  if (!c.id) return '—';
  return `${c.mode}  ${compactSub(cell, id)}`;
}
function formatCellHTML(cell, id) {
  const c = parseCell(cell, id);
  if (!c.id) return '—';
  const fmt = c.format && c.format !== 'none' ? `<span class="fmt">${c.format}</span>` : '';
  const layer = c.layer ? `<span class="dtype" style="opacity:.85">${c.layer}</span> ` : '';
  const lr = c.lr ? `<span class="fmt">lr=${c.lr}</span>` : '';
  return `${archBadge(cell, id)}${modeChip(c.mode)}${layer}<span class="dtype">${c.dtype}</span>${fmt}${lr}`;
}
function chartLabelHTML(cell, id) {
  const c = parseCell(cell, id);
  return `${modeChip(c.mode)}<span class="sub">${compactSub(cell, id)}</span>`;
}

function boardRows() {
  if (!lastLive) return [];
  return [...(lastLive.leaderboard || [])];
}
function boardMobileRows() {
  if (!lastLive) return [];
  return [...(lastLive.leaderboard_mobile || [])];
}
function boardLearnRows() {
  if (!lastLive) return [];
  return [...(lastLive.leaderboard_learn || [])];
}
function boardLearnMobileRows() {
  if (!lastLive) return [];
  return [...(lastLive.leaderboard_learn_mobile || [])];
}

function fmtSec(sec) {
  if (!sec || sec <= 0) return '—';
  if (sec < 60) return sec.toFixed(1) + 's';
  return (sec/60).toFixed(1) + 'm';
}

function drawLearnCharts() {
  const to50 = boardLearnRows().filter(r => (r.snapshot?.time_to_acc50_sec || 0) > 0).slice(0, 12);
  if (!to50.length) {
    learnTo50Chart.innerHTML = '<div class="empty">no cell has hit 50% acc yet</div>';
  } else {
    const speeds = to50.map(r => 1 / r.snapshot.time_to_acc50_sec);
    const maxSp = Math.max(...speeds, 1e-9);
    learnTo50Chart.innerHTML = to50.map((row, i) => {
      const s = row.snapshot || {};
      const id = row.cell?.id || '';
      const pct = Math.max(6, 100 * speeds[i] / maxSp);
      const col = modeColor(parseCell(row.cell, id).mode);
      return `<div class="hbar clickable" data-cell="${escAttr(id)}" title="${escAttr(id)}">
        <div class="hbar-lab">${chartLabelHTML(row.cell, id)}</div>
        <div class="hbar-track"><div class="hbar-fill" style="width:${pct}%;background:${col}"></div></div>
        <div class="hbar-val">${fmtSec(s.time_to_acc50_sec)}<small>t→25 ${fmtSec(s.time_to_acc25_sec)}</small></div>
      </div>`;
    }).join('');
    bindRowClicks(learnTo50Chart);
  }

  const rate = boardLearnMobileRows().slice(0, 10);
  if (!rate.length) {
    learnRateChart.innerHTML = '<div class="empty">no data yet</div>';
    return;
  }
  const maxRaw = Math.max(...rate.map(r => r.snapshot?.acc_per_sec || 0), 1e-9);
  const maxMob = Math.max(...rate.map(r => r.snapshot?.mobile_acc_per_sec || 0), 1e-9);
  learnRateChart.innerHTML = rate.map(row => {
    const s = row.snapshot || {};
    const id = row.cell?.id || '';
    const raw = s.acc_per_sec || 0;
    const mob = s.mobile_acc_per_sec || 0;
    return `<div class="hbar clickable" data-cell="${escAttr(id)}" title="${escAttr(id)}" style="grid-template-columns: minmax(148px, 210px) 1fr;">
      <div class="hbar-lab">${chartLabelHTML(row.cell, id)}</div>
      <div class="hbar-stack">
        <div class="hbar-mini">
          <span>acc/s</span>
          <div class="hbar-track"><div class="hbar-fill" style="width:${Math.max(4, 100*raw/maxRaw)}%;background:#3dd6c6"></div></div>
          <span class="n">${raw.toFixed(3)}</span>
        </div>
        <div class="hbar-mini">
          <span>/MiB</span>
          <div class="hbar-track"><div class="hbar-fill" style="width:${Math.max(4, 100*mob/maxMob)}%;background:#c3a6ff"></div></div>
          <span class="n">${mob.toFixed(2)}</span>
        </div>
      </div>
    </div>`;
  }).join('');
  bindRowClicks(learnRateChart);
}

function drawRank() {
  let rows = boardRows().filter(r => r.status === 'ok' || r.status === 'running');
  const idx = lpdIndex();
  const lpOf = row => idx[prettyCellId(row.cell?.id || '')] || {};
  rows = rows.slice().sort((a, b) => {
    const la = lpOf(a), lb = lpOf(b);
    if ((lb.lpd || 0) !== (la.lpd || 0)) return (lb.lpd || 0) - (la.lpd || 0);
    if ((lb.q || 0) !== (la.q || 0)) return (lb.q || 0) - (la.q || 0);
    return (b.snapshot?.avg_accuracy || 0) - (a.snapshot?.avg_accuracy || 0);
  }).slice(0, 14);
  if (!rows.length) {
    rankChart.innerHTML = '<div class="empty">no finished models yet</div>';
    return;
  }
  const maxLpd = Math.max(...rows.map(r => lpOf(r).lpd || 0), 1e-9);
  const maxThru = Math.max(...rows.map(r => r.snapshot?.throughput || 0), 1);
  const head = `<div class="rank-head"><span>model</span><span>LPD</span><span>Q</span><span>HardAcc</span><span>thru</span><span>avail</span><span>RAM%</span></div>`;
  const body = rows.map(row => {
    const s = row.snapshot || {};
    const id = row.cell?.id || '';
    const lp = lpOf(row);
    const ram = lp.ram_frac != null ? lp.ram_frac : 1;
    const vals = [
      { n: (lp.lpd || 0) / maxLpd, t: (lp.lpd || 0).toFixed(2), c: '#b7791f' },
      { n: lp.q || 0, t: ((lp.q || 0) * 100).toFixed(0) + '%', c: '#3dd6c6' },
      { n: (s.avg_accuracy || 0) / 100, t: (s.avg_accuracy || 0).toFixed(1), c: '#e6b35a' },
      { n: (s.throughput || 0) / maxThru, t: (s.throughput || 0).toFixed(0), c: '#7aa2f7' },
      { n: (s.availability || 0) / 100, t: (s.availability || 0).toFixed(1), c: '#c3a6ff' },
      { n: Math.max(0, 1 - ram), t: (ram * 100).toFixed(0) + '%', c: '#8aa0ad' },
    ];
    const metrics = vals.map(v =>
      `<div class="rm"><div class="hbar-track"><div class="hbar-fill" style="width:${Math.max(4, 100*Math.min(1, Math.max(0, v.n)))}%;background:${v.c}"></div></div><span class="n">${v.t}</span></div>`
    ).join('');
    const band = lp.band ? ` style="color:${bandColor(lp.band)}"` : '';
    return `<div class="rank-row clickable ${row.status==='running'?'running':''}" data-cell="${escAttr(id)}" title="${escAttr(id)}"${band}>
      <div class="hbar-lab">${chartLabelHTML(row.cell, id)}</div>
      ${metrics}
    </div>`;
  }).join('');
  rankChart.innerHTML = head + body;
  bindRowClicks(rankChart);
}

function showPoint(p) {
  const cur = document.getElementById('current');
  if (!p) { cur.textContent = '—'; return; }
  const t = p.at ? new Date(p.at).toLocaleTimeString() : '';
  const sc = scoreChamp();
  const bits = METRICS.map(m => {
    const live = p[m.key] || 0;
    const mc = champForMetric(m);
    const mv = snapMetric(mc, m.snap);
    const sv = snapMetric(sc, m.snap);
    let s = `<b style="color:${m.color}">${m.key}</b> ${live.toFixed(m.digits)}`;
    if (mv != null) {
      const d = live - mv;
      s += ` <span class="pill">vs metric-champ ${d>=0?'+':''}${d.toFixed(m.digits)}</span>`;
    }
    if (sv != null && sc && (!mc || sc.cell?.id !== mc.cell?.id)) {
      const d = live - sv;
      s += ` <span class="pill">vs score-champ ${d>=0?'+':''}${d.toFixed(m.digits)}</span>`;
    }
    return s;
  }).join('<br/>');
  const champIds = [];
  if (sc?.cell?.id) champIds.push(['score-champ', sc.cell, sc.cell.id]);
  METRICS.forEach(m => {
    const mc = champForMetric(m);
    if (mc?.cell?.id && (!sc || mc.cell.id !== sc.cell.id)) {
      champIds.push([`${m.key}-champ`, mc.cell, mc.cell.id]);
    }
  });
  cur.innerHTML = `<div>${formatCellHTML(null, p.cell_id||'—')} <span class="pill">${p.phase||''}</span> <span class="pill">${p.status||''}</span> ${t}</div>
    <div style="margin-top:0.45rem; line-height:1.55">${bits}</div>
    <div style="margin-top:0.45rem; font-size:0.78rem">${champIds.map(([lab, cell, id]) =>
      `<div class="pill" style="margin:0.15rem 0.15rem 0 0">${lab}: ${formatCellHTML(cell, id)}</div>`).join('')}</div>`;
}

function setFollow(on) {
  followLive = on;
  if (liveBtn) liveBtn.classList.toggle('off', !on);
  if (on && history.length && scrub) {
    scrub.value = String(history.length - 1);
    if (scrubLabel) scrubLabel.textContent = 'live';
    showPoint(history[history.length - 1]);
    refreshPulseChart(history.length - 1);
  }
}
if (liveBtn) liveBtn.onclick = () => setFollow(true);
const cellModalClose = document.getElementById('cellModalClose');
if (cellModalClose) cellModalClose.onclick = closeCellDetail;
const cellModal = document.getElementById('cellModal');
if (cellModal) {
  cellModal.addEventListener('click', (e) => {
    if (e.target.id === 'cellModal') closeCellDetail();
  });
}
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeCellDetail();
});
if (scrub) scrub.addEventListener('input', () => {
  followLive = false;
  if (liveBtn) liveBtn.classList.add('off');
  const i = +scrub.value;
  if (scrubLabel) scrubLabel.textContent = history[i]?.at ? new Date(history[i].at).toLocaleTimeString() : String(i);
  showPoint(history[i]);
  refreshPulseChart(i);
});

function refreshScrub() {
  if (!scrub) return;
  scrub.max = String(Math.max(0, history.length - 1));
  if (followLive) {
    scrub.value = scrub.max;
    scrubLabel.textContent = 'live';
    showPoint(history[history.length - 1]);
    refreshPulseChart(history.length - 1);
  } else {
    const i = Math.min(+scrub.value, history.length - 1);
    scrub.value = String(i);
    showPoint(history[i]);
    refreshPulseChart(i);
  }
}

function fmtMs(v) {
  if (v == null || !isFinite(v)) return '—';
  if (v >= 1000) return (v/1000).toFixed(1) + 's';
  return Math.round(v) + '';
}

async function openCellDetail(rowOrId) {
  const id = typeof rowOrId === 'string' ? rowOrId : (rowOrId?.cell?.id);
  if (!id) return;
  let row = typeof rowOrId === 'object' ? rowOrId : resultById(id);
  try {
    const r = await fetch('/api/cell?id=' + encodeURIComponent(id));
    if (r.ok) row = await r.json();
  } catch (_) {}
  if (!row) return;
  const s = row.snapshot || {};
  const cellId = row.cell?.id || '—';
  document.getElementById('cellModalId').innerHTML = formatCellHTML(row.cell, cellId) +
    `<div class="sub" style="margin-top:0.35rem">${prettyCellId(cellId)}</div>`;
  const metrics = [
    ['Score', (s.score||0).toFixed(3)],
    ['SoftAcc', (s.soft_acc||0).toFixed(1) + '%'],
    ['Hard Acc', (s.avg_accuracy||0).toFixed(1) + '%'],
    ['Adapt', (s.adapt_pct||0).toFixed(1) + '%'],
    ['Avail', (s.availability||0).toFixed(1) + '%'],
    ['Stab', (s.stability||0).toFixed(1)],
    ['Cons', (s.consistency||0).toFixed(1) + '%'],
    ['Tput', (s.throughput||0).toFixed(1) + '/s'],
    ['InferMs', fmtMs(s.infer_ms)],
    ['TrainMs', fmtMs(s.train_ms)],
    ['RAM', ((s.weight_bytes||0)/1024).toFixed(1) + ' KiB'],
    ['Mobile', (s.mobile_score||0).toFixed(1) + '/MiB'],
  ];
  document.getElementById('cellModalMetrics').innerHTML = metrics.map(([k,v]) =>
    `<div class="m"><b>${k}</b>${v}</div>`).join('');

  const blocks = s.soft_acc_blocks || [];
  const phases = s.phase_blocks || [];
  const switches = s.switch_blocks || [];
  const strip = document.getElementById('cellModalStrip');
  if (!blocks.length) {
    document.getElementById('cellModalStripHint').textContent =
      'No SoftAcc 1s strip on this cell yet (needs a run after SoftAccBlocks was added).';
    strip.innerHTML = '';
  } else {
    const switchSecs = [];
    switches.forEach((sw, i) => { if (sw) switchSecs.push(((i+1)).toFixed(0) + 's'); });
    document.getElementById('cellModalStripHint').textContent =
      `SoftAcc % (1s blocks)${switchSecs.length ? ' — switches near ' + switchSecs.join(' / ') : ''} · click outside or close`;
    // chunk into rows of 20 for readability on long epochs
    const chunk = 20;
    let html = '';
    for (let start = 0; start < blocks.length; start += chunk) {
      const end = Math.min(start + chunk, blocks.length);
      let head = '<tr><th>t</th>';
      let soft = '<tr><th>SoftAcc</th>';
      let phase = '<tr><th>Phase</th>';
      for (let i = start; i < end; i++) {
        const cls = switches[i] ? ' class="sw"' : '';
        head += `<th${cls}>${i+1}s</th>`;
        soft += `<td${cls}>${Math.round(blocks[i]||0)}%</td>`;
        phase += `<td${cls}>${phases[i]||'—'}</td>`;
      }
      head += '</tr>'; soft += '</tr>'; phase += '</tr>';
      html += head + soft + phase;
    }
    const avg = blocks.reduce((a,b)=>a+(b||0),0) / blocks.length;
    html += `<tr><th>Avg</th><td colspan="${Math.min(chunk, blocks.length)}">${avg.toFixed(1)}% · Score ${(s.score||0).toFixed(1)}</td></tr>`;
    strip.innerHTML = html;
  }
  document.getElementById('cellModal').classList.add('open');
}

function closeCellDetail() {
  document.getElementById('cellModal').classList.remove('open');
}

function lpdIndex() {
  const l = (lastLive && lastLive.lpd) || {};
  const m = {};
  (l.top||[]).concat(l.gold||[], l.near||[], l.trap||[]).forEach(r => {
    if (r.id) m[prettyCellId(r.id)] = r;
  });
  return m;
}
function lpdBandOf(id) {
  return lpdIndex()[prettyCellId(id||'')] || null;
}

function rowHTML(row, i, mobile) {
  const s = row.snapshot || {};
  const id = row.cell?.id || '';
  const esc = id.replace(/\\/g,'\\\\').replace(/'/g, "\\'");
  if (mobile) {
    const lp = lpdBandOf(id);
    const band = lp ? (lp.band||'—') : '—';
    const q = lp ? ((lp.q||0)*100).toFixed(0) : '—';
    return `<tr class="clickable" data-cell="${esc}" title="click for SoftAcc 1s strip" style="color:${lp ? bandColor(band) : ''}">
      <td>${i+1}</td><td class="cellid">${formatCellHTML(row.cell, id)}</td>
      <td>${band}</td>
      <td class="num">${q}</td>
      <td>${(s.mobile_score||0).toFixed(2)}</td>
      <td>${(s.score||0).toFixed(3)}</td>
      <td>${(s.soft_acc||0).toFixed(1)}</td>
      <td>${(s.avg_accuracy||0).toFixed(1)}</td>
      <td>${(s.adapt_pct||0).toFixed(1)}</td>
      <td>${(s.availability||0).toFixed(1)}</td>
      <td>${(s.throughput||0).toFixed(1)}</td>
      <td>${fmtMs(s.infer_ms)}</td>
      <td>${fmtMs(s.train_ms)}</td>
      <td>${((s.weight_bytes||0)/1024).toFixed(1)}K</td>
      <td class="${row.status}">${row.status}</td>
    </tr>`;
  }
  return `<tr class="clickable" data-cell="${esc}" title="click for SoftAcc 1s strip">
    <td>${i+1}</td><td class="cellid">${formatCellHTML(row.cell, id)}</td>
    <td>${(s.score||0).toFixed(3)}</td>
    <td>${(s.soft_acc||0).toFixed(1)}</td>
    <td>${(s.avg_accuracy||0).toFixed(1)}</td>
    <td>${(s.adapt_pct||0).toFixed(1)}</td>
    <td>${(s.availability||0).toFixed(1)}</td>
    <td>${(s.stability||0).toFixed(1)}</td>
    <td>${(s.consistency||0).toFixed(1)}</td>
    <td>${(s.throughput||0).toFixed(1)}</td>
    <td>${fmtMs(s.infer_ms)}</td>
    <td>${fmtMs(s.train_ms)}</td>
    <td>${((s.weight_bytes||0)/1024).toFixed(1)}K</td>
    <td class="${row.status}">${row.status}</td>
  </tr>`;
}

function learnRowHTML(row, i, mobile) {
  const s = row.snapshot || {};
  const id = row.cell?.id || '';
  const dur = (s.duration || 0) / 1e9; // Go json Duration = ns
  if (mobile) {
    const lp = lpdBandOf(id);
    const band = lp ? (lp.band||'—') : '—';
    const q = lp ? ((lp.q||0)*100).toFixed(0) : '—';
    return `<tr style="color:${lp ? bandColor(band) : ''}">
      <td>${i+1}</td><td class="cellid">${formatCellHTML(row.cell, id)}</td>
      <td>${band}</td>
      <td class="num">${q}</td>
      <td>${(s.mobile_acc_per_sec||0).toFixed(2)}</td>
      <td>${(s.acc_per_sec||0).toFixed(3)}</td>
      <td>${fmtSec(s.time_to_acc50_sec)}</td>
      <td>${((s.weight_bytes||0)/1024).toFixed(1)}K</td>
      <td>${(s.avg_accuracy||0).toFixed(1)}</td>
      <td class="${row.status}">${row.status}</td>
    </tr>`;
  }
  return `<tr>
    <td>${i+1}</td><td class="cellid">${formatCellHTML(row.cell, id)}</td>
    <td>${fmtSec(s.time_to_acc25_sec)}</td>
    <td>${fmtSec(s.time_to_acc50_sec)}</td>
    <td>${(s.acc_per_sec||0).toFixed(3)}</td>
    <td>${(s.avg_accuracy||0).toFixed(1)}</td>
    <td>${fmtSec(dur)}</td>
    <td class="${row.status}">${row.status}</td>
  </tr>`;
}

function renderBoards() {
  const raw = boardRows().filter(r => !filterRaw || (r.cell?.id||'').toLowerCase().includes(filterRaw));
  const mob = boardMobileRows().filter(r => !filterMobile || (r.cell?.id||'').toLowerCase().includes(filterMobile));
  const learn = boardLearnRows().filter(r => !filterLearn || (r.cell?.id||'').toLowerCase().includes(filterLearn));
  const learnM = boardLearnMobileRows().filter(r => !filterLearnMobile || (r.cell?.id||'').toLowerCase().includes(filterLearnMobile));
  document.getElementById('boardCount').textContent = `${raw.length} shown · ${boardRows().length} total`;
  document.getElementById('boardMobileCount').textContent = `${mob.length} shown · ${boardMobileRows().length} total`;
  document.getElementById('boardLearnCount').textContent = `${learn.length} shown · ${boardLearnRows().length} total`;
  document.getElementById('boardLearnMobileCount').textContent = `${learnM.length} shown · ${boardLearnMobileRows().length} total`;
  document.getElementById('board').innerHTML = raw.map((r,i)=>rowHTML(r,i,false)).join('');
  document.getElementById('boardMobile').innerHTML = mob.map((r,i)=>rowHTML(r,i,true)).join('');
  document.getElementById('boardLearn').innerHTML = learn.map((r,i)=>learnRowHTML(r,i,false)).join('');
  document.getElementById('boardLearnMobile').innerHTML = learnM.map((r,i)=>learnRowHTML(r,i,true)).join('');
  const byId = {};
  [...boardRows(), ...boardMobileRows()].forEach(r => { if (r.cell?.id) byId[r.cell.id] = r; });
  document.querySelectorAll('#board tr.clickable, #boardMobile tr.clickable').forEach(tr => {
    tr.onclick = () => openCellDetail(tr.getAttribute('data-cell'));
  });
}

async function loadHistory() {
  const r = await fetch('/api/history');
  const j = await r.json();
  history = Array.isArray(j.history) ? j.history : [];
  seeded = true;
  refreshScrub();
}

async function syncHistoryTip(serverLen, tip) {
  if (!seeded) { await loadHistory(); return; }
  if (serverLen < history.length) { await loadHistory(); return; }
  if (serverLen > history.length) {
    const r = await fetch('/api/history?from=' + history.length);
    const j = await r.json();
    if (Array.isArray(j.history) && j.history.length) history = history.concat(j.history);
  } else if (tip?.length && history.length) {
    history[history.length - 1] = tip[tip.length - 1];
  }
  refreshScrub();
}

function applyTaskMeta(j) {
  const task = (j && j.task) ? String(j.task).trim() : '';
  const sub = (j && j.subtitle) ? String(j.subtitle).trim() : '';
  const lr = (j && j.lr != null && +j.lr > 0) ? String(+j.lr) : '';
  const titleEl = document.getElementById('taskTitle');
  const subEl = document.getElementById('taskSub');
  let title = task ? (task + ' · live adaptation') : 'live adaptation';
  if (lr) title += ' · lr ' + lr;
  if (titleEl) titleEl.textContent = title;
  document.title = task ? ('tide — ' + task + (lr ? (' · lr ' + lr) : '')) : 'tide — live adaptation';
  if (subEl) {
    if (sub && lr) subEl.textContent = sub + ' · lr ' + lr;
    else if (sub) subEl.textContent = sub;
    else if (lr) subEl.textContent = 'lr ' + lr;
  }
}

function heatRGB(t) {
  if (t < 0.5) {
    const u = t * 2;
    return [Math.round(197 + (231-197)*u), Math.round(48 + (179-48)*u), Math.round(48 + (90-48)*u)];
  }
  const u = (t - 0.5) * 2;
  return [Math.round(231 + (45-231)*u), Math.round(179 + (166-179)*u), Math.round(90 + (150-90)*u)];
}
function clipHead(s, n) {
  s = String(s || '');
  if (s.length <= n) return s;
  return s.slice(0, Math.max(1, n - 1)) + '~';
}
function heatTable(elId, rows, cols, grid, signed) {
  const el = document.getElementById(elId);
  if (!el) return;
  if (!rows || !cols || !grid || !rows.length || !cols.length) {
    el.innerHTML = '<p class="hint">no finished cells yet</p>';
    return;
  }
  let min = Infinity, max = -Infinity, maxAbs = 0;
  rows.forEach((_, i) => cols.forEach((_, j) => {
    const v = grid[i] && grid[i][j];
    if (v == null || v === '') return;
    const n = +v;
    if (!isFinite(n)) return;
    if (n < min) min = n;
    if (n > max) max = n;
    if (Math.abs(n) > maxAbs) maxAbs = Math.abs(n);
  }));
  if (!isFinite(min)) { min = 0; max = 1; }
  if (max <= min) max = min + 1;
  if (maxAbs < 1e-9) maxAbs = 1;
  let html = '<table class="heat"><thead><tr><th></th>' + cols.map(c => `<th title="${c}">${prettyMode(c)}</th>`).join('') + '</tr></thead><tbody>';
  rows.forEach((r, i) => {
    html += `<tr><th title="${r}">${prettyMode(r)}</th>`;
    cols.forEach((_, j) => {
      const raw = grid[i] && grid[i][j];
      if (raw == null || raw === '') { html += '<td></td>'; return; }
      const v = +raw;
      if (!signed && !v) { html += '<td></td>'; return; }
      const t = signed ? 0.5 + 0.5 * (v / maxAbs) : (v - min) / (max - min);
      const [cr,cg,cb] = heatRGB(t);
      const ink = (cr*3+cg*5+cb*2) > 1400 ? '#1a2024' : '#f4f7f8';
      const lab = signed ? ((v>=0?'+':'') + v.toFixed(1)) : v.toFixed(0);
      html += `<td style="background:rgb(${cr},${cg},${cb});color:${ink}">${lab}</td>`;
    });
    html += '</tr>';
  });
  html += '</tbody></table>';
  el.innerHTML = html;
}
function pivotBins(bins, valueKey) {
  const rows = [], cols = [], seenR = {}, seenC = {};
  (bins || []).forEach(b => {
    if (!b || !b.n || !b.mode || !b.key) return;
    if (!seenR[b.mode]) { seenR[b.mode] = 1; rows.push(b.mode); }
    if (!seenC[b.key]) { seenC[b.key] = 1; cols.push(b.key); }
  });
  rows.sort(); cols.sort();
  const grid = rows.map(() => cols.map(() => null));
  const ri = {}, ci = {};
  rows.forEach((r, i) => { ri[r] = i; });
  cols.forEach((c, i) => { ci[c] = i; });
  (bins || []).forEach(b => {
    if (!b || !b.n) return;
    if (ri[b.mode] == null || ci[b.key] == null) return;
    grid[ri[b.mode]][ci[b.key]] = b[valueKey];
  });
  return { rows, cols, grid };
}
function drawSignedBars(canvasId, rows, key) {
  const c = document.getElementById(canvasId);
  if (!c || !c.getContext) return;
  fitCanvas(c);
  const ctx = c.getContext('2d');
  const w = c.width, h = c.height;
  ctx.clearRect(0, 0, w, h);
  if (!rows || !rows.length) return;
  const vals = rows.map(r => +r[key] || 0);
  let maxAbs = Math.max(...vals.map(v => Math.abs(v)), 1);
  const padL = 110, padR = 48, padT = 8, padB = 8;
  const barH = Math.max(8, Math.min(22, (h - padT - padB) / rows.length - 3));
  const mid = padL + (w - padL - padR) / 2;
  ctx.strokeStyle = '#8aa0ad';
  ctx.beginPath(); ctx.moveTo(mid, padT); ctx.lineTo(mid, h - padB); ctx.stroke();
  rows.forEach((r, i) => {
    const y = padT + i * ((h - padT - padB) / rows.length);
    const v = vals[i];
    const bw = ((w - padL - padR) / 2) * (Math.abs(v) / maxAbs);
    ctx.fillStyle = v >= 0 ? '#2da696' : '#c53030';
    if (v >= 0) ctx.fillRect(mid, y + 2, bw, barH);
    else ctx.fillRect(mid - bw, y + 2, bw, barH);
    ctx.fillStyle = '#c5d0d6';
    ctx.font = '11px sans-serif';
    ctx.textAlign = 'right';
    ctx.fillText(prettyMode(r.mode || ''), padL - 8, y + barH);
    ctx.textAlign = 'left';
    ctx.fillText((v >= 0 ? '+' : '') + v.toFixed(1), mid + (v >= 0 ? bw + 6 : 6), y + barH);
  });
}
function renderVs(heat) {
  const sec = document.getElementById('vsSection');
  const vs = heat && heat.vs;
  if (!sec) return;
  if (!vs || !vs.baseline || !(vs.modes || []).length) {
    sec.style.display = 'none';
    return;
  }
  sec.style.display = '';
  const title = document.getElementById('vsTitle');
  const hint = document.getElementById('vsHint');
  if (title) title.textContent = 'vs ' + prettyMode(vs.baseline) + ' (matched dtype × format × arch)';
  if (hint) hint.textContent = 'Δ columns are Hard Acc / SoftAcc / Avail / Thru / Score vs the baseline mode on the same recipe — not Acc-keep%. Positive = better than baseline. Acc win% = share of matched cells with Hard AccΔ > 0.5. Green bars = wins; red = losses. Use Acc keep % / Lean in Lucy density above for footprint vs Acc-champ.';
  const wrap = document.getElementById('vsTableWrap');
  if (wrap) {
    let html = '<table><thead><tr><th>mode</th><th class="num">n</th><th class="num">Hard AccΔ</th><th class="num">Acc win%</th><th class="num">SoftΔ</th><th class="num">AvailΔ</th><th class="num">ThruΔ</th><th class="num">ScoreΔ</th><th class="num">Score win%</th></tr></thead><tbody>';
    vs.modes.forEach(m => {
      const acc = m.acc_delta||0, thr = m.thru_delta||0, av = m.avail_delta||0, sc = m.score_delta||0;
      const tone = (v) => v > 0.5 ? 'color:#2da696' : (v < -0.5 ? 'color:#c53030' : '');
      html += `<tr>
        <td>${prettyMode(m.mode||'')}</td>
        <td class="num">${m.n||0}</td>
        <td class="num" style="${tone(acc)}">${acc>=0?'+':''}${acc.toFixed(1)}</td>
        <td class="num">${(m.acc_win||0).toFixed(0)}</td>
        <td class="num">${(m.soft_delta||0)>=0?'+':''}${(m.soft_delta||0).toFixed(1)}</td>
        <td class="num" style="${tone(av)}">${av>=0?'+':''}${av.toFixed(1)}</td>
        <td class="num" style="${tone(thr)}">${thr>=0?'+':''}${thr.toFixed(0)}</td>
        <td class="num" style="${tone(sc)}">${sc>=0?'+':''}${sc.toFixed(1)}</td>
        <td class="num">${(m.score_win||0).toFixed(0)}</td>
      </tr>`;
    });
    wrap.innerHTML = html + '</tbody></table>';
  }
  const stack = document.getElementById('vsStack');
  if (!stack) return;
  const heats = [
    ['Hard AccΔ vs ' + vs.baseline + ' — mode × dtype (green=win)', vs.by_dtype, 'acc'],
    ['ScoreΔ vs ' + vs.baseline + ' — mode × dtype', vs.by_dtype, 'score'],
    ['AvailΔ vs ' + vs.baseline + ' — mode × dtype', vs.by_dtype, 'avail'],
    ['Hard AccΔ vs ' + vs.baseline + ' — mode × arch / cam', vs.by_arch, 'acc'],
  ];
  if ((vs.by_layer || []).length) {
    heats.push(['Hard AccΔ vs ' + vs.baseline + ' — mode × layer', vs.by_layer, 'acc']);
  }
  let html = `
    <div class="chart-panel"><h3>mean Hard AccΔ vs ${prettyMode(vs.baseline)} (pp)</h3><canvas id="vsBarAcc" width="960" height="${Math.max(160, vs.modes.length*28)}"></canvas></div>
    <div class="chart-panel"><h3>mean ThruΔ vs ${prettyMode(vs.baseline)}</h3><canvas id="vsBarThru" width="960" height="${Math.max(160, vs.modes.length*28)}"></canvas></div>
    <div class="chart-panel"><h3>mean ScoreΔ vs ${prettyMode(vs.baseline)}</h3><canvas id="vsBarScore" width="960" height="${Math.max(160, vs.modes.length*28)}"></canvas></div>
    <div class="chart-panel"><h3>mean AvailΔ vs ${prettyMode(vs.baseline)}</h3><canvas id="vsBarAvail" width="960" height="${Math.max(160, vs.modes.length*28)}"></canvas></div>`;
  heats.forEach((h, i) => {
    html += `<div class="chart-panel"><h3>${h[0]}</h3><div class="table-wrap" id="vsHeat${i}"></div></div>`;
  });
  if ((vs.families || []).length) {
    html += `<div class="chart-panel"><h3>Family collapse — Step* vs non-Step</h3><div class="table-wrap" id="vsFam"></div></div>`;
  }
  stack.innerHTML = html;
  drawSignedBars('vsBarAcc', vs.modes, 'acc_delta');
  drawSignedBars('vsBarThru', vs.modes, 'thru_delta');
  drawSignedBars('vsBarScore', vs.modes, 'score_delta');
  drawSignedBars('vsBarAvail', vs.modes, 'avail_delta');
  heats.forEach((h, i) => {
    const p = pivotBins(h[1], h[2]);
    heatTable('vsHeat' + i, p.rows, p.cols, p.grid, true);
  });
  const famEl = document.getElementById('vsFam');
  if (famEl) {
    famEl.innerHTML = '<table><thead><tr><th>step</th><th>plain</th><th class="num">n</th><th class="num">mean |Hard AccΔ|</th></tr></thead><tbody>' +
      vs.families.map(f => `<tr><td>${f.step||''}</td><td>${f.plain||''}</td><td class="num">${f.n||0}</td><td class="num">${(f.mean_abs_acc||0).toFixed(2)}</td></tr>`).join('') +
      '</tbody></table>';
  }
}
function okPoints() {
  const done = (lastLive && lastLive.completed || []).filter(r => r.status === 'ok');
  if (done.length) {
    return done.map(r => ({
      mode: r.cell && r.cell.mode,
      dtype: r.cell && r.cell.dtype,
      arch: parseCell(r.cell, r.cell && r.cell.id).tag,
      score: r.snapshot && r.snapshot.score,
      soft_acc: r.snapshot && r.snapshot.soft_acc,
      avg_accuracy: r.snapshot && r.snapshot.avg_accuracy,
      availability: r.snapshot && r.snapshot.availability,
      throughput: r.snapshot && r.snapshot.throughput,
      adapt_pct: r.snapshot && r.snapshot.adapt_pct,
    }));
  }
  return (lastLive && lastLive.heat && lastLive.heat.points) || [];
}
function drawScatter(canvasId, pts, xk, yk) {
  const c = document.getElementById(canvasId);
  if (!c || !c.getContext) return;
  fitCanvas(c);
  const ctx = c.getContext('2d');
  const w = c.width, h = c.height;
  ctx.clearRect(0, 0, w, h);
  if (!pts.length) {
    ctx.fillStyle = '#8aa0ad';
    ctx.font = '13px sans-serif';
    ctx.fillText('no finished cells yet', 20, 30);
    return;
  }
  const xs = pts.map(p => +p[xk] || 0), ys = pts.map(p => +p[yk] || 0);
  let xmin = Math.min(...xs), xmax = Math.max(...xs), ymin = Math.min(...ys), ymax = Math.max(...ys);
  if (xmax <= xmin) xmax = xmin + 1;
  if (ymax <= ymin) ymax = ymin + 1;
  const padL = 48, padR = 16, padT = 16, padB = 32;
  const X = v => padL + (w - padL - padR) * (v - xmin) / (xmax - xmin);
  const Y = v => h - padB - (h - padT - padB) * (v - ymin) / (ymax - ymin);
  ctx.strokeStyle = '#1d3342';
  ctx.strokeRect(padL, padT, w - padL - padR, h - padT - padB);
  pts.forEach((p, i) => {
    const arch = String(p.arch || '');
    ctx.fillStyle = /tri/i.test(arch) ? '#a882dc' : /bi/i.test(arch) ? '#4ea3ff' : '#3dd6c6';
    ctx.fillRect(X(xs[i])-3, Y(ys[i])-3, 6, 6);
  });
  const front = paretoJS(pts, xk, yk);
  ctx.strokeStyle = '#b7791f';
  ctx.lineWidth = 1.6;
  ctx.beginPath();
  front.forEach((p, i) => {
    const x = X(+p[xk]||0), y = Y(+p[yk]||0);
    if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
  });
  ctx.stroke();
  strokeAxisLabels(ctx, w, h, padL, padB, xk, yk);
}
function paretoJS(pts, xk, yk) {
  const all = pts.slice().sort((a,b) => (+a[xk]||0) - (+b[xk]||0));
  const out = [];
  let best = -Infinity;
  all.forEach(p => {
    const y = +p[yk] || 0;
    if (y >= best) { best = y; out.push(p); }
  });
  return out;
}
function bandColor(b) {
  if (b === 'gold') return '#b7791f';
  if (b === 'near') return '#e6b35a';
  if (b === 'trap') return '#e06c75';
  if (b === 'keep') return '#3dd6c6';
  if (b === 'acc') return '#7aa2f7';
  return '#8aa0ad';
}
function lpdMerged(l) {
  const seen = new Set();
  const out = [];
  (l.gold||[]).concat(l.near||[], l.trap||[], l.top||[]).forEach(r => {
    const k = (r.tide||'') + '|' + (r.id||'');
    if (seen.has(k)) return;
    seen.add(k);
    out.push(r);
  });
  return out;
}
function lpdRowById(l, id) {
  if (!id) return null;
  return lpdMerged(l).find(r => r.id === id) || (l.top||[]).find(r => r.id === id) || null;
}
function drawRadar(canvasId, series) {
  const c = document.getElementById(canvasId);
  if (!c || !c.getContext) return;
  fitCanvas(c);
  const ctx = c.getContext('2d');
  const w = c.width, h = c.height;
  ctx.clearRect(0, 0, w, h);
  const cx = Math.min(w * 0.42, h * 0.5), cy = h * 0.52, radius = Math.min(cx, cy) - 36;
  const labels = ['Acc', 'Thru', 'Avail'];
  const n = 3;
  const ang = i => -Math.PI/2 + i * 2 * Math.PI / n;
  ctx.strokeStyle = '#1d3342';
  for (let ring = 1; ring <= 4; ring++) {
    ctx.beginPath();
    for (let i = 0; i < n; i++) {
      const r = radius * ring / 4;
      const x = cx + r * Math.cos(ang(i)), y = cy + r * Math.sin(ang(i));
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    }
    ctx.closePath();
    ctx.stroke();
  }
  ctx.fillStyle = '#8aa0ad';
  ctx.font = '14px sans-serif';
  ctx.textAlign = 'center';
  labels.forEach((lab, i) => {
    const x = cx + (radius + 22) * Math.cos(ang(i));
    const y = cy + (radius + 22) * Math.sin(ang(i));
    ctx.fillText(lab, x, y + 4);
    ctx.beginPath();
    ctx.moveTo(cx, cy);
    ctx.lineTo(cx + radius * Math.cos(ang(i)), cy + radius * Math.sin(ang(i)));
    ctx.stroke();
  });
  (series || []).forEach(s => {
    const vals = s.vals || [0, 0, 0];
    ctx.strokeStyle = s.color;
    ctx.lineWidth = 2.4;
    ctx.beginPath();
    vals.forEach((v, i) => {
      const t = Math.max(0, Math.min(1, +v || 0));
      const x = cx + radius * t * Math.cos(ang(i));
      const y = cy + radius * t * Math.sin(ang(i));
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    });
    ctx.closePath();
    ctx.stroke();
    ctx.lineWidth = 1;
  });
  ctx.font = '13px sans-serif';
  ctx.textAlign = 'left';
  const lx = Math.min(w * 0.68, cx + radius + 36);
  (series || []).forEach((s, i) => {
    ctx.fillStyle = s.color;
    ctx.fillRect(lx, 22 + i * 22, 12, 12);
    ctx.fillStyle = '#c8d6dc';
    ctx.fillText(s.label || '', lx + 18, 33 + i * 22);
  });
}
function drawLPDScatter(canvasId, rows, xk, yk, vline, hline, goldSide) {
  const c = document.getElementById(canvasId);
  if (!c || !c.getContext) return;
  fitCanvas(c);
  const ctx = c.getContext('2d');
  const w = c.width, h = c.height;
  ctx.clearRect(0, 0, w, h);
  if (!rows || !rows.length) {
    ctx.fillStyle = '#8aa0ad';
    ctx.font = '13px sans-serif';
    ctx.fillText('no finished cells yet', 20, 30);
    return;
  }
  const xs = rows.map(p => +p[xk] || 0), ys = rows.map(p => +p[yk] || 0);
  let xmin = Math.min(...xs), xmax = Math.max(...xs), ymin = Math.min(...ys), ymax = Math.max(...ys);
  if (xmax <= xmin) xmax = xmin + 1;
  if (ymax <= ymin) ymax = ymin + 1;
  if (ymin > 0) ymin = 0;
  if (ymax < 100 && (yk === 'qpct' || yk === 'accpct')) ymax = 100;
  const padL = 48, padR = 16, padT = 16, padB = 32;
  const X = v => padL + (w - padL - padR) * (v - xmin) / (xmax - xmin);
  const Y = v => h - padB - (h - padT - padB) * (v - ymin) / (ymax - ymin);
  ctx.strokeStyle = '#1d3342';
  ctx.strokeRect(padL, padT, w - padL - padR, h - padT - padB);
  if (hline != null && vline != null) {
    ctx.fillStyle = 'rgba(183,121,31,0.12)';
    const y1 = padT, y2 = Math.min(h-padB, Math.max(padT, Y(hline)));
    if (goldSide === 'left') {
      const x2 = Math.min(w-padR, Math.max(padL, X(vline)));
      ctx.fillRect(padL, y1, x2 - padL, y2 - y1);
    } else if (goldSide === 'right') {
      const x1 = Math.min(w-padR, Math.max(padL, X(vline)));
      ctx.fillRect(x1, y1, w-padR - x1, y2 - y1);
    }
  }
  if (vline != null && vline >= xmin && vline <= xmax) {
    ctx.strokeStyle = 'rgba(183,121,31,0.7)';
    ctx.setLineDash([4, 3]);
    ctx.beginPath(); ctx.moveTo(X(vline), padT); ctx.lineTo(X(vline), h-padB); ctx.stroke();
    ctx.setLineDash([]);
  }
  if (hline != null && hline >= ymin && hline <= ymax) {
    ctx.strokeStyle = 'rgba(183,121,31,0.7)';
    ctx.setLineDash([4, 3]);
    ctx.beginPath(); ctx.moveTo(padL, Y(hline)); ctx.lineTo(w-padR, Y(hline)); ctx.stroke();
    ctx.setLineDash([]);
  }
  rows.forEach((p, i) => {
    ctx.fillStyle = bandColor(p.band);
    const s = p.gold ? 8 : 6;
    ctx.fillRect(X(xs[i])-s/2, Y(ys[i])-s/2, s, s);
  });
  strokeAxisLabels(ctx, w, h, padL, padB, xk, yk);
}
function drawLPD() {
  const l = (lastLive && lastLive.lpd) || {};
  const champEl = document.getElementById('lpdChamp');
  const leanEl = document.getElementById('lpdLean');
  const goldEl = document.getElementById('lpdGold');
  const stdEl = document.getElementById('lpdStd');
  const tb = document.getElementById('lpdTable');
  const modesEl = document.getElementById('lpdModes');
  const leanArchEl = document.getElementById('lpdLeanArch');
  const gold = l.gold || [];
  const near = l.near || [];
  const trap = l.trap || [];
  const lean = l.lean || [];
  if (champEl) {
    const c = l.champ || {};
    const ac = l.acc_champ || {};
    const lc = l.live_champ || {};
    champEl.innerHTML = ac.id || c.id
      ? `<span class="pill">Acc champ (Hard Acc)</span> ${prettyCellId(ac.id||'—')} · Hard Acc ${(ac.avg_accuracy||0).toFixed(1)}% · Thru ${(ac.throughput||0).toFixed(0)} · ${(ac.ram_kib||0).toFixed(1)} KiB
         · <span class="pill">lucy score</span> ${prettyCellId(c.id||'—')} · Score ${(c.score||0).toFixed(1)} · Hard Acc ${(c.avg_accuracy||0).toFixed(1)}% · Soft ${(c.soft_acc||0).toFixed(1)}
         · <span class="pill">live-fit</span> ${prettyCellId(lc.id||'—')}
         · <span class="pill">gold ${gold.length}</span> <span class="pill">near ${near.length}</span> <span class="pill">lean ${lean.length}</span> <span class="pill">trap ${trap.length}</span>
         <div style="margin-top:0.35rem">board fastest ${prettyCellId(l.fast_id||'—')} · ${(l.fast_thru||0).toFixed(0)}/s
         · learner Thru peak ${(l.peak_thru||0).toFixed(0)} · learner Avail peak ${(l.peak_avail||0).toFixed(1)}%</div>`
      : 'Need finished ok cells.';
  }
  if (leanEl) {
    const lc = l.lean_champ || {};
    if (lc.id) {
      leanEl.className = 'lpd-callout';
      leanEl.innerHTML = `<b>Lean ≥95% Acc keep</b> — smallest RAM among cells with Hard Acc ≥95% of Acc champ:
        ${prettyCellId(lc.id)} · Hard Acc ${(lc.avg_accuracy||0).toFixed(1)}% · Acc keep ${((lc.rel_acc||0)*100).toFixed(0)}% ·
        ${(lc.throughput||0).toFixed(0)}/s · Avail ${(lc.availability||0).toFixed(1)}% · ${(lc.ram_kib||0).toFixed(1)} KiB
        · <span class="pill">${lean.length} in band</span>`;
    } else {
      leanEl.className = 'lpd-callout empty';
      leanEl.textContent = 'No lean cell yet. Need Acc keep ≥95% of Acc champ, then pick smallest RAM / fastest Thru.';
    }
  }
  if (goldEl) {
    if (gold.length) {
      goldEl.className = 'lpd-callout';
      goldEl.innerHTML = '<b>Gold</b> — ' + gold.slice(0, 8).map(r =>
        `${prettyCellId(r.id)} · Q ${((r.q||0)*100).toFixed(0)}% · Acc keep ${((r.rel_acc||0)*100).toFixed(0)}% · ${((r.ram_frac||0)*100).toFixed(0)}% Acc-champ RAM · ${ (r.shrink||0).toFixed(1)}× smaller`
      ).join('<br>');
    } else {
      goldEl.className = 'lpd-callout empty';
      goldEl.textContent = 'No gold cell yet. Need all three pillars at ≥80% of learner peaks in ≤20% of Acc-champ RAM.';
    }
  }
  if (stdEl) {
    const s = l.gold_std || {};
    stdEl.innerHTML = s.id
      ? `<span class="pill">gold-std</span> Acc keep ≥80% plus Thru or Avail ≥80%, then smallest+fastest: ${prettyCellId(s.id)} · mode <b>${prettyMode(s.mode||'—')}</b> · Hard Acc ${(s.avg_accuracy||0).toFixed(1)}% · Acc keep ${((s.rel_acc||0)*100).toFixed(0)}% · ${(s.throughput||0).toFixed(0)}/s · ${(s.ram_kib||0).toFixed(1)} KiB`
      : 'No gold-std yet — need Acc keep ≥80% of Acc champ plus at least one other pillar.';
  }
  if (modesEl) {
    const modes = l.gold_modes || [];
    modesEl.innerHTML = modes.length ? modes.map(m => {
      let cell = m.smallest || '';
      if (m.fastest && m.fastest !== m.smallest) cell += ' / ' + m.fastest;
      return `<tr>
      <td>${prettyMode(m.mode||'—')}</td>
      <td class="num">${m.n||0}</td>
      <td class="num">${(m.best_acc||0).toFixed(1)}</td>
      <td class="num">${(m.min_ram_kib||0).toFixed(1)}</td>
      <td class="num">${(m.max_thru||0).toFixed(0)}</td>
      <td class="cellid">${formatCellHTML(null, cell)}</td>
    </tr>`;
    }).join('') : '<tr><td colspan="6">no 2+ pillar Acc-keep cells yet</td></tr>';
  }
  if (leanArchEl) {
    const arches = l.lean_by_arch || [];
    leanArchEl.innerHTML = arches.length ? arches.map(m => {
      let cell = m.smallest || '';
      if (m.fastest && m.fastest !== m.smallest) cell += ' / ' + m.fastest;
      return `<tr>
      <td>${(m.mode||'—').replace(/^cnn$/i,'single')}</td>
      <td class="num">${m.n||0}</td>
      <td class="num">${(m.best_acc||0).toFixed(1)}</td>
      <td class="num">${(m.min_ram_kib||0).toFixed(1)}</td>
      <td class="num">${(m.max_thru||0).toFixed(0)}</td>
      <td class="cellid">${formatCellHTML(null, cell)}</td>
    </tr>`;
    }).join('') : '<tr><td colspan="6">no lean (≥95% Acc keep) cells yet</td></tr>';
  }
  const rows = l.top || [];
  if (tb) {
    tb.innerHTML = rows.length ? rows.map(r => `<tr class="clickable" data-cell="${escAttr(r.id||'')}" style="color:${bandColor(r.band)}">
      <td>${r.band||'—'}</td>
      <td class="cellid">${formatCellHTML(null, r.id)}</td>
      <td class="num">${(r.avg_accuracy||0).toFixed(1)}</td>
      <td class="num">${((r.rel_acc||0)*100).toFixed(0)}</td>
      <td class="num">${((r.rel_thru||0)*100).toFixed(0)}</td>
      <td class="num">${((r.rel_avail||0)*100).toFixed(0)}</td>
      <td class="num">${((r.q||0)*100).toFixed(0)}</td>
      <td class="num">${((r.ram_frac||0)*100).toFixed(0)}</td>
      <td class="num">${(r.lpd||0).toFixed(2)}</td>
      <td class="num">${(r.ram_kib||0).toFixed(1)}</td>
    </tr>`).join('') : '<tr><td colspan="10">no finished ok cells yet</td></tr>';
    bindRowClicks(tb);
  }
}
function renderLucyExtras() {
  const h = (lastLive && lastLive.heat) || {};
  renderVs(h);
}

async function tick() {
  try {
    const r = await fetch('/api/live', {
      headers: {
        'Accept-Encoding': 'gzip',
        ...(liveETag ? { 'If-None-Match': liveETag } : {}),
      },
    });
    if (r.status === 304) return;
    if (!r.ok) throw new Error('live unavailable');
    liveETag = r.headers.get('ETag') || liveETag;
    const j = await r.json();
    lastLive = j;
    applyTaskMeta(j);
    const awaiting = !!j.awaiting_start && !j.started && !(runningN(j) > 0);
    const banner = document.getElementById('startBanner');
    if (banner) banner.classList.toggle('show', awaiting);
    const doneN = j.epoch_done != null ? j.epoch_done : 0;
    const totalN = j.cell_total || 0;
    const epochN = j.epoch || 1;
    const epochMax = j.epoch_max || 0;
    const epochsLeft = j.epochs_left != null ? j.epochs_left
      : (epochMax > 0 ? Math.max(0, epochMax - epochN + 1) : 0);
    const epochLabel = epochMax > 0 ? `epoch ${epochN}/${epochMax}` : `epoch ${epochN}`;
    const canResume = doneN > 0;
    const startBtnLive = document.getElementById('startBtn');
    const startTitle = document.getElementById('startTitle');
    const startSub = document.getElementById('startSub');
    if (awaiting) {
      if (startBtnLive && !startBtnLive.disabled) {
        startBtnLive.textContent = canResume
          ? `Resume ${epochLabel} — ${doneN}/${totalN} done`
          : 'Start training';
      }
      if (startTitle) startTitle.textContent = canResume ? 'Resume training' : 'Training paused';
      if (startSub) startSub.textContent = canResume
        ? `Finished cells keep their ${epochLabel} scores. Only new modes / arches / leftover IDs will run.` +
          (epochMax > 0 ? ` ${epochsLeft} epoch(s) left for this LR.` : '')
        : 'Nothing trains until you press Start. Checkpoint is not written while paused.';
    }
    const overallPct = j.epoch_overall_pct != null ? Number(j.epoch_overall_pct).toFixed(0) + '%' : null;
    const statusEl = document.getElementById('status');
    if (statusEl) statusEl.textContent =
      `${prettyCellId(j.message || '')} · phase ${j.phase || '—'} · ${epochLabel}` +
      (epochMax > 0 ? ` · ${epochsLeft} left · ${overallPct || '—'} overall` : '') +
      ` · hist ${j.history_len||0} · ${
        awaiting ? 'PAUSED' : (runningN(j) > 0 ? `RUNNING ×${runningN(j)}` : 'idle')
      }`;

    const eta = updateETA(j);
    if (has('pieChart')) {
      drawProgressPie(eta);
      renderModeQueue(j.mode_progress || []);
      drawModePie();
      await syncHistoryTip(j.history_len || 0, j.history_tip || []);
    }
    if (has('winSettingsMode')) renderWinners(j.winners || {});

    const bestEl = document.getElementById('bests');
    const b = j.best || {};
    const line = (label, row, key) => {
      if (!row) return `<div>${label}: —</div>`;
      const s = row.snapshot || {};
      const v = key === 'score' ? (s.score||0).toFixed(3)
        : key === 'acc' ? (s.avg_accuracy||0).toFixed(1)+'%'
        : key === 'thru' ? (s.throughput||0).toFixed(1)+'/s'
        : (s.availability||0).toFixed(1)+'%';
      return `<div><span class="pill">${label}</span> ${v} · ${formatCellHTML(row.cell, row.cell?.id)}</div>`;
    };
    if (bestEl) bestEl.innerHTML = [
      line('score', b.score, 'score'),
      line('throughput', b.throughput, 'thru'),
      line('availability', b.availability, 'avail'),
      line('accuracy', b.accuracy, 'acc'),
    ].join('');
    const axEl = document.getElementById('lucyAxes');
    if (axEl) {
      const axes = j.axes || [];
      axEl.innerHTML = axes.length ? axes.map(a => `<tr>
        <td>${a.name||''}</td>
        <td class="num">${(a.value||0).toFixed(2)}</td>
        <td>${prettyMode(a.mode||'—')}</td>
        <td>${a.dtype||'—'}</td>
        <td>${(a.arch||'—').replace(/^cnn$/i,'single')}</td>
        <td>${prettyCellId(a.cell_id||'')}</td>
      </tr>`).join('') : '<tr><td colspan="6">—</td></tr>';
    }

    const mobEl = document.getElementById('mobile');
    const m = j.best_mobile || {};
    const mline = (label, row, effKey) => {
      if (!row) return `<div>${label}: —</div>`;
      const s = row.snapshot || {};
      return `<div><span class="pill">${label}</span> ${(s[effKey]||0).toFixed(2)}/MiB · ${((s.weight_bytes||0)/1024).toFixed(1)} KiB · ${formatCellHTML(row.cell, row.cell?.id)}</div>`;
    };
    if (mobEl) mobEl.innerHTML = [
      mline('score', m.score, 'mobile_score'),
      mline('throughput', m.throughput, 'mobile_throughput'),
      mline('availability', m.availability, 'mobile_availability'),
      mline('accuracy', m.accuracy, 'mobile_accuracy'),
    ].join('');

    const bl = j.best_learn || {};
    const blEl = document.getElementById('bestLearn');
    const learnLine = (label, row, fmt) => {
      if (!row) return `<div>${label}: —</div>`;
      const s = row.snapshot || {};
      return `<div><span class="pill">${label}</span> ${fmt(s)} · ${formatCellHTML(row.cell, row.cell?.id)}</div>`;
    };
    if (blEl) blEl.innerHTML = [
      learnLine('t→25%', bl.to25, s => fmtSec(s && s.time_to_acc25_sec)),
      learnLine('t→50%', bl.to50, s => fmtSec(s && s.time_to_acc50_sec)),
      learnLine('acc/sec', bl.acc_per_sec, s => ((s && s.acc_per_sec)||0).toFixed(3)),
    ].join('');
    const blm = j.best_learn_mobile || {};
    const blmEl = document.getElementById('bestLearnMobile');
    if (blmEl) blmEl.innerHTML = [
      learnLine('acc/sec/MiB', blm.acc_per_sec, s => ((s && s.mobile_acc_per_sec)||0).toFixed(2)),
      learnLine('t→50% /MiB', blm.to50, s => fmtSec(s && s.time_to_acc50_sec)),
    ].join('');

    if (has('board')) renderBoards();
    if (has('rankChart')) drawRank();
    if (has('learnTo50Chart')) drawLearnCharts();
    if (has('lpdChamp')) drawLPD();
    if (has('vsSection') || document.querySelector('[data-chart^="heat-"]')) renderLucyExtras();
    refreshCharts(j.chart_rev || Date.now(), followLive && scrub ? Math.max(0, history.length - 1) : (scrub ? +scrub.value : 0));
  } catch (e) {
    const msg = (e && e.message) ? e.message : String(e);
    const statusEl = document.getElementById('status');
    if (statusEl) statusEl.textContent = 'live error: ' + msg;
    console.error('tide live tick', e);
  }
}
setInterval(tick, 1000);
tick();
let resizeTimer;
window.addEventListener('resize', () => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    if (!lastLive) return;
    refreshCharts(chartRev, followLive && scrub ? Math.max(0, history.length - 1) : (scrub ? +scrub.value : 0));
  }, 120);
});

const startBtn = document.getElementById('startBtn');
if (startBtn) {
  startBtn.addEventListener('click', async () => {
    startBtn.disabled = true;
    startBtn.textContent = 'Starting…';
    try {
      await fetch('/api/start', { method: 'POST' });
      startBtn.textContent = 'Started';
      const banner = document.getElementById('startBanner');
      if (banner) banner.classList.remove('show');
    } catch (e) {
      startBtn.disabled = false;
      startBtn.textContent = 'Start training';
    }
    tick();
  });
}