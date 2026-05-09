

'use strict';

//  CONFIG 
// Backend WebSocket endpoint: registered at /ws in internal/api/server.go
const WS_URL        = `ws://${location.hostname}:8080/ws`;
const MAX_HISTORY   = 60;   // data points per sparkline
const MAX_EVENTS    = 200;  // event stream rows
const MAX_LOG_LINES = 300;  // log terminal lines
const RECONNECT_MS  = 3000;
const SPARKLINE_PTS = 50;

//  STATE 
const state = {
  connected:    false,
  lastPayload:  null,
  prevBundles:  {},          // for trend detection
  history: {                 // rolling arrays for sparklines
    rps:   [], lat:  [],
    queue: [], rho:  [],
    risk:  [], burst: [],
  },
  incidents:    [],          // active reasoning events
  causalSteps:  [],          // causal chain for display
  actions:      [],          // pending recommendations
  pendingAction: null,       // action waiting for approval
};

//  CLOCK ─
function startClock() {
  const el = document.getElementById('clock');
  setInterval(() => {
    el.textContent = new Date().toLocaleTimeString('en-GB', { hour12: false });
  }, 1000);
}

//  WEBSOCKET ─
let ws = null;
let reconnectTimer = null;

function connect() {
  clearTimeout(reconnectTimer);
  setWsStatus('connecting');

  ws = new WebSocket(WS_URL);

  ws.onopen = () => {
    setWsStatus('connected');
    logLine('info', `[ws] connected to ${WS_URL}`);
  };

  ws.onmessage = (e) => {
    try {
      const payload = JSON.parse(e.data);
      if (payload.type === 'ping') return;
      handleTick(payload);
    } catch (err) {
      logLine('err', `[ws] parse error: ${err.message}`);
    }
  };

  ws.onclose = () => {
    setWsStatus('disconnected');
    logLine('warn', '[ws] disconnected — reconnecting in 3s…');
    reconnectTimer = setTimeout(connect, RECONNECT_MS);
  };

  ws.onerror = () => {
    setWsStatus('error');
    logLine('err', '[ws] connection error');
  };
}

function setWsStatus(status) {
  const dot   = document.getElementById('ws-indicator');
  const label = document.getElementById('ws-label');
  dot.className = `ws-dot ${status}`;
  const map = {
    connected:    'LIVE',
    connecting:   'CONNECTING…',
    disconnected: 'DISCONNECTED',
    error:        'ERROR',
  };
  label.textContent = map[status] || status.toUpperCase();
  state.connected = status === 'connected';
}

//  MAIN TICK HANDLER ─
function handleTick(payload) {
  state.lastPayload = payload;

  // Update tick counter
  const cp = payload.control_plane || {};
  if (cp.tick) {
    document.getElementById('tick-count').textContent =
      cp.tick.toLocaleString();
  }

  // Derive aggregate metrics from all service bundles
  const bundles   = payload.bundles || {};
  const services  = Object.keys(bundles);
  const hasData   = services.length > 0;

  if (!hasData) {
    setBadge('waiting');
    return;
  }

  // Aggregate metrics across all services
  const agg = aggregateMetrics(bundles, payload);

  // Push history
  pushHistory('rps',   agg.totalRps);
  pushHistory('lat',   agg.maxLat);
  pushHistory('queue', agg.maxQueue);
  pushHistory('rho',   agg.maxRho);
  pushHistory('risk',  agg.maxRisk);
  pushHistory('burst', agg.maxBurst);

  // Update hero banner
  updateHero(agg, payload, services.length);

  // Update sparklines
  drawAllSparklines(agg);

  // Update service table
  updateServiceTable(bundles, payload);

  // Handle reasoning events → incidents + event stream + causal chain
  if (Array.isArray(payload.events)) {
    processEvents(payload.events);
  }

  // Build action recommendations from events
  buildRecommendations(payload);

  // Update debug panel if open
  updateDebug(payload);

  // Log tick summary
  logTick(payload, agg);

  // Store prev bundles for trend calc
  state.prevBundles = bundles;
}

//  METRIC AGGREGATION 
function aggregateMetrics(bundles, payload) {
  let totalRps  = 0, maxLat   = 0, maxQueue = 0;
  let maxRho    = 0, maxRisk  = 0, maxBurst = 0;
  let hottestSvc = '', hottestRho = 0;
  let degraded  = 0;

  for (const [svc, b] of Object.entries(bundles)) {
    const q  = b.queue      || {};
    const st = b.stochastic || {};
    const sb = b.stability  || {};

    const rps   = q.arrival_rate    || 0;
    const lat   = q.mean_sojourn_ms || q.mean_wait_ms || 0;
    const queue = q.mean_queue_len  || 0;
    const rho   = q.utilisation     || 0;
    const risk  = sb.collapse_risk  || 0;
    const burst = st.burst_amplification || 0;

    totalRps  += rps;
    if (lat   > maxLat)   maxLat   = lat;
    if (queue > maxQueue) maxQueue = queue;
    if (rho   > maxRho)   { maxRho = rho; }
    if (risk  > maxRisk)  maxRisk  = risk;
    if (burst > maxBurst) maxBurst = burst;

    if (rho > hottestRho) { hottestRho = rho; hottestSvc = svc; }

    const zone = (sb.collapse_zone || 'safe').toLowerCase();
    if (zone === 'warning' || zone === 'collapse') degraded++;
  }

  // System-level rho from network equilibrium
  const neq = payload.network_equilibrium || {};
  const systemRho = neq.system_rho_mean || maxRho;

  // Stability envelope headroom
  const env = payload.stability_envelope || {};

  return {
    totalRps, maxLat, maxQueue, maxRho: systemRho,
    maxRisk, maxBurst, hottestSvc,
    degraded, serviceCount: Object.keys(bundles).length,
    envHeadroom: env.envelope_headroom || 0,
    satRisk: neq.network_saturation_risk || maxRisk,
  };
}

//  HERO UPDATE ─
function updateHero(agg, payload, svcCount) {
  // Determine system status
  let status = 'stable';
  if (agg.satRisk > 0.7 || agg.maxRho > 0.9 || agg.degraded >= svcCount * 0.5) {
    status = 'unstable';
  } else if (agg.satRisk > 0.35 || agg.maxRho > 0.65 || agg.degraded > 0) {
    status = 'degraded';
  }

  setBadge(status);

  // Safety mode banner
  const safetyBanner = document.getElementById('safety-mode-banner');
  if (payload.safety_mode) {
    safetyBanner.classList.remove('hidden');
  } else {
    safetyBanner.classList.add('hidden');
  }

  // Update hero metrics
  setHeroVal('rps',      fmt(agg.totalRps, 1) + '/s',       levelForRho(0));
  setHeroVal('lat',      fmt(agg.maxLat, 1) + 'ms',         agg.maxLat > 500 ? 'crit' : agg.maxLat > 200 ? 'warn' : 'ok');
  setHeroVal('queue',    fmt(agg.maxQueue, 1),               agg.maxQueue > 100 ? 'crit' : agg.maxQueue > 30 ? 'warn' : 'ok');
  setHeroVal('risk',     pct(agg.maxRisk),                   agg.maxRisk > 0.7 ? 'crit' : agg.maxRisk > 0.35 ? 'warn' : 'ok');
  setHeroVal('services', svcCount,                           'ok');
  setHeroVal('rho',      pct(agg.maxRho),                    levelForRho(agg.maxRho));
  setHeroVal('degraded', agg.degraded,                       agg.degraded > 0 ? (agg.degraded > 2 ? 'crit' : 'warn') : 'ok');
  setHeroVal('hottest',  agg.hottestSvc || '—',              'ok');

  // Uptime
  const rm = payload.runtime_metrics || {};
}

function setBadge(status) {
  const badge = document.getElementById('system-badge');
  const icon  = badge.querySelector('.badge-icon');
  const text  = document.getElementById('badge-text');

  badge.className = `system-badge ${status}`;

  const map = {
    stable:   { icon: '●', text: 'STABLE' },
    degraded: { icon: '◐', text: 'DEGRADED' },
    unstable: { icon: '■', text: 'UNSTABLE' },
    waiting:  { icon: '◌', text: 'WAITING FOR TELEMETRY' },
  };
  const m = map[status] || map.waiting;
  icon.textContent = m.icon;
  text.textContent = m.text;
}

function setHeroVal(id, val, level) {
  const el = document.getElementById(`hv-${id}`);
  if (!el) return;
  el.textContent = val;
  el.className   = `hmetric-value${level ? ' ' + level : ''}`;
}

function levelForRho(rho) {
  if (rho > 0.85) return 'crit';
  if (rho > 0.65) return 'warn';
  return 'ok';
}

//  SPARKLINES 
function pushHistory(key, val) {
  const h = state.history[key];
  h.push(isFinite(val) ? val : 0);
  if (h.length > MAX_HISTORY) h.shift();
}

const chartColors = {
  rps:   '#00b8f5',
  lat:   '#f5c400',
  queue: '#f09800',
  rho:   '#9d72ff',
  risk:  '#ff3c3c',
  burst: '#00d488',
};

const chartThresholds = {
  rps:   null,
  lat:   500,
  queue: 100,
  rho:   0.85,
  risk:  0.7,
  burst: 2.0,
};

function drawSparkline(id, data, color, threshold) {
  const canvas = document.getElementById(`chart-${id}`);
  if (!canvas) return;
  const ctx  = canvas.getContext('2d');
  const W    = canvas.offsetWidth  || canvas.width;
  const H    = canvas.offsetHeight || canvas.height;
  canvas.width  = W;
  canvas.height = H;

  ctx.clearRect(0, 0, W, H);

  if (data.length < 2) return;

  const max = Math.max(...data, 0.001);
  const pts = data.slice(-SPARKLINE_PTS);
  const n   = pts.length;
  const pad = 3;

  // Draw threshold line if set
  if (threshold !== null && threshold <= max) {
    const ty = H - pad - ((threshold / max) * (H - pad * 2));
    ctx.strokeStyle = 'rgba(255,60,60,0.3)';
    ctx.setLineDash([3, 3]);
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, ty);
    ctx.lineTo(W, ty);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  // Fill under line
  const gradient = ctx.createLinearGradient(0, 0, 0, H);
  gradient.addColorStop(0, color + '33');
  gradient.addColorStop(1, color + '00');

  ctx.beginPath();
  for (let i = 0; i < n; i++) {
    const x = (i / (n - 1)) * W;
    const y = H - pad - ((pts[i] / max) * (H - pad * 2));
    i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
  }
  ctx.lineTo(W, H);
  ctx.lineTo(0, H);
  ctx.closePath();
  ctx.fillStyle = gradient;
  ctx.fill();

  // Line
  ctx.beginPath();
  ctx.strokeStyle = color;
  ctx.lineWidth = 1.5;
  ctx.lineJoin  = 'round';
  for (let i = 0; i < n; i++) {
    const x = (i / (n - 1)) * W;
    const y = H - pad - ((pts[i] / max) * (H - pad * 2));
    i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
  }
  ctx.stroke();

  // Current dot
  const last = pts[n - 1];
  const lx   = W;
  const ly   = H - pad - ((last / max) * (H - pad * 2));
  ctx.beginPath();
  ctx.arc(lx - 2, ly, 3, 0, Math.PI * 2);
  ctx.fillStyle = color;
  ctx.fill();
}

function drawAllSparklines(agg) {
  const keys = ['rps', 'lat', 'queue', 'rho', 'risk', 'burst'];
  const vals = {
    rps:   agg.totalRps,
    lat:   agg.maxLat,
    queue: agg.maxQueue,
    rho:   agg.maxRho,
    risk:  agg.maxRisk,
    burst: agg.maxBurst,
  };
  const fmts = {
    rps:   v => fmt(v, 1) + '/s',
    lat:   v => fmt(v, 1) + 'ms',
    queue: v => fmt(v, 1),
    rho:   v => pct(v),
    risk:  v => pct(v),
    burst: v => fmt(v, 2) + '×',
  };

  for (const k of keys) {
    drawSparkline(k, state.history[k], chartColors[k], chartThresholds[k]);
    const el = document.getElementById(`cval-${k}`);
    if (el) el.textContent = fmts[k](vals[k]);
  }
}

//  SERVICE TABLE ─
function updateServiceTable(bundles, payload) {
  const tbody = document.getElementById('service-tbody');
  const prq   = payload.priority_risk_queue || [];
  const zones = payload.stability_zones     || {};
  const ph    = payload.pressure_heatmap    || {};
  const simOv = payload.sim_overlay         || {};

  // Order by risk queue, then alphabetical for unlisted
  const ordered = prq.map(r => r.service_id)
    .filter(id => bundles[id])
    .concat(
      Object.keys(bundles).filter(id => !prq.find(r => r.service_id === id))
    );

  if (ordered.length === 0) {
    tbody.innerHTML = '<tr><td colspan="10" class="empty-cell">Waiting for telemetry…</td></tr>';
    return;
  }

  let html = '';
  for (const svc of ordered) {
    const b   = bundles[svc] || {};
    const q   = b.queue      || {};
    const st  = b.stochastic || {};
    const sb  = b.stability  || {};
    const sig = b.signal     || {};

    const rho    = q.utilisation     || 0;
    const lat    = q.mean_sojourn_ms || 0;
    const queue  = q.mean_queue_len  || 0;
    const risk   = sb.collapse_risk  || 0;
    const burst  = st.burst_amplification || 0;
    const zone   = (sb.collapse_zone || zones[svc] || 'safe').toLowerCase();
    const rps    = q.arrival_rate    || 0;

    // Status
    let statusClass, statusText;
    if (zone === 'collapse' || risk > 0.7) {
      statusClass = 'crit'; statusText = 'CRITICAL';
    } else if (zone === 'warning' || rho > 0.65) {
      statusClass = 'warn'; statusText = 'DEGRADED';
    } else if (rho > 0) {
      statusClass = 'ok';   statusText = 'HEALTHY';
    } else {
      statusClass = 'wait'; statusText = 'WAITING';
    }

    // Trend vs prev tick
    const prev  = state.prevBundles[svc];
    const prevR = prev ? (prev.queue || {}).utilisation || 0 : rho;
    const delta = rho - prevR;
    let trendHtml;
    if (delta > 0.005)       trendHtml = '<span class="trend-up">▲</span>';
    else if (delta < -0.005) trendHtml = '<span class="trend-down">▼</span>';
    else                     trendHtml = '<span class="trend-flat">—</span>';

    // Bar helpers
    const rhoBar   = barHtml(rho,   1,    levelForRho(rho));
    const riskBar  = barHtml(risk,  1,    risk > 0.7 ? 'crit' : risk > 0.35 ? 'warn' : 'ok');

    // Zone badge
    const zoneBadge = `<span class="zone-badge ${zone}">${zone.toUpperCase()}</span>`;

    html += `<tr>
      <td style="color:var(--cyan);font-weight:600">${escHtml(svc)}</td>
      <td>
        <div class="status-cell">
          <div class="status-dot ${statusClass}"></div>
          <span class="status-text ${statusClass}">${statusText}</span>
        </div>
      </td>
      <td>${fmt(rps, 1)}/s</td>
      <td>${fmt(lat, 1)}ms</td>
      <td>${fmt(queue, 1)}</td>
      <td>${rhoBar}</td>
      <td>${riskBar}</td>
      <td style="color:${burst > 2 ? 'var(--red)' : burst > 1.3 ? 'var(--yellow)' : 'var(--green)'}">${fmt(burst, 2)}×</td>
      <td>${zoneBadge}</td>
      <td>${trendHtml}</td>
    </tr>`;
  }

  tbody.innerHTML = html;
  document.getElementById('svc-table-meta').textContent =
    `${ordered.length} service${ordered.length !== 1 ? 's' : ''} · live`;
}

function barHtml(val, max, level) {
  const pct = Math.min(100, (val / max) * 100);
  return `<div class="bar-cell">
    <div class="bar-bg"><div class="bar-fill ${level}" style="width:${pct}%"></div></div>
    <span>${isFinite(val) ? (max === 1 ? (val * 100).toFixed(0) + '%' : val.toFixed(1)) : '—'}</span>
  </div>`;
}

//  EVENTS → INCIDENTS + STREAM ─
function processEvents(events) {
  // Only show events from this tick (filter by recency is done by backend)
  const newEvents = events.filter(e => e && e.description);
  if (newEvents.length === 0) return;

  // Update incident list (critical + warning only)
  const incidents = newEvents.filter(e => e.severity >= 1); // WARN or CRIT
  state.incidents = incidents.slice(0, 8);
  renderIncidents();

  // Push to event stream
  for (const e of newEvents) {
    pushEventRow(e);
  }

  // Build causal chain from high-priority events
  buildCausalChain(newEvents);
}

function renderIncidents() {
  const el  = document.getElementById('incident-list');
  const cnt = document.getElementById('incident-count');
  cnt.textContent = state.incidents.length;

  if (state.incidents.length === 0) {
    el.innerHTML = '<div class="empty-state">No active incidents detected</div>';
    return;
  }

  el.innerHTML = state.incidents.map(e => {
    const sevClass = e.severity >= 2 ? 'crit' : e.severity === 1 ? 'warn' : 'info';
    const sevLabel = e.severity >= 2 ? 'CRIT' : e.severity === 1 ? 'WARN' : 'INFO';
    const ts       = formatTs(e.timestamp);
    const ev       = e.evidence || {};
    return `<div class="incident-item ${sevClass}">
      <div class="inc-header">
        <span class="inc-severity ${sevClass}">${sevLabel}</span>
        <span class="inc-svc">${escHtml(e.service_id || 'system')}</span>
        <span class="inc-time">${ts}</span>
      </div>
      <div class="inc-desc">${escHtml(e.description || '')}</div>
      ${e.recommendation ? `<div class="inc-rec">→ ${escHtml(e.recommendation)}</div>` : ''}
    </div>`;
  }).join('');
}

const eventPause = document.getElementById('event-pause');
function pushEventRow(e) {
  if (eventPause.checked) return;
  const stream   = document.getElementById('event-stream');
  const sevClass = e.severity >= 2 ? 'crit' : e.severity === 1 ? 'warn' : 'info';
  const sevLabel = e.severity >= 2 ? 'CRIT' : e.severity === 1 ? 'WARN' : 'INFO';
  const ts       = formatTs(e.timestamp);
  const svc      = e.service_id ? `<span class="svc-tag">[${escHtml(e.service_id)}]</span> ` : '';

  const row = document.createElement('div');
  row.className = 'event-row';
  row.innerHTML = `
    <span class="event-ts">${ts}</span>
    <span class="event-sev ${sevClass}">${sevLabel}</span>
    <span class="event-msg">${svc}${escHtml(e.description || e.category || '')}</span>`;

  stream.prepend(row);

  // Trim
  while (stream.children.length > MAX_EVENTS) {
    stream.removeChild(stream.lastChild);
  }
}

//  CAUSAL CHAIN 
function buildCausalChain(events) {
  const el = document.getElementById('causal-chain');

  // Use model_chain from highest-priority event if available
  const ranked = [...events].sort((a, b) => b.operational_priority - a.operational_priority);
  const top    = ranked[0];

  if (!top) {
    el.innerHTML = '<div class="empty-state">No chain available</div>';
    return;
  }

  if (top.model_chain) {
    // Parse "cause→model→prediction→action"
    const steps = top.model_chain.split('→').map(s => s.trim()).filter(Boolean);
    if (steps.length > 0) {
      state.causalSteps = steps;
      el.innerHTML = steps.map((step, i) => {
        const cls = i === 0 ? 'root' : i === steps.length - 1 ? 'final' : '';
        const arrow = i === 0 ? '' :
          `<span class="causal-arrow">↓</span>`;
        return `<div class="causal-step">${arrow}<span class="causal-text ${cls}">${escHtml(step)}</span></div>`;
      }).join('');
      return;
    }
  }

  // Build from multiple events
  const steps = ranked.slice(0, 4).map(e => e.description).filter(Boolean);
  if (steps.length > 0) {
    el.innerHTML = steps.map((step, i) => {
      const cls = i === 0 ? 'root' : i === steps.length - 1 ? 'final' : '';
      const arrow = i === 0 ? '' : `<span class="causal-arrow">↓</span>`;
      return `<div class="causal-step">${arrow}<span class="causal-text ${cls}">${escHtml(step)}</span></div>`;
    }).join('');
  } else {
    el.innerHTML = '<div class="empty-state">System operating normally</div>';
  }
}

//  RECOMMENDATIONS ─
function buildRecommendations(payload) {
  const events  = payload.events || [];
  const directives = payload.directives || {};
  const prq     = payload.priority_risk_queue || [];

  const recs = [];

  // From reasoning events with recommendations
  for (const e of events) {
    if (e.recommendation && e.severity >= 1) {
      recs.push({
        icon:    e.severity >= 2 ? '🔴' : '🟡',
        title:   e.recommendation,
        detail:  e.service_id ? `Service: ${e.service_id}` : 'System-level',
        event:   e,
      });
    }
  }

  // From control directives
  for (const [svc, dir] of Object.entries(directives)) {
    if (dir.scale_factor && Math.abs(dir.scale_factor - 1.0) > 0.05) {
      const scaleDir = dir.scale_factor > 1.0 ? 'up' : 'down';
      const pct      = Math.abs((dir.scale_factor - 1.0) * 100).toFixed(0);
      recs.push({
        icon:   scaleDir === 'up' ? '⬆' : '⬇',
        title:  `Scale ${scaleDir} ${svc} by ${pct}%`,
        detail: `Current factor: ${dir.scale_factor.toFixed(2)}`,
        dir:    dir,
        svc,
      });
    }
  }

  // Deduplicate by title
  const seen  = new Set();
  state.actions = recs.filter(r => {
    if (seen.has(r.title)) return false;
    seen.add(r.title);
    return true;
  }).slice(0, 6);

  renderActions();
}

function renderActions() {
  const el  = document.getElementById('action-list');
  const cnt = document.getElementById('action-count');
  cnt.textContent = state.actions.length;

  if (state.actions.length === 0) {
    el.innerHTML = '<div class="empty-state">No recommendations at this time</div>';
    return;
  }

  el.innerHTML = state.actions.map((a, i) => `
    <div class="action-item">
      <span class="action-icon">${a.icon}</span>
      <div class="action-body">
        <div class="action-title">${escHtml(a.title)}</div>
        <div class="action-detail">${escHtml(a.detail)}</div>
      </div>
      <button class="action-apply" data-idx="${i}">APPLY</button>
    </div>`).join('');

  // Wire up apply buttons
  el.querySelectorAll('.action-apply').forEach(btn => {
    btn.addEventListener('click', () => openModal(parseInt(btn.dataset.idx)));
  });
}

//  MODAL ─
function openModal(idx) {
  const action = state.actions[idx];
  if (!action) return;
  state.pendingAction = action;

  document.getElementById('modal-service-name').textContent =
    action.svc || (action.event && action.event.service_id) || 'System';
  document.getElementById('modal-action-desc').textContent = action.title;

  // Evidence block
  const ev  = (action.event && action.event.evidence) || {};
  const evEl = document.getElementById('modal-evidence');
  const kvs  = [
    ['Utilisation',   ev.utilisation   != null ? pct(ev.utilisation)    : null],
    ['Collapse Risk', ev.collapse_risk != null ? pct(ev.collapse_risk)  : null],
    ['Queue Wait',    ev.queue_wait_ms != null ? fmt(ev.queue_wait_ms, 1) + 'ms' : null],
    ['Burst Factor',  ev.burst_factor  != null ? fmt(ev.burst_factor, 2) + '×'  : null],
    ['Cascade Risk',  ev.cascade_risk  != null ? pct(ev.cascade_risk)   : null],
    ['Stab. Margin',  ev.stability_margin != null ? pct(ev.stability_margin) : null],
  ].filter(([, v]) => v !== null);

  evEl.innerHTML = kvs.map(([k, v]) =>
    `<div class="ev-kv"><span class="ev-key">${k}</span><span class="ev-val">${v}</span></div>`
  ).join('') || '<span style="color:var(--text-dim)">No evidence data</span>';

  document.getElementById('modal-overlay').classList.remove('hidden');
}

function closeModal() {
  document.getElementById('modal-overlay').classList.add('hidden');
  state.pendingAction = null;
}

document.getElementById('modal-close').addEventListener('click', closeModal);
document.getElementById('modal-deny').addEventListener('click', closeModal);
document.getElementById('modal-overlay').addEventListener('click', e => {
  if (e.target === document.getElementById('modal-overlay')) closeModal();
});

document.getElementById('modal-approve').addEventListener('click', () => {
  const a = state.pendingAction;
  if (!a) { closeModal(); return; }
  logLine('info', `[action] APPROVED: ${a.title}`);
  pushEventRowDirect('INFO', null, `Operator approved: ${a.title}`);
  closeModal();
  // NOTE: actual actuation wire-up connects to POST /api/v1/actuate
  // For now we log the intent — backend integration point here.
});

//  DEBUG PANEL ─
function updateDebug(payload) {
  const tab = document.getElementById('debug-tab');
  if (!tab.open) return;

  // Fixed point
  const fp = payload.fixed_point_equilibrium || {};
  renderKV('debug-fp', {
    'Converged':         fp.converged ? 'YES' : 'NO',
    'Convergence Rate':  fmt(fp.convergence_rate, 3),
    'Stability Margin':  fmt(fp.stability_margin, 3),
    'Systemic Collapse': pct(fp.systemic_collapse_prob || 0),
    'Iterations':        fp.converged_iterations,
  });

  // Network equilibrium
  const neq = payload.network_equilibrium || {};
  renderKV('debug-neq', {
    'System ρ Mean':    pct(neq.system_rho_mean || 0),
    'System ρ Var':     fmt(neq.system_rho_variance || 0, 4),
    'Eq. Delta':        fmt(neq.equilibrium_delta || 0, 4),
    'Converging':       neq.is_converging ? 'YES' : 'NO',
    'Critical Svc':     neq.critical_service_id || '—',
    'Network Sat Risk': pct(neq.network_saturation_risk || 0),
  });

  // Stability envelope
  const env = payload.stability_envelope || {};
  renderKV('debug-env', {
    'Safe ρ Max':       pct(env.safe_system_rho_max || 0),
    'Current ρ Mean':   pct(env.current_system_rho_mean || 0),
    'Headroom':         pct(env.envelope_headroom || 0),
    'Most Vulnerable':  env.most_vulnerable_service || '—',
    'Worst Pert.Δ':     fmt(env.worst_perturbation_delta || 0, 4),
  });

  // Runtime metrics
  const rm = payload.runtime_metrics || {};
  renderKV('debug-rt', {
    'Prune ms':         fmt(rm.avg_prune_ms,     2),
    'Windows ms':       fmt(rm.avg_windows_ms,   2),
    'Topology ms':      fmt(rm.avg_topology_ms,  2),
    'Modelling ms':     fmt(rm.avg_modelling_ms, 2),
    'Optimise ms':      fmt(rm.avg_optimise_ms,  2),
    'Sim ms':           fmt(rm.avg_sim_ms,       2),
    'Reasoning ms':     fmt(rm.avg_reasoning_ms, 2),
    'Overruns':         rm.total_overruns,
    'Consec Overruns':  rm.consec_overruns,
  });

  // Raw payload (truncated)
  const raw = document.getElementById('debug-raw');
  raw.textContent = JSON.stringify(payload, null, 2).slice(0, 4000);
}

function renderKV(id, obj) {
  const el = document.getElementById(id);
  el.innerHTML = Object.entries(obj).map(([k, v]) =>
    `<div><span class="k">${k}</span><span class="v">${v ?? '—'}</span></div>`
  ).join('');
}

//  LOG TERMINAL 
const logPause = document.getElementById('log-pause');
document.getElementById('log-clear').addEventListener('click', () => {
  document.getElementById('log-terminal').innerHTML = '';
});

function logLine(level, msg) {
  if (logPause.checked) return;
  const term = document.getElementById('log-terminal');
  const line = document.createElement('p');
  line.className = `log-line ${level}`;
  const ts   = new Date().toLocaleTimeString('en-GB', { hour12: false });
  line.textContent = `${ts}  ${msg}`;
  term.appendChild(line);
  term.scrollTop = term.scrollHeight;
  while (term.children.length > MAX_LOG_LINES) {
    term.removeChild(term.firstChild);
  }
}

function logTick(payload, agg) {
  const cp  = payload.control_plane || {};
  const tick = cp.tick || 0;
  const rm  = payload.runtime_metrics || {};
  const health = payload.tick_health_ms;
  logLine('info',
    `tick=${tick}  rps=${fmt(agg.totalRps,1)}/s  ρ=${pct(agg.maxRho)}  risk=${pct(agg.maxRisk)}  lat=${fmt(agg.maxLat,1)}ms  tick_ms=${fmt(health,1)}`
  );
}

function pushEventRowDirect(sev, svc, msg) {
  const stream = document.getElementById('event-stream');
  const sevClass = sev === 'CRIT' ? 'crit' : sev === 'WARN' ? 'warn' : 'info';
  const ts       = new Date().toLocaleTimeString('en-GB', { hour12: false });
  const svcTag   = svc ? `<span class="svc-tag">[${escHtml(svc)}]</span> ` : '';

  const row = document.createElement('div');
  row.className = 'event-row';
  row.innerHTML = `
    <span class="event-ts">${ts}</span>
    <span class="event-sev ${sevClass}">${sev}</span>
    <span class="event-msg">${svcTag}${escHtml(msg)}</span>`;
  stream.prepend(row);
  while (stream.children.length > MAX_EVENTS) stream.removeChild(stream.lastChild);
}

//  UTILITIES ─
function fmt(v, decimals = 2) {
  if (v == null || !isFinite(v)) return '—';
  return Number(v).toFixed(decimals);
}

function pct(v) {
  if (v == null || !isFinite(v)) return '—';
  return (v * 100).toFixed(1) + '%';
}

function escHtml(s) {
  if (!s) return '';
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function formatTs(ts) {
  if (!ts) return new Date().toLocaleTimeString('en-GB', { hour12: false });
  const d = new Date(ts);
  if (isNaN(d)) return '—';
  return d.toLocaleTimeString('en-GB', { hour12: false });
}

//  INIT 
startClock();
connect();

// Handle window resize for sparklines
window.addEventListener('resize', () => {
  if (state.lastPayload) {
    const b   = state.lastPayload.bundles || {};
    const agg = aggregateMetrics(b, state.lastPayload);
    drawAllSparklines(agg);
  }
});