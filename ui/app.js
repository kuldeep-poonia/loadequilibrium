'use strict';

// ── CONFIG ────────────────────────────────────────────────────────
// ServiceModelBundle has NO json tags → Go serialises as PascalCase:
//   b.Queue, b.Stochastic, b.Signal, b.Stability
//   q.ArrivalRate, q.Utilisation, q.MeanQueueLen, q.MeanSojournMs ...
//   st.BurstAmplification
//   sb.CollapseRisk, sb.CollapseZone
// ControlDirective also has NO json tags → dir.ScaleFactor
// reasoning.Event, TickPayload, NetworkEquilibriumSnapshot etc. DO have
// json tags → those stay snake_case (service_id, system_rho_mean, etc.)

const WS_URL      = `ws://${location.hostname}:8080/ws`;
const MAX_HISTORY = 60;
const MAX_EVENTS  = 200;
const MAX_LOG     = 300;
const RECONNECT   = 3000;
const SPARK_PTS   = 50;

// ── STATE ─────────────────────────────────────────────────────────
const state = {
  connected: false,
  lastPayload: null,
  prevBundles: {},
  history: { rps:[], lat:[], queue:[], rho:[], risk:[], burst:[] },
  incidents: [],
  actions: [],
  pendingAction: null,
};

// ── CLOCK ─────────────────────────────────────────────────────────
setInterval(() => {
  document.getElementById('clock').textContent =
    new Date().toLocaleTimeString('en-GB', { hour12: false });
}, 1000);

// ── WEBSOCKET ─────────────────────────────────────────────────────
let ws, reconnTimer;

function connect() {
  clearTimeout(reconnTimer);
  setWs('connecting');
  ws = new WebSocket(WS_URL);

  ws.onopen  = () => { setWs('connected');    logLine('info', `[ws] connected to ${WS_URL}`); };
  ws.onclose = () => { setWs('disconnected'); logLine('warn', '[ws] disconnected — reconnecting…'); reconnTimer = setTimeout(connect, RECONNECT); };
  ws.onerror = () => { setWs('error');        logLine('err',  '[ws] connection error'); };
  ws.onmessage = (e) => {
    try {
      const p = JSON.parse(e.data);
      if (p.type === 'ping') return;
      handleTick(p);
    } catch(err) { logLine('err', `[ws] parse error: ${err.message}`); }
  };
}

function setWs(s) {
  const dot = document.getElementById('ws-indicator');
  const lbl = document.getElementById('ws-label');
  dot.className = `ws-dot ${s}`;
  lbl.textContent = { connected:'LIVE', connecting:'CONNECTING…', disconnected:'DISCONNECTED', error:'ERROR' }[s] || s.toUpperCase();
  state.connected = s === 'connected';
}

// ── MAIN TICK ─────────────────────────────────────────────────────
function handleTick(p) {
  state.lastPayload = p;

  // tick counter — ControlPlaneState HAS json tags
  const cp = p.control_plane || {};
  if (cp.tick) document.getElementById('tick-count').textContent = cp.tick.toLocaleString();

  // bundles: keys are service names, values are ServiceModelBundle (PascalCase)
  const bundles  = p.bundles || {};
  const services = Object.keys(bundles);
  if (!services.length) { setBadge('waiting'); return; }

  const agg = aggregate(bundles, p);

  push('rps',   agg.totalRps);
  push('lat',   agg.maxLat);
  push('queue', agg.maxQueue);
  push('rho',   agg.maxRho);
  push('risk',  agg.maxRisk);
  push('burst', agg.maxBurst);

  updateHero(agg, p, services.length);
  drawAll(agg);
  updateTable(bundles, p);

  if (Array.isArray(p.events)) processEvents(p.events);
  buildActions(p);
  updateDebug(p);
  logTick(p, agg);

  state.prevBundles = bundles;
}

// ── AGGREGATE ─────────────────────────────────────────────────────
// ALL bundle sub-fields are PascalCase (no json tags on Go structs)
function aggregate(bundles, p) {
  let totalRps=0, maxLat=0, maxQueue=0, maxRho=0, maxRisk=0, maxBurst=0;
  let hottestSvc='', hottestRho=0, degraded=0;

  for (const [svc, b] of Object.entries(bundles)) {
    // b.Queue  = QueueModel  (PascalCase)
    const q  = b.Queue      || {};
    const st = b.Stochastic || {};
    const sb = b.Stability  || {};

    const rps   = q.ArrivalRate       || 0;
    const lat   = q.MeanSojournMs     || q.MeanWaitMs || 0;
    const queue = q.MeanQueueLen      || 0;
    const rho   = q.Utilisation       || 0;
    const risk  = sb.CollapseRisk     || 0;
    const burst = st.BurstAmplification || 0;
    const zone  = (sb.CollapseZone || 'safe').toLowerCase();

    totalRps += rps;
    if (lat   > maxLat)   maxLat   = lat;
    if (queue > maxQueue) maxQueue = queue;
    if (rho   > maxRho)   maxRho   = rho;
    if (risk  > maxRisk)  maxRisk  = risk;
    if (burst > maxBurst) maxBurst = burst;
    if (rho   > hottestRho) { hottestRho = rho; hottestSvc = svc; }
    if (zone === 'warning' || zone === 'collapse') degraded++;
  }

  // NetworkEquilibriumSnapshot HAS json tags → snake_case
  const neq = p.network_equilibrium || {};
  const systemRho = neq.system_rho_mean || maxRho;

  return { totalRps, maxLat, maxQueue, maxRho: systemRho,
           maxRisk, maxBurst, hottestSvc, degraded,
           serviceCount: Object.keys(bundles).length,
           satRisk: neq.network_saturation_risk || maxRisk };
}

// ── HERO ──────────────────────────────────────────────────────────
function updateHero(agg, p, svcCount) {
  let status = 'stable';
  if (agg.satRisk > 0.7 || agg.maxRho > 0.9 || agg.degraded >= svcCount * 0.5) status = 'unstable';
  else if (agg.satRisk > 0.35 || agg.maxRho > 0.65 || agg.degraded > 0)        status = 'degraded';
  setBadge(status);

  document.getElementById('safety-mode-banner').classList.toggle('hidden', !p.safety_mode);

  setHV('rps',      fmt(agg.totalRps,1)+'/s',    'ok');
  setHV('lat',      fmt(agg.maxLat,1)+'ms',       agg.maxLat  > 500  ? 'crit' : agg.maxLat  > 200 ? 'warn' : 'ok');
  setHV('queue',    fmt(agg.maxQueue,1),           agg.maxQueue > 100 ? 'crit' : agg.maxQueue > 30 ? 'warn' : 'ok');
  setHV('risk',     pct(agg.maxRisk),              agg.maxRisk > 0.7  ? 'crit' : agg.maxRisk > 0.35? 'warn' : 'ok');
  setHV('services', svcCount,                      'ok');
  setHV('rho',      pct(agg.maxRho),               rhoLevel(agg.maxRho));
  setHV('degraded', agg.degraded,                  agg.degraded > 2   ? 'crit' : agg.degraded > 0  ? 'warn' : 'ok');
  setHV('hottest',  agg.hottestSvc || '—',         'ok');
}

function setBadge(s) {
  const b = document.getElementById('system-badge');
  const icon = b.querySelector('.badge-icon');
  const text = document.getElementById('badge-text');
  b.className = `system-badge ${s}`;
  const m = { stable:{i:'●',t:'STABLE'}, degraded:{i:'◐',t:'DEGRADED'}, unstable:{i:'■',t:'UNSTABLE'}, waiting:{i:'◌',t:'WAITING FOR TELEMETRY'} }[s] || {i:'◌',t:'WAITING FOR TELEMETRY'};
  icon.textContent = m.i; text.textContent = m.t;
}

function setHV(id, val, level) {
  const el = document.getElementById(`hv-${id}`);
  if (!el) return;
  el.textContent = val;
  el.className = `hmetric-value${level ? ' '+level : ''}`;
}

function rhoLevel(r) { return r > 0.85 ? 'crit' : r > 0.65 ? 'warn' : 'ok'; }

// ── SPARKLINES ────────────────────────────────────────────────────
function push(k, v) {
  const h = state.history[k];
  h.push(isFinite(v) ? v : 0);
  if (h.length > MAX_HISTORY) h.shift();
}

const COLORS = { rps:'#00b8f5', lat:'#f5c400', queue:'#f09800', rho:'#9d72ff', risk:'#ff3c3c', burst:'#00d488' };
const THRESH = { rps:null, lat:500, queue:100, rho:0.85, risk:0.7, burst:2.0 };

function drawSparkline(id, data, color, threshold) {
  const cv = document.getElementById(`chart-${id}`);
  if (!cv) return;
  const W = cv.offsetWidth || cv.width;
  const H = cv.offsetHeight || cv.height;
  cv.width = W; cv.height = H;
  const ctx = cv.getContext('2d');
  ctx.clearRect(0,0,W,H);
  if (data.length < 2) return;

  const pts = data.slice(-SPARK_PTS);
  const n   = pts.length;
  const max = Math.max(...pts, 0.001);
  const pad = 3;

  if (threshold !== null && threshold <= max) {
    const ty = H - pad - (threshold/max)*(H-pad*2);
    ctx.strokeStyle = 'rgba(255,60,60,0.35)';
    ctx.setLineDash([3,3]); ctx.lineWidth = 1;
    ctx.beginPath(); ctx.moveTo(0,ty); ctx.lineTo(W,ty); ctx.stroke();
    ctx.setLineDash([]);
  }

  const g = ctx.createLinearGradient(0,0,0,H);
  g.addColorStop(0, color+'33'); g.addColorStop(1, color+'00');
  ctx.beginPath();
  for (let i=0;i<n;i++) {
    const x = (i/(n-1))*W;
    const y = H - pad - (pts[i]/max)*(H-pad*2);
    i===0 ? ctx.moveTo(x,y) : ctx.lineTo(x,y);
  }
  ctx.lineTo(W,H); ctx.lineTo(0,H); ctx.closePath();
  ctx.fillStyle = g; ctx.fill();

  ctx.beginPath(); ctx.strokeStyle = color; ctx.lineWidth = 1.5; ctx.lineJoin = 'round';
  for (let i=0;i<n;i++) {
    const x = (i/(n-1))*W;
    const y = H - pad - (pts[i]/max)*(H-pad*2);
    i===0 ? ctx.moveTo(x,y) : ctx.lineTo(x,y);
  }
  ctx.stroke();

  const ly = H - pad - (pts[n-1]/max)*(H-pad*2);
  ctx.beginPath(); ctx.arc(W-2,ly,3,0,Math.PI*2); ctx.fillStyle=color; ctx.fill();
}

function drawAll(agg) {
  const vals = { rps:agg.totalRps, lat:agg.maxLat, queue:agg.maxQueue, rho:agg.maxRho, risk:agg.maxRisk, burst:agg.maxBurst };
  const fmts = { rps:v=>fmt(v,1)+'/s', lat:v=>fmt(v,1)+'ms', queue:v=>fmt(v,1), rho:v=>pct(v), risk:v=>pct(v), burst:v=>fmt(v,2)+'×' };
  for (const k of Object.keys(COLORS)) {
    drawSparkline(k, state.history[k], COLORS[k], THRESH[k]);
    const el = document.getElementById(`cval-${k}`);
    if (el) el.textContent = fmts[k](vals[k]);
  }
}

// ── SERVICE TABLE ─────────────────────────────────────────────────
function updateTable(bundles, p) {
  const tbody = document.getElementById('service-tbody');
  const prq   = p.priority_risk_queue || [];   // has json tags ✓
  const zones = p.stability_zones     || {};   // has json tags ✓

  const ordered = prq.map(r => r.service_id)
    .filter(id => bundles[id])
    .concat(Object.keys(bundles).filter(id => !prq.find(r => r.service_id === id)));

  if (!ordered.length) {
    tbody.innerHTML = '<tr><td colspan="10" class="empty-cell">Waiting for telemetry…</td></tr>';
    return;
  }

  let html = '';
  for (const svc of ordered) {
    const b  = bundles[svc] || {};
    // PascalCase — ServiceModelBundle has no json tags
    const q  = b.Queue      || {};
    const st = b.Stochastic || {};
    const sb = b.Stability  || {};

    const rho   = q.Utilisation         || 0;
    const lat   = q.MeanSojournMs       || q.MeanWaitMs || 0;
    const queue = q.MeanQueueLen        || 0;
    const risk  = sb.CollapseRisk       || 0;
    const burst = st.BurstAmplification || 0;
    const zone  = (sb.CollapseZone || zones[svc] || 'safe').toLowerCase();
    const rps   = q.ArrivalRate         || 0;

    let sc, st2;
    if (zone === 'collapse' || risk > 0.7)     { sc='crit'; st2='CRITICAL'; }
    else if (zone === 'warning' || rho > 0.65) { sc='warn'; st2='DEGRADED'; }
    else if (rps > 0 || rho > 0)               { sc='ok';   st2='HEALTHY';  }
    else                                        { sc='wait'; st2='WAITING';  }

    const prev  = state.prevBundles[svc];
    const prevR = prev ? ((prev.Queue||{}).Utilisation||0) : rho;
    const delta = rho - prevR;
    const trend = delta > 0.005 ? '<span class="trend-up">▲</span>'
                : delta < -0.005 ? '<span class="trend-down">▼</span>'
                : '<span class="trend-flat">—</span>';

    html += `<tr>
      <td style="color:var(--cyan);font-weight:600">${esc(svc)}</td>
      <td><div class="status-cell"><div class="status-dot ${sc}"></div><span class="status-text ${sc}">${st2}</span></div></td>
      <td>${fmt(rps,1)}/s</td>
      <td>${fmt(lat,1)}ms</td>
      <td>${fmt(queue,1)}</td>
      <td>${bar(rho,1,rhoLevel(rho))}</td>
      <td>${bar(risk,1,risk>0.7?'crit':risk>0.35?'warn':'ok')}</td>
      <td style="color:${burst>2?'var(--red)':burst>1.3?'var(--yellow)':'var(--green)'}">${fmt(burst,2)}×</td>
      <td><span class="zone-badge ${zone}">${zone.toUpperCase()}</span></td>
      <td>${trend}</td>
    </tr>`;
  }
  tbody.innerHTML = html;
  document.getElementById('svc-table-meta').textContent = `${ordered.length} service${ordered.length!==1?'s':''} · live`;
}

function bar(val, max, level) {
  const p2 = Math.min(100, (val/max)*100);
  const disp = isFinite(val) ? (max===1 ? (val*100).toFixed(0)+'%' : val.toFixed(1)) : '—';
  return `<div class="bar-cell"><div class="bar-bg"><div class="bar-fill ${level}" style="width:${p2}%"></div></div><span>${disp}</span></div>`;
}

// ── EVENTS ────────────────────────────────────────────────────────
// reasoning.Event HAS json tags → snake_case (service_id, severity, etc.)
function processEvents(events) {
  const valid = events.filter(e => e && e.description);
  if (!valid.length) return;

  state.incidents = valid.filter(e => e.severity >= 1).slice(0,8);
  renderIncidents();

  for (const e of valid) pushEvent(e);
  buildCausal(valid);
}

function renderIncidents() {
  const el  = document.getElementById('incident-list');
  const cnt = document.getElementById('incident-count');
  cnt.textContent = state.incidents.length;
  if (!state.incidents.length) { el.innerHTML = '<div class="empty-state">No active incidents detected</div>'; return; }
  el.innerHTML = state.incidents.map(e => {
    const sc  = e.severity >= 2 ? 'crit' : e.severity === 1 ? 'warn' : 'info';
    const lbl = e.severity >= 2 ? 'CRIT' : e.severity === 1 ? 'WARN' : 'INFO';
    return `<div class="incident-item ${sc}">
      <div class="inc-header">
        <span class="inc-severity ${sc}">${lbl}</span>
        <span class="inc-svc">${esc(e.service_id||'system')}</span>
        <span class="inc-time">${fmtTs(e.timestamp)}</span>
      </div>
      <div class="inc-desc">${esc(e.description||'')}</div>
      ${e.recommendation ? `<div class="inc-rec">→ ${esc(e.recommendation)}</div>` : ''}
    </div>`;
  }).join('');
}

const evPause = document.getElementById('event-pause');
function pushEvent(e) {
  if (evPause.checked) return;
  const stream = document.getElementById('event-stream');
  const sc  = e.severity >= 2 ? 'crit' : e.severity === 1 ? 'warn' : 'info';
  const lbl = e.severity >= 2 ? 'CRIT' : e.severity === 1 ? 'WARN' : 'INFO';
  const svc = e.service_id ? `<span class="svc-tag">[${esc(e.service_id)}]</span> ` : '';
  const row = document.createElement('div');
  row.className = 'event-row';
  row.innerHTML = `<span class="event-ts">${fmtTs(e.timestamp)}</span><span class="event-sev ${sc}">${lbl}</span><span class="event-msg">${svc}${esc(e.description||e.category||'')}</span>`;
  stream.prepend(row);
  while (stream.children.length > MAX_EVENTS) stream.removeChild(stream.lastChild);
}

function buildCausal(events) {
  const el = document.getElementById('causal-chain');
  const top = [...events].sort((a,b) => b.operational_priority - a.operational_priority)[0];
  if (!top) { el.innerHTML = '<div class="empty-state">No chain available</div>'; return; }

  if (top.model_chain) {
    const steps = top.model_chain.split('→').map(s=>s.trim()).filter(Boolean);
    if (steps.length) {
      el.innerHTML = steps.map((s,i) => {
        const cls = i===0?'root':i===steps.length-1?'final':'';
        const arr = i===0?'':`<span class="causal-arrow">↓</span>`;
        return `<div class="causal-step">${arr}<span class="causal-text ${cls}">${esc(s)}</span></div>`;
      }).join('');
      return;
    }
  }

  const steps = [...events].sort((a,b) => b.operational_priority - a.operational_priority)
    .slice(0,4).map(e=>e.description).filter(Boolean);
  if (steps.length) {
    el.innerHTML = steps.map((s,i) => {
      const cls = i===0?'root':i===steps.length-1?'final':'';
      const arr = i===0?'':`<span class="causal-arrow">↓</span>`;
      return `<div class="causal-step">${arr}<span class="causal-text ${cls}">${esc(s)}</span></div>`;
    }).join('');
  } else {
    el.innerHTML = '<div class="empty-state">System operating normally</div>';
  }
}

// ── ACTIONS ───────────────────────────────────────────────────────
// ControlDirective has NO json tags → PascalCase (ScaleFactor)
function buildActions(p) {
  const events     = p.events    || [];
  const directives = p.directives|| {};  // map[string]ControlDirective
  const recs = [];
  const seen = new Set();

  for (const e of events) {
    if (e.recommendation && e.severity >= 1 && !seen.has(e.recommendation)) {
      seen.add(e.recommendation);
      recs.push({ icon: e.severity>=2?'🔴':'🟡', title: e.recommendation,
                  detail: e.service_id ? `Service: ${e.service_id}` : 'System-level', event: e });
    }
  }

  // ControlDirective.ScaleFactor — PascalCase (no json tags)
  for (const [svc, dir] of Object.entries(directives)) {
    if (dir.ScaleFactor && Math.abs(dir.ScaleFactor - 1.0) > 0.05) {
      const up  = dir.ScaleFactor > 1.0;
      const pct2 = Math.abs((dir.ScaleFactor-1)*100).toFixed(0);
      const title = `Scale ${up?'up':'down'} ${svc} by ${pct2}%`;
      if (!seen.has(title)) {
        seen.add(title);
        recs.push({ icon: up?'⬆':'⬇', title, detail:`Factor: ${dir.ScaleFactor.toFixed(2)}`, svc });
      }
    }
  }

  state.actions = recs.slice(0,6);
  renderActions();
}

function renderActions() {
  const el  = document.getElementById('action-list');
  const cnt = document.getElementById('action-count');
  cnt.textContent = state.actions.length;
  if (!state.actions.length) { el.innerHTML = '<div class="empty-state">No recommendations at this time</div>'; return; }
  el.innerHTML = state.actions.map((a,i) => `
    <div class="action-item">
      <span class="action-icon">${a.icon}</span>
      <div class="action-body">
        <div class="action-title">${esc(a.title)}</div>
        <div class="action-detail">${esc(a.detail)}</div>
      </div>
      <button class="action-apply" data-idx="${i}">APPLY</button>
    </div>`).join('');
  el.querySelectorAll('.action-apply').forEach(btn => {
    btn.addEventListener('click', () => openModal(+btn.dataset.idx));
  });
}

// ── MODAL ─────────────────────────────────────────────────────────
function openModal(idx) {
  const a = state.actions[idx]; if (!a) return;
  state.pendingAction = a;
  document.getElementById('modal-service-name').textContent = a.svc || (a.event&&a.event.service_id) || 'System';
  document.getElementById('modal-action-desc').textContent  = a.title;

  // Evidence HAS json tags → snake_case
  const ev  = (a.event && a.event.evidence) || {};
  const kvs = [
    ['Utilisation',    ev.utilisation    != null ? pct(ev.utilisation)      : null],
    ['Collapse Risk',  ev.collapse_risk  != null ? pct(ev.collapse_risk)    : null],
    ['Queue Wait',     ev.queue_wait_ms  != null ? fmt(ev.queue_wait_ms,1)+'ms' : null],
    ['Burst Factor',   ev.burst_factor   != null ? fmt(ev.burst_factor,2)+'×'   : null],
    ['Cascade Risk',   ev.cascade_risk   != null ? pct(ev.cascade_risk)     : null],
    ['Stab. Margin',   ev.stability_margin != null ? pct(ev.stability_margin): null],
  ].filter(([,v]) => v !== null);

  document.getElementById('modal-evidence').innerHTML =
    kvs.map(([k,v]) => `<div class="ev-kv"><span class="ev-key">${k}</span><span class="ev-val">${v}</span></div>`).join('')
    || '<span style="color:var(--text-dim)">No evidence data</span>';

  document.getElementById('modal-overlay').classList.remove('hidden');
}

function closeModal() {
  document.getElementById('modal-overlay').classList.add('hidden');
  state.pendingAction = null;
}

document.getElementById('modal-close').addEventListener('click', closeModal);
document.getElementById('modal-deny').addEventListener('click',  closeModal);
document.getElementById('modal-overlay').addEventListener('click', e => { if (e.target===document.getElementById('modal-overlay')) closeModal(); });
document.getElementById('modal-approve').addEventListener('click', () => {
  const a = state.pendingAction; if (!a) { closeModal(); return; }
  logLine('info', `[action] APPROVED: ${a.title}`);
  pushEventDirect('INFO', null, `Operator approved: ${a.title}`);
  closeModal();
});

// ── DEBUG ─────────────────────────────────────────────────────────
// All these structs have json tags → snake_case
function updateDebug(p) {
  const tab = document.getElementById('debug-tab');
  if (!tab.open) return;

  const fp  = p.fixed_point_equilibrium || {};
  kv('debug-fp', {
    'Converged':        fp.converged ? 'YES' : 'NO',
    'Convergence Rate': fmt(fp.convergence_rate,3),
    'Stability Margin': fmt(fp.stability_margin,3),
    'Systemic Collapse':pct(fp.systemic_collapse_prob||0),
    'Iterations':       fp.converged_iterations,
  });

  const neq = p.network_equilibrium || {};
  kv('debug-neq', {
    'System ρ Mean':    pct(neq.system_rho_mean||0),
    'System ρ Var':     fmt(neq.system_rho_variance||0,4),
    'Eq. Delta':        fmt(neq.equilibrium_delta||0,4),
    'Converging':       neq.is_converging ? 'YES' : 'NO',
    'Critical Svc':     neq.critical_service_id||'—',
    'Network Sat Risk': pct(neq.network_saturation_risk||0),
  });

  const env = p.stability_envelope || {};
  kv('debug-env', {
    'Safe ρ Max':      pct(env.safe_system_rho_max||0),
    'Current ρ Mean':  pct(env.current_system_rho_mean||0),
    'Headroom':        pct(env.envelope_headroom||0),
    'Most Vulnerable': env.most_vulnerable_service||'—',
    'Worst Pert.Δ':    fmt(env.worst_perturbation_delta||0,4),
  });

  const rm = p.runtime_metrics || {};
  kv('debug-rt', {
    'Prune ms':    fmt(rm.avg_prune_ms,2),
    'Windows ms':  fmt(rm.avg_windows_ms,2),
    'Topology ms': fmt(rm.avg_topology_ms,2),
    'Modelling ms':fmt(rm.avg_modelling_ms,2),
    'Optimise ms': fmt(rm.avg_optimise_ms,2),
    'Sim ms':      fmt(rm.avg_sim_ms,2),
    'Reasoning ms':fmt(rm.avg_reasoning_ms,2),
    'Overruns':    rm.total_overruns,
    'Consec':      rm.consec_overruns,
  });

  document.getElementById('debug-raw').textContent = JSON.stringify(p,null,2).slice(0,4000);
}

function kv(id, obj) {
  document.getElementById(id).innerHTML = Object.entries(obj)
    .map(([k,v]) => `<div><span class="k">${k}</span><span class="v">${v??'—'}</span></div>`).join('');
}

// ── LOG ───────────────────────────────────────────────────────────
const logPause = document.getElementById('log-pause');
document.getElementById('log-clear').addEventListener('click', () => {
  document.getElementById('log-terminal').innerHTML = '';
});

function logLine(level, msg) {
  if (logPause.checked) return;
  const term = document.getElementById('log-terminal');
  const p = document.createElement('p');
  p.className = `log-line ${level}`;
  p.textContent = `${new Date().toLocaleTimeString('en-GB',{hour12:false})}  ${msg}`;
  term.appendChild(p);
  term.scrollTop = term.scrollHeight;
  while (term.children.length > MAX_LOG) term.removeChild(term.firstChild);
}

function logTick(p, agg) {
  const tick = (p.control_plane||{}).tick || 0;
  logLine('info', `tick=${tick}  rps=${fmt(agg.totalRps,1)}/s  ρ=${pct(agg.maxRho)}  risk=${pct(agg.maxRisk)}  lat=${fmt(agg.maxLat,1)}ms  tick_ms=${fmt(p.tick_health_ms,1)}`);
}

function pushEventDirect(sev, svc, msg) {
  const stream = document.getElementById('event-stream');
  const sc = sev==='CRIT'?'crit':sev==='WARN'?'warn':'info';
  const ts = new Date().toLocaleTimeString('en-GB',{hour12:false});
  const sv = svc ? `<span class="svc-tag">[${esc(svc)}]</span> ` : '';
  const row = document.createElement('div');
  row.className = 'event-row';
  row.innerHTML = `<span class="event-ts">${ts}</span><span class="event-sev ${sc}">${sev}</span><span class="event-msg">${sv}${esc(msg)}</span>`;
  stream.prepend(row);
  while (stream.children.length > MAX_EVENTS) stream.removeChild(stream.lastChild);
}

// ── UTILS ─────────────────────────────────────────────────────────
function fmt(v, d=2) { if (v==null||!isFinite(v)) return '—'; return Number(v).toFixed(d); }
function pct(v)      { if (v==null||!isFinite(v)) return '—'; return (v*100).toFixed(1)+'%'; }
function esc(s)      { if (!s) return ''; return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); }
function fmtTs(ts)   { if (!ts) return new Date().toLocaleTimeString('en-GB',{hour12:false}); const d=new Date(ts); return isNaN(d)?'—':d.toLocaleTimeString('en-GB',{hour12:false}); }

// ── INIT ──────────────────────────────────────────────────────────
connect();
window.addEventListener('resize', () => {
  if (state.lastPayload) drawAll(aggregate(state.lastPayload.bundles||{}, state.lastPayload));
});