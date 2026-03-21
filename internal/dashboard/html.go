package dashboard

var dashboardHTML = []byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>LoadEquilibrium — Infrastructure Intelligence</title>
<style>
:root{
  --bg:#060c18;--surf:#0b1422;--surf2:#101d2e;--surf3:#0e1929;--border:#182440;--border2:#1e2e4a;
  --accent:#00d4ff;--accent2:#0096cc;--purple:#7c3aed;--green:#10b981;--teal:#06b6d4;
  --warn:#f59e0b;--crit:#ef4444;--ok:#10b981;--text:#e2e8f0;--muted:#4a6080;
  --font:'Courier New',monospace;--nav-w:48px;
}
*{box-sizing:border-box;margin:0;padding:0}
html,body{height:100%;overflow:hidden;background:var(--bg);color:var(--text);font-family:var(--font);font-size:12px}
body{display:grid;grid-template-rows:44px 1fr;grid-template-columns:var(--nav-w) 1fr}

/* ── Header ──────────────────────────────────────── */
header{grid-column:1/3;display:flex;align-items:center;padding:0 12px;gap:12px;
  background:var(--surf);border-bottom:1px solid var(--border);z-index:10}
.logo{font-size:12px;font-weight:700;letter-spacing:3px;color:var(--accent);text-transform:uppercase;white-space:nowrap;margin-right:4px}
.logo span{color:var(--muted)}
.hs{display:flex;flex-direction:column;align-items:center;min-width:60px}
.hs .l{font-size:8px;color:var(--muted);text-transform:uppercase;letter-spacing:.8px}
.hs .v{font-size:12px;font-weight:600;transition:color .3s}
.badge{display:inline-block;padding:1px 6px;border-radius:3px;font-size:9px;font-weight:700;letter-spacing:.8px}
.b-ok{background:rgba(16,185,129,.12);color:var(--ok);border:1px solid var(--ok)}
.b-warn{background:rgba(245,158,11,.12);color:var(--warn);border:1px solid var(--warn)}
.b-crit{background:rgba(239,68,68,.12);color:var(--crit);border:1px solid var(--crit)}
.spacer{flex:1}
.conn{display:flex;align-items:center;gap:5px;font-size:9px;color:var(--muted)}
.dot{width:7px;height:7px;border-radius:50%;background:var(--crit);transition:background .3s}
.dot.live{background:var(--ok);box-shadow:0 0 6px var(--ok)}
#safety-bar{height:2px;position:absolute;top:44px;left:0;right:0;z-index:20;transition:background .5s}

/* ── Sidebar nav ─────────────────────────────────── */
nav{grid-row:2;background:var(--surf);border-right:1px solid var(--border);
  display:flex;flex-direction:column;align-items:center;padding:6px 0;gap:2px}
.nb{width:40px;height:40px;border-radius:6px;display:flex;flex-direction:column;align-items:center;
  justify-content:center;cursor:pointer;transition:background .15s,color .15s;
  color:var(--muted);border:1px solid transparent;font-size:7px;gap:2px;text-transform:uppercase;letter-spacing:.5px}
.nb span:first-child{font-size:14px;line-height:1}
.nb:hover{background:var(--surf2);color:var(--text)}
.nb.active{background:rgba(0,212,255,.08);color:var(--accent);border-color:rgba(0,212,255,.2)}
.nb-sep{width:28px;height:1px;background:var(--border);margin:3px 0}

/* ── Main content area ───────────────────────────── */
#content{grid-row:2;overflow:hidden;position:relative}
.view{position:absolute;inset:0;overflow:hidden;display:none}
.view.active{display:flex}

/* ── Panel primitives ────────────────────────────── */
.panel{background:var(--surf);display:flex;flex-direction:column;overflow:hidden;border:1px solid var(--border)}
.pt{padding:6px 10px;font-size:8px;text-transform:uppercase;letter-spacing:1.5px;
  color:var(--muted);border-bottom:1px solid var(--border);flex-shrink:0;
  display:flex;align-items:center;gap:6px;white-space:nowrap}
.pb{flex:1;overflow:hidden;position:relative}
.scrollable{overflow-y:auto;height:100%}

/* ── Service list ───────────────────────────────── */
.slist{padding:6px}
.si{padding:6px 8px;border:1px solid var(--border);border-radius:4px;margin-bottom:4px;
  cursor:pointer;transition:border-color .2s,background .2s}
.si:hover,.si.sel{border-color:var(--accent);background:rgba(0,212,255,.04)}
.sn{font-size:10px;font-weight:600;margin-bottom:3px;display:flex;justify-content:space-between;align-items:center}
.sm{display:flex;gap:4px;flex-wrap:wrap}
.mc{display:flex;flex-direction:column;align-items:center;min-width:38px}
.mc .ml{font-size:7px;color:var(--muted);text-transform:uppercase}
.mc .mv{font-size:9px;font-weight:600}
.rbar{width:100%;height:2px;background:var(--border);border-radius:1px;margin-top:3px;overflow:hidden}
.rf{height:100%;border-radius:1px;transition:width .5s,background .5s}

/* ── Stability gauges ───────────────────────────── */
.sgrid{display:grid;grid-template-columns:repeat(auto-fill,minmax(110px,1fr));gap:5px;padding:6px;overflow-y:auto;height:100%}
.sc{border:1px solid var(--border);border-radius:4px;padding:5px}
.scn{font-size:8px;color:var(--muted);margin-bottom:3px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.gr{display:flex;justify-content:center;align-items:center;position:relative}
.gr svg{transform:rotate(-90deg)}
.gv{position:absolute;text-align:center;font-size:10px;font-weight:700}
.gs{font-size:7px;color:var(--muted)}

/* ── Charts ─────────────────────────────────────── */
.cw{width:100%;height:100%;display:flex;flex-direction:column;gap:3px;padding:6px}
.cr{flex:1;display:flex;flex-direction:column;min-height:0}
.cl{font-size:8px;color:var(--muted);text-transform:uppercase;letter-spacing:.8px;margin-bottom:1px;flex-shrink:0}

/* ── Console ─────────────────────────────────────── */
.console{height:100%;overflow-y:auto;padding:5px 7px}
.cl2{padding:3px 5px;border-left:2px solid transparent;margin-bottom:2px;border-radius:2px;animation:fi .2s ease}
@keyframes fi{from{opacity:0;transform:translateX(-3px)}to{opacity:1;transform:none}}
.cl2.INFO{border-color:var(--muted);color:var(--muted)}
.cl2.WARN{border-color:var(--warn);color:var(--warn);background:rgba(245,158,11,.04)}
.cl2.CRIT{border-color:var(--crit);color:var(--crit);background:rgba(239,68,68,.06)}
.cts{font-size:8px;opacity:.6;margin-right:4px}
.ccat{font-size:8px;font-weight:700;margin-right:4px}
.cmsg{font-size:9px}
.crec{font-size:8px;color:var(--muted);margin-top:1px;padding-left:7px}
.cchain{font-size:7px;color:rgba(74,96,128,.8);margin-top:1px;padding-left:7px}
.prio-badge{display:inline-block;font-size:7px;padding:1px 3px;border-radius:2px;
  margin-right:3px;font-weight:700;background:rgba(124,58,237,.2);color:var(--purple)}
.mpc-badge{font-size:7px;padding:1px 3px;border-radius:2px;font-weight:700;margin-left:3px}
.mpc-over{background:rgba(245,158,11,.2);color:var(--warn)}
.mpc-under{background:rgba(239,68,68,.2);color:var(--crit)}

/* ── Overview KPI cards ─────────────────────────── */
.kpi-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(130px,1fr));gap:6px;padding:8px}
.kpi{background:var(--surf2);border:1px solid var(--border2);border-radius:6px;padding:10px 12px;
  display:flex;flex-direction:column;gap:3px;transition:border-color .3s}
.kpi.warn{border-color:var(--warn)}
.kpi.crit{border-color:var(--crit);background:rgba(239,68,68,.03)}
.kpi-label{font-size:8px;color:var(--muted);text-transform:uppercase;letter-spacing:1px}
.kpi-val{font-size:22px;font-weight:700;line-height:1;transition:color .3s}
.kpi-sub{font-size:8px;color:var(--muted)}

/* ── Metric bar ──────────────────────────────────── */
.mbar-wrap{display:flex;align-items:center;gap:6px;margin-bottom:4px}
.mbar-label{font-size:9px;color:var(--muted);width:80px;flex-shrink:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.mbar-track{flex:1;height:4px;background:var(--border);border-radius:2px;overflow:hidden}
.mbar-fill{height:100%;border-radius:2px;transition:width .5s,background .5s}
.mbar-val{font-size:9px;font-weight:600;width:38px;text-align:right;flex-shrink:0}

/* ── Sim panel ───────────────────────────────────── */
.sim-badge{position:absolute;bottom:6px;right:8px;font-size:8px;padding:2px 6px;
  border-radius:3px;font-weight:700;letter-spacing:1px;pointer-events:none}

/* ── Optimisation table ──────────────────────────── */
.opt-table{width:100%;border-collapse:collapse;font-size:9px}
.opt-table th{font-size:8px;color:var(--muted);text-transform:uppercase;letter-spacing:.5px;
  border-bottom:1px solid var(--border);padding:4px 6px;text-align:left;font-weight:normal}
.opt-table td{padding:4px 6px;border-bottom:1px solid var(--border2)}
.opt-table tr:hover td{background:var(--surf2)}

/* ── Risk timeline ───────────────────────────────── */
.rtl-row{display:flex;align-items:center;gap:4px;margin-bottom:3px;min-height:16px}
.rtl-id{font-size:8px;width:80px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--muted);flex-shrink:0}
.rtl-seg{flex:1;height:6px;border-radius:1px;position:relative;overflow:hidden;background:var(--border)}

/* ── Telemetry ───────────────────────────────────── */
.fresh-dot{width:6px;height:6px;border-radius:50%;display:inline-block;margin-right:4px}

/* ── Scrollbar ───────────────────────────────────── */
::-webkit-scrollbar{width:3px;height:3px}
::-webkit-scrollbar-track{background:var(--surf)}
::-webkit-scrollbar-thumb{background:var(--border);border-radius:2px}
</style>
</head>
<body>

<div id="safety-bar"></div>

<header>
  <div class="logo">Load<span>Equilibrium</span></div>
  <div class="hs"><span class="l">Services</span><span class="v" id="h-svc">—</span></div>
  <div class="hs"><span class="l">Obj Score</span><span class="v" id="h-obj">—</span></div>
  <div class="hs"><span class="l">Cascade</span><span class="v" id="h-cas">—</span></div>
  <div class="hs"><span class="l">P99 Est</span><span class="v" id="h-p99">—</span></div>
  <div class="hs"><span class="l">Tick ms</span><span class="v" id="h-tick" style="font-size:10px">—</span></div>
  <div class="hs"><span class="l">dRisk/dt</span><span class="v" id="h-raccel" style="font-size:10px">—</span></div>
  <div class="hs"><span class="l">Safety</span><span class="v" id="h-safety" style="font-size:9px">OK</span></div>
  <div class="hs"><span class="l">Net Equil</span><span class="v" id="h-equil" style="font-size:9px">—</span></div>
  <div class="hs"><span class="l">Traj Cost</span><span class="v" id="h-traj" style="font-size:10px">—</span></div>
  <div class="hs"><span class="l">FP Collapse</span><span class="v" id="h-fp" style="font-size:9px">—</span></div>
  <div class="hs"><span class="l">System</span><span class="badge b-ok" id="h-sys">NOMINAL</span></div>
  <div class="spacer"></div>
  <div class="conn"><div class="dot" id="dot"></div><span id="clabel">CONNECTING</span></div>
</header>

<nav id="nav">
  <div class="nb active" data-view="overview" onclick="switchView('overview')"><span>⬡</span>Over</div>
  <div class="nb" data-view="topology" onclick="switchView('topology')"><span>⬢</span>Topo</div>
  <div class="nb" data-view="stability" onclick="switchView('stability')"><span>◎</span>Stab</div>
  <div class="nb" data-view="prediction" onclick="switchView('prediction')"><span>▲</span>Pred</div>
  <div class="nb-sep"></div>
  <div class="nb" data-view="optimisation" onclick="switchView('optimisation')"><span>⚙</span>Opt</div>
  <div class="nb" data-view="simulation" onclick="switchView('simulation')"><span>⚃</span>Sim</div>
  <div class="nb-sep"></div>
  <div class="nb" data-view="telemetry" onclick="switchView('telemetry')"><span>◈</span>Telem</div>
  <div class="nb" data-view="reasoning" onclick="switchView('reasoning')"><span>❯</span>Log</div>
  <div class="nb" data-view="health" onclick="switchView('health')"><span>♥</span>Health</div>
</nav>

<div id="content">

<!-- ═══ OVERVIEW ════════════════════════════════════════════════════════════ -->
<div class="view active" id="view-overview" style="flex-direction:column;gap:1px;background:var(--border)">
  <div style="display:flex;gap:1px;flex:0 0 auto">
    <div class="panel" style="flex:1">
      <div class="pt"><span>⬡</span> System KPIs</div>
      <div class="pb scrollable"><div id="kpi-grid" class="kpi-grid"></div></div>
    </div>
    <div class="panel" style="width:320px">
      <div class="pt"><span>▲</span> Risk Priority Queue</div>
      <div class="pb scrollable"><div id="ov-riskqueue" style="padding:6px"></div></div>
    </div>
  </div>
  <div style="display:flex;gap:1px;flex:1;min-height:0">
    <div class="panel" style="flex:1">
      <div class="pt"><span>◈</span> Services</div>
      <div class="pb"><div class="scrollable slist" id="slist"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>⬡</span> Topology</div>
      <div class="pb" style="padding:0">
        <canvas id="topo"></canvas>
        <div class="sim-badge" id="sim-badge" style="display:none"></div>
      </div>
    </div>
  </div>
</div>

<!-- ═══ TOPOLOGY ════════════════════════════════════════════════════════════ -->
<div class="view" id="view-topology" style="flex-direction:column;gap:1px;background:var(--border)">
  <div style="display:flex;gap:1px;flex:1;min-height:0">
    <div class="panel" style="width:220px">
      <div class="pt"><span>◈</span> Service Pressure</div>
      <div class="pb scrollable"><div id="tp-slist" style="padding:6px"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>⬢</span> Dependency Graph — Heatmap</div>
      <div class="pb" style="padding:0"><canvas id="topo2"></canvas></div>
    </div>
    <div class="panel" style="width:220px">
      <div class="pt"><span>⬡</span> Network Equilibrium</div>
      <div class="pb scrollable"><div id="tp-equil" style="padding:6px"></div></div>
    </div>
  </div>
</div>

<!-- ═══ STABILITY ═══════════════════════════════════════════════════════════ -->
<div class="view" id="view-stability" style="flex-direction:column;gap:1px;background:var(--border)">
  <div style="display:flex;gap:1px;flex:0 0 180px">
    <div class="panel" style="flex:1">
      <div class="pt"><span>◎</span> Equilibrium Stability Envelope</div>
      <div class="pb scrollable"><div id="stab-envelope" style="padding:8px"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>⬡</span> Fixed-Point Solver State</div>
      <div class="pb scrollable"><div id="stab-fp" style="padding:8px"></div></div>
    </div>
  </div>
  <div style="display:flex;gap:1px;flex:1;min-height:0">
    <div class="panel" style="flex:2">
      <div class="pt"><span>◎</span> Per-Service Stability Gauges</div>
      <div class="pb"><div class="sgrid" id="sgrid"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>▲</span> Perturbation Sensitivity</div>
      <div class="pb scrollable"><div id="stab-perturb" style="padding:8px"></div></div>
    </div>
  </div>
</div>

<!-- ═══ PREDICTION ══════════════════════════════════════════════════════════ -->
<div class="view" id="view-prediction" style="flex-direction:column;gap:1px;background:var(--border)">
  <div style="display:flex;gap:1px;flex:1;min-height:0">
    <div class="panel" style="width:200px">
      <div class="pt"><span>◈</span> Select Service</div>
      <div class="pb scrollable"><div id="pred-slist" style="padding:6px"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>▲</span> Utilisation + Prediction CI</div>
      <div class="pb"><div class="cw" id="cw"></div></div>
    </div>
    <div class="panel" style="width:240px">
      <div class="pt"><span>⚡</span> Risk Timeline Runway</div>
      <div class="pb scrollable"><div id="pred-rtl" style="padding:6px"></div></div>
    </div>
  </div>
</div>

<!-- ═══ OPTIMISATION ════════════════════════════════════════════════════════ -->
<div class="view" id="view-optimisation" style="flex-direction:column;gap:1px;background:var(--border)">
  <div style="display:flex;gap:1px;flex:0 0 160px">
    <div class="panel" style="flex:1">
      <div class="pt"><span>⚙</span> Global Objective</div>
      <div class="pb scrollable"><div id="opt-obj" style="padding:8px"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>⚙</span> Control Trajectory</div>
      <div class="pb scrollable"><div id="opt-traj" style="padding:8px"></div></div>
    </div>
  </div>
  <div style="display:flex;gap:1px;flex:1;min-height:0">
    <div class="panel" style="flex:1">
      <div class="pt"><span>⚙</span> Control Directives</div>
      <div class="pb scrollable"><div id="opt-directives" style="padding:6px"></div></div>
    </div>
  </div>
</div>

<!-- ═══ SIMULATION ══════════════════════════════════════════════════════════ -->
<div class="view" id="view-simulation" style="flex-direction:column;gap:1px;background:var(--border)">
  <div style="display:flex;gap:1px;flex:0 0 140px">
    <div class="panel" style="flex:1">
      <div class="pt"><span>⚃</span> Monte-Carlo Scenario Comparison</div>
      <div class="pb scrollable"><div id="sim-scenarios" style="padding:8px"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>⚡</span> Cascade Failure Distribution</div>
      <div class="pb scrollable"><div id="sim-cascade" style="padding:8px"></div></div>
    </div>
  </div>
  <div style="display:flex;gap:1px;flex:1;min-height:0">
    <div class="panel" style="flex:1">
      <div class="pt"><span>◎</span> Queue Distribution at Horizon</div>
      <div class="pb scrollable"><div id="sim-qdist" style="padding:6px"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>▲</span> SLA Violation Probability</div>
      <div class="pb scrollable"><div id="sim-sla" style="padding:6px"></div></div>
    </div>
  </div>
</div>

<!-- ═══ TELEMETRY ════════════════════════════════════════════════════════════ -->
<div class="view" id="view-telemetry" style="flex-direction:column;gap:1px;background:var(--border)">
  <div style="display:flex;gap:1px;flex:1;min-height:0">
    <div class="panel" style="flex:1">
      <div class="pt"><span>◈</span> Signal Freshness &amp; Confidence</div>
      <div class="pb scrollable"><div id="telem-freshness" style="padding:6px"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>▲</span> Arrival Rates</div>
      <div class="pb scrollable"><div id="telem-rates" style="padding:6px"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>⚡</span> Degraded &amp; Missing Signals</div>
      <div class="pb scrollable"><div id="telem-degraded" style="padding:6px"></div></div>
    </div>
  </div>
</div>

<!-- ═══ REASONING ═══════════════════════════════════════════════════════════ -->
<div class="view" id="view-reasoning" style="flex-direction:column;background:var(--bg)">
  <div class="panel" style="flex:1">
    <div class="pt"><span>❯</span> Causal Reasoning Stream — Live</div>
    <div class="pb"><div class="console" id="console"></div></div>
  </div>
</div>

<!-- ═══ SYSTEM HEALTH ════════════════════════════════════════════════════════ -->
<div class="view" id="view-health" style="flex-direction:column;gap:1px;background:var(--border)">
  <div style="display:flex;gap:1px;flex:0 0 200px">
    <div class="panel" style="flex:1">
      <div class="pt"><span>♥</span> Runtime Stage Latencies</div>
      <div class="pb scrollable"><div id="health-stages" style="padding:8px"></div></div>
    </div>
    <div class="panel" style="flex:1">
      <div class="pt"><span>⚡</span> Safety Escalation State</div>
      <div class="pb scrollable"><div id="health-safety" style="padding:8px"></div></div>
    </div>
  </div>
  <div style="display:flex;gap:1px;flex:1;min-height:0">
    <div class="panel" style="flex:1">
      <div class="pt"><span>◎</span> Overrun &amp; Budget Metrics</div>
      <div class="pb scrollable"><div id="health-overrun" style="padding:8px"></div></div>
    </div>
  </div>
</div>

</div><!-- #content -->

<script>
const MAX_H=60;
const state={
  svcs:{}, hist:{}, topo:null, obj:null, events:[],
  sel:null, simResult:null, satCountdowns:{}, stabilityZones:{},
  predTimeline:{}, directives:{}, degraded:[],
  tickHealthMs:0, degradedFraction:0, netCoupling:{},
  safetyMode:false, jitterMs:0, runtimeMetrics:{}, netEquilibrium:{}, topoSensitivity:{},
  riskQueue:[], pressureHeatmap:{}, fixedPointEquil:{}, scenarioComp:null,
  riskTimeline:{}, stabilityEnvelope:{}
};
let topoPos={}, topoVel={}, prevTopoNodes={}, prevTopoEdges={};

// WebSocket
let ws=null, reconnTimer=null;
function connect(){
  const proto=location.protocol==='https:'?'wss:':'ws:';
  ws=new WebSocket(proto+'//'+location.host+'/ws');
  ws.onopen=()=>{
    el('dot').classList.add('live');
    el('clabel').textContent='LIVE';
    clearTimeout(reconnTimer);
  };
  ws.onclose=()=>{
    el('dot').classList.remove('live');
    el('clabel').textContent='RECONNECT';
    reconnTimer=setTimeout(connect,3000);
  };
  ws.onerror=()=>ws.close();
  ws.onmessage=ev=>{
    try{const m=JSON.parse(ev.data);if(m.type==='tick')tick(m);}catch(_){}
  };
}

function tick(m){
  state.topo=m.topology;
  state.obj=m.objective;
  state.events=(m.events||[]).slice(0,80);
  state.satCountdowns=m.sat_countdowns||{};
  state.stabilityZones=m.stability_zones||{};
  state.predTimeline=m.prediction_timeline||{};
  state.directives=m.directives||{};
  state.degraded=m.degraded_services||[];
  state.tickHealthMs=m.tick_health_ms||0;
  state.degradedFraction=m.degraded_fraction||0;
  state.netCoupling=m.network_coupling||{};
  state.safetyMode=m.safety_mode||false;
  state.jitterMs=m.jitter_ms||0;
  state.runtimeMetrics=m.runtime_metrics||{};
  state.netEquilibrium=m.network_equilibrium||{};
  state.topoSensitivity=m.topology_sensitivity||{};
  state.riskQueue=m.priority_risk_queue||[];
  state.pressureHeatmap=m.pressure_heatmap||{};
  state.fixedPointEquil=m.fixed_point_equilibrium||{};
  state.scenarioComp=m.scenario_comparison||null;
  state.riskTimeline=m.risk_timeline||{};
  state.stabilityEnvelope=m.stability_envelope||{};
  if(m.sim_result)state.simResult=m.sim_result;

  // Apply topology diff if not full snapshot
  if(m.topo_diff){
    applyTopoDiff(m.topo_diff, m.topology);
  }

  const ids=Object.keys(m.bundles||{});
  ids.forEach(id=>{
    const b=m.bundles[id];
    state.svcs[id]=b;
    if(!state.hist[id])state.hist[id]=[];
    state.hist[id].push({
      rho:      b.queue&&b.queue.utilisation||0,
      lat:      b.queue&&b.queue.mean_sojourn_ms||0,
      arr:      b.queue&&b.queue.arrival_rate||0,
      osc:      b.stability&&b.stability.oscillation_risk||0,
      collapse: b.stability&&b.stability.collapse_risk||0,
    });
    if(state.hist[id].length>MAX_H)state.hist[id].shift();
  });
  Object.keys(state.svcs).forEach(id=>{if(!m.bundles[id])delete state.svcs[id];});

  renderHeader(); renderList(); renderStability(); renderCharts(); renderConsole(); renderSim(); renderRuntimeMetrics();
  renderScenarioComparison(); renderSafetyBar(); renderEquilibriumProximity(); renderCascadeDistribution();
  // Tab-specific renders — only run when tab is visible to save CPU.
  const av=document.querySelector('.view.active');
  if(av){
    const v=av.id.replace('view-','');
    if(v==='overview')renderOverview();
    if(v==='topology')renderTopologyTab();
    if(v==='stability')renderStabilityTab();
    if(v==='prediction')renderPredictionTab();
    if(v==='optimisation')renderOptimisationTab();
    if(v==='simulation')renderSimulationTab();
    if(v==='telemetry')renderTelemetryTab();
    if(v==='health')renderHealthTab();
  }
}

function applyTopoDiff(diff, fullSnap){
  if(diff.is_full){
    // Full snapshot — rebuild node/edge indices.
    prevTopoNodes={};prevTopoEdges={};
    (diff.added_nodes||[]).forEach(n=>prevTopoNodes[n.service_id]=n);
    (diff.added_edges||[]).forEach(e=>prevTopoEdges[e.source+':'+e.target]=e);
    return;
  }
  (diff.added_nodes||[]).forEach(n=>prevTopoNodes[n.service_id]=n);
  (diff.updated_nodes||[]).forEach(n=>prevTopoNodes[n.service_id]=n);
  (diff.removed_nodes||[]).forEach(id=>delete prevTopoNodes[id]);
  (diff.added_edges||[]).forEach(e=>prevTopoEdges[e.source+':'+e.target]=e);
  (diff.updated_edges||[]).forEach(e=>prevTopoEdges[e.source+':'+e.target]=e);
  (diff.removed_edges||[]).forEach(k=>delete prevTopoEdges[k]);
}

function renderHeader(){
  const o=state.obj;if(!o)return;
  const score=o.composite_score||0;
  el('h-svc').textContent=Object.keys(state.svcs).length;
  el('h-obj').textContent=pct(score);
  el('h-cas').textContent=pct(o.cascade_failure_probability);
  el('h-p99').textContent=ms(o.predicted_p99_latency_ms);
  el('h-osc').textContent=pct(o.oscillation_risk);
  el('h-obj').style.color=score>.8?'var(--crit)':score>.5?'var(--warn)':'var(--ok)';
  const badge=el('h-sys');
  badge.className='badge '+(score>.8?'b-crit':score>.5?'b-warn':'b-ok');
  badge.textContent=score>.8?'CRITICAL':score>.5?'DEGRADED':'NOMINAL';
  // Safety mode indicator
  const smEl=el('h-safety');
  if(smEl){
    smEl.textContent=state.safetyMode?'⚠ SAFETY':'OK';
    smEl.style.color=state.safetyMode?'var(--crit)':'var(--muted)';
  }
  // Network equilibrium convergence
  const neq=state.netEquilibrium;
  const eqEl=el('h-equil');
  if(eqEl&&neq){
    const converging=neq.is_converging;
    eqEl.textContent=(converging?'↓ CONV':'↑ DIV')+(neq.network_saturation_risk>0?' '+pct(neq.network_saturation_risk):'');
    eqEl.style.color=neq.network_saturation_risk>.6?'var(--crit)':converging?'var(--ok)':'var(--warn)';
  }
  // Tick health indicator with predictive overrun warning
  const hEl=el('h-tick');
  if(hEl){
    const hMs=state.tickHealthMs;
    const predicted=state.runtimeMetrics&&state.runtimeMetrics.predicted_overrun;
    hEl.textContent=hMs.toFixed(0)+'ms'+(predicted?' ⚡':'');
    hEl.style.color=hMs>1500||predicted?'var(--crit)':hMs>800?'var(--warn)':'var(--muted)';
    hEl.title='jitter: '+(state.jitterMs||0).toFixed(1)+'ms | predicted: '+((state.runtimeMetrics&&state.runtimeMetrics.predicted_critical_ms)||0).toFixed(0)+'ms | overruns: '+((state.runtimeMetrics&&state.runtimeMetrics.total_overruns)||0);
  }
  // Risk acceleration
  const raEl=el('h-raccel');
  if(raEl&&o.risk_acceleration!=null){
    const ra=o.risk_acceleration||0;
    raEl.textContent=ra>0?'+'+ra.toFixed(3):ra.toFixed(3);
    raEl.style.color=ra>0.05?'var(--crit)':ra>0.01?'var(--warn)':'var(--ok)';
  }
  // Trajectory score: risk-latency cost of control path over prediction horizon
  const tsEl=el('h-traj');
  if(tsEl){
    const ts=o.trajectory_score||0;
    tsEl.textContent=pct(ts);
    tsEl.style.color=ts>.7?'var(--crit)':ts>.4?'var(--warn)':'var(--ok)';
    tsEl.title='Risk-latency cost of current control trajectory (0=safe, 1=critical)';
  }
  // Fixed-point systemic collapse probability from Gauss-Seidel solver
  const fpEl=el('h-fp');
  if(fpEl){
    const fp=state.fixedPointEquil||{};
    const fpCollapse=fp.systemic_collapse_prob||0;
    const fpConverged=fp.converged;
    fpEl.textContent=pct(fpCollapse)+(fpConverged?'':'*');
    fpEl.style.color=fpCollapse>.5?'var(--crit)':fpCollapse>.2?'var(--warn)':'var(--muted)';
    fpEl.title='Fixed-point systemic P(collapse)'+(fpConverged?'':' — solver not converged')+' | iters: '+(fp.converged_iterations||0);
  }
}

function renderList(){
  const list=el('slist');
  // Order services by risk queue urgency (highest first); fallback to alphabetical.
  const rqMap={};
  (state.riskQueue||[]).forEach((item,i)=>{rqMap[item.service_id]=i;});
  const ids=Object.keys(state.svcs).sort((a,b)=>{
    const ra=rqMap[a]!=null?rqMap[a]:999;
    const rb=rqMap[b]!=null?rqMap[b]:999;
    return ra-rb;
  });
  ids.forEach(id=>{
    const b=state.svcs[id];
    const q=b.queue||{}, st=b.stability||{};
    const rho=q.utilisation||0;
    const col=rhoColor(rho);
    const isDegraded=state.degraded.includes(id);
    let e=document.getElementById('si-'+id);
    if(!e){
      e=document.createElement('div');
      e.className='si';e.id='si-'+id;
      e.onclick=()=>{
        state.sel=id;
        document.querySelectorAll('.si').forEach(x=>x.classList.remove('sel'));
        e.classList.add('sel');renderCharts();
      };
      list.appendChild(e);
    }
    if(state.sel===id)e.classList.add('sel');

    // Saturation countdown
    let cdHTML='';
    const cd=state.satCountdowns[id];
    if(cd&&cd>0&&cd<300){
      const cdCol=cd<60?'var(--crit)':cd<120?'var(--warn)':'var(--ok)';
      cdHTML='<div class="countdown" style="color:'+cdCol+'">⚠ sat in '+cd.toFixed(0)+'s</div>';
    }

    const nc=state.netCoupling[id]||{};
    const pressure=state.pressureHeatmap[id]||rho;
    const sens=state.topoSensitivity&&state.topoSensitivity.by_service&&state.topoSensitivity.by_service[id]||{};
    const isKeystone2=sens.is_keystone||false;
    // Sim overlay: cascade failure probability from Monte-Carlo runs
    const simOverlay=state.simOverlay||{};
    const cfp=(simOverlay.cascade_failure_prob&&simOverlay.cascade_failure_prob[id])||0;
    const p95q=(simOverlay.p95_queue_len&&simOverlay.p95_queue_len[id])||0;
    const slaViolProb=(simOverlay.sla_violation_prob&&simOverlay.sla_violation_prob[id])||0;
    const simAge=simOverlay.sim_tick_age||0;
    const simFade=simAge>5?0.4:1.0; // fade stale overlay data
    let mpcHTML='';
    if(dir.mpc_overshoot_risk)mpcHTML+='<span class="mpc-badge mpc-over">MPC-OVER</span>';
    if(dir.mpc_underactuation_risk)mpcHTML+='<span class="mpc-badge mpc-under">MPC-LOW</span>';
    if(isKeystone2)mpcHTML+='<span class="mpc-badge" style="background:rgba(124,58,237,.2);color:var(--purple)">KEY</span>';
    const upPressure=nc.effective_pressure||0;
    const eqRho=nc.path_equilibrium_rho||0;
    const pathCP=nc.path_collapse_prob||0;
    const pressureChip=upPressure>.3?chip('UP',pct(upPressure),upPressure>.7?'var(--crit)':'var(--warn)'):'';
    const eqChip=eqRho>rho+0.05?chip('EQ',eqRho.toFixed(2),eqRho>.9?'var(--crit)':'var(--warn)'):'';
    const cpChip=pathCP>.3?'<div class="mc" style="opacity:'+simFade+'"><span class="ml">P(fail)</span><span class="mv" style="color:'+(pathCP>.6?'var(--crit)':'var(--warn)')+'">'+pct(pathCP)+'</span></div>':'';
    const cfpChip=cfp>.05?'<div class="mc" style="opacity:'+simFade+'"><span class="ml">MC</span><span class="mv" style="color:'+(cfp>.3?'var(--crit)':'var(--warn)')+'">'+pct(cfp)+'</span></div>':'';
    const slaChip=slaViolProb>.05?'<div class="mc" style="opacity:'+simFade+'"><span class="ml">SLA</span><span class="mv" style="color:'+(slaViolProb>.3?'var(--crit)':'var(--warn)')+'">'+pct(slaViolProb)+'</span></div>':'';
    // Fixed-point equilibrium rho for this service
    const fpRho=(state.fixedPointEquil&&state.fixedPointEquil.equilibrium_rho&&state.fixedPointEquil.equilibrium_rho[id])||0;
    const fpChip=fpRho>rho+0.05?'<div class="mc"><span class="ml">FP</span><span class="mv" style="color:'+(fpRho>.9?'var(--crit)':'var(--warn)')+'">'+fpRho.toFixed(2)+'</span></div>':'';
    const presCol=pressure>.8?'var(--crit)':pressure>.6?'var(--warn)':'var(--ok)';

    e.innerHTML=
      '<div class="sn"><span style="'+(isDegraded?'opacity:.5':'')+'">'+esc(id)+'</span>'+
        '<span style="font-size:8px;color:var(--muted)">c='+(q.concurrency||1).toFixed(0)+mpcHTML+'</span></div>'+
      '<div class="sm">'+
        chip('ρ',pct(rho),col)+
        chip('λ',fmt1(q.arrival_rate)+'/s','var(--text)')+
        chip('Wq',ms(q.adjusted_wait_ms),col)+
        chip('P',pct(pressure),presCol)+
        pressureChip+eqChip+cpChip+cfpChip+slaChip+fpChip+
      '</div>'+
      cdHTML+
      '<div class="rbar"><div class="rf" style="width:'+Math.min(pressure*100,100)+'%;background:'+presCol+'"></div></div>';
  });
  list.querySelectorAll('.si').forEach(e=>{
    const id=e.id.replace('si-','');
    if(!state.svcs[id])e.remove();
  });
  if(!state.sel&&ids.length>0){
    state.sel=ids[0];
    const f=document.getElementById('si-'+ids[0]);
    if(f)f.classList.add('sel');
  }
}

// ── Topology ────────────────────────────────────────────────────────────────
const topoCanvas=document.getElementById('topo');
const tCtx=topoCanvas.getContext('2d');
function renderTopo(){
  const par=topoCanvas.parentElement;
  topoCanvas.width=par.clientWidth;topoCanvas.height=par.clientHeight;
  const snap=state.topo;
  const W=topoCanvas.width,H=topoCanvas.height;
  if(!snap||!snap.nodes||snap.nodes.length===0){
    tCtx.fillStyle='rgba(74,96,128,.4)';tCtx.font='11px monospace';tCtx.textAlign='center';
    tCtx.fillText('Awaiting topology…',W/2,H/2);return;
  }
  const nodes=snap.nodes,edges=snap.edges||[];
  const critNodes=new Set((snap.critical_path&&snap.critical_path.nodes)||[]);

  nodes.forEach(n=>{
    if(!topoPos[n.service_id]){
      topoPos[n.service_id]={x:40+Math.random()*(W-80),y:30+Math.random()*(H-60)};
      topoVel[n.service_id]={x:0,y:0};
    }
  });

  for(let iter=0;iter<5;iter++){
    const f={};nodes.forEach(n=>{f[n.service_id]={x:0,y:0}});
    for(let i=0;i<nodes.length;i++)for(let j=i+1;j<nodes.length;j++){
      const a=nodes[i].service_id,b=nodes[j].service_id;
      const pa=topoPos[a],pb=topoPos[b];
      const dx=pa.x-pb.x,dy=pa.y-pb.y,d=Math.max(Math.sqrt(dx*dx+dy*dy),1);
      const ff=2800/(d*d);
      f[a].x+=dx/d*ff;f[a].y+=dy/d*ff;f[b].x-=dx/d*ff;f[b].y-=dy/d*ff;
    }
    edges.forEach(e=>{
      const pa=topoPos[e.source],pb=topoPos[e.target];if(!pa||!pb)return;
      const dx=pb.x-pa.x,dy=pb.y-pa.y,d=Math.max(Math.sqrt(dx*dx+dy*dy),1);
      const ideal=90+(1-e.weight)*50;const ff=(d-ideal)*0.04;
      f[e.source].x+=dx/d*ff;f[e.source].y+=dy/d*ff;
      f[e.target].x-=dx/d*ff;f[e.target].y-=dy/d*ff;
    });
    nodes.forEach(n=>{
      const id=n.service_id,p=topoPos[id],v=topoVel[id];
      v.x=(v.x+f[id].x)*0.55;v.y=(v.y+f[id].y)*0.55;
      p.x=Math.max(28,Math.min(W-28,p.x+v.x));
      p.y=Math.max(20,Math.min(H-20,p.y+v.y));
    });
  }

  tCtx.clearRect(0,0,W,H);
  // Draw pressure heatmap halos before edges and nodes.
  applyHeatmapToNodes(W,H,nodes);
  edges.forEach(e=>{
    const pa=topoPos[e.source],pb=topoPos[e.target];if(!pa||!pb)return;
    const onCrit=critNodes.has(e.source)&&critNodes.has(e.target);
    const alpha=.15+e.weight*.7;
    tCtx.beginPath();tCtx.moveTo(pa.x,pa.y);tCtx.lineTo(pb.x,pb.y);
    tCtx.strokeStyle=onCrit?'rgba(239,68,68,'+alpha+')':'rgba(0,212,255,'+alpha+')';
    tCtx.lineWidth=.8+e.weight*2.5;tCtx.stroke();
    const ang=Math.atan2(pb.y-pa.y,pb.x-pa.x);
    const ax=pb.x-Math.cos(ang)*13,ay=pb.y-Math.sin(ang)*13;
    tCtx.beginPath();tCtx.moveTo(ax,ay);
    tCtx.lineTo(ax-Math.cos(ang-.4)*6,ay-Math.sin(ang-.4)*6);
    tCtx.lineTo(ax-Math.cos(ang+.4)*6,ay-Math.sin(ang+.4)*6);
    tCtx.closePath();tCtx.fillStyle=tCtx.strokeStyle;tCtx.fill();
  });
  nodes.forEach(n=>{
    const p=topoPos[n.service_id];
    const b=state.svcs[n.service_id];
    const rho=b?(b.queue&&b.queue.utilisation||0):n.normalised_load||0;
    const nc=rhoColor(rho);const r=12+rho*7;
    if(rho>.70){
      tCtx.beginPath();tCtx.arc(p.x,p.y,r+7,0,Math.PI*2);
      const g=tCtx.createRadialGradient(p.x,p.y,r,p.x,p.y,r+9);
      g.addColorStop(0,nc+'44');g.addColorStop(1,'transparent');
      tCtx.fillStyle=g;tCtx.fill();
    }
    tCtx.beginPath();tCtx.arc(p.x,p.y,r,0,Math.PI*2);
    tCtx.fillStyle='var(--surf)';tCtx.fill();
    tCtx.strokeStyle=nc;tCtx.lineWidth=1.8;tCtx.stroke();
    tCtx.fillStyle=nc;tCtx.font='8px monospace';tCtx.textAlign='center';
    tCtx.fillText(n.service_id.substring(0,12),p.x,p.y+3);
  });
}

function renderSim(){
  const badge=el('sim-badge');const sr=state.simResult;
  if(!sr){badge.style.display='none';return;}
  badge.style.display='block';
  const meta=sr.meta||{};
  const budgetStr=meta.budget_used_pct?meta.budget_used_pct.toFixed(0)+'%':'-';
  const degStr=sr.degraded_service_count>0?' dg='+sr.degraded_service_count:'';
  const recMs=sr.recovery_convergence_ms;
  const recStr=recMs>0?' rec='+recMs.toFixed(0)+'ms':recMs===-1?' rec=∞':'';
  if(sr.collapse_detected){
    badge.className='sim-badge b-crit';
    badge.textContent='SIM:COLLAPSE'+(sr.cascade_triggered?'+CASCADE':'')+degStr+' ['+budgetStr+']';
  }else{
    badge.className='sim-badge b-ok';
    badge.textContent='SIM:STABLE'+degStr+recStr+' ['+budgetStr+']';
  }
}

// ── Stability gauges ─────────────────────────────────────────────────────────
function renderStability(){
  const grid=el('sgrid');
  const ids=Object.keys(state.svcs).sort().slice(0,8);
  grid.innerHTML='';
  ids.forEach(id=>{
    const b=state.svcs[id];if(!b)return;
    const st=b.stability||{};
    const risk=st.collapse_risk||0;const osc=st.oscillation_risk||0;
    const cas=st.cascade_amplification_score||0;
    const tam=st.trend_adjusted_margin;
    const dRisk=st.stability_derivative||0;
    const nc=state.netCoupling[id]||{};
    const eqRho=nc.path_equilibrium_rho||0;
    const ssP0=nc.steady_state_p0;
    const ssMq=nc.steady_state_mean_queue;
    const col=risk>.8?'var(--crit)':risk>.5?'var(--warn)':'var(--ok)';
    const R=21,C=2*Math.PI*R,dash=C*Math.max(1-risk,0);
    const zone=st.collapse_zone||'safe';
    const zoneCol=zone==='collapse'?'var(--crit)':zone==='warning'?'var(--warn)':'var(--muted)';
    const dRiskCol=dRisk>0.02?'var(--crit)':dRisk>0.005?'var(--warn)':'var(--ok)';
    const tamStr=tam!=null?(tam<0?'<span style="color:var(--crit)">▼'+tam.toFixed(2)+'</span>':tam.toFixed(2)):'—';
    const eqStr=eqRho>0?'eq:'+eqRho.toFixed(2):'';
    const ssStr=(ssP0!=null&&ssP0>0)?'P₀:'+pct(ssP0)+(ssMq!=null&&isFinite(ssMq)?' Lq:'+ssMq.toFixed(1):''):'';
    grid.innerHTML+='<div class="sc">'+
      '<div class="scn">'+esc(id)+'</div>'+
      '<div class="gr">'+
        '<svg width="54" height="54" viewBox="0 0 54 54">'+
          '<circle cx="27" cy="27" r="'+R+'" fill="none" stroke="var(--border)" stroke-width="5"/>'+
          '<circle cx="27" cy="27" r="'+R+'" fill="none" stroke="'+col+'" stroke-width="5" '+
            'stroke-dasharray="'+C+'" stroke-dashoffset="'+(C-dash)+'" stroke-linecap="round"/>'+
        '</svg>'+
        '<div class="gv" style="color:'+col+'">'+pct(risk)+
          '<div class="gs" style="color:'+zoneCol+'">'+zone+'</div>'+
          '<div class="gs" style="color:'+dRiskCol+'">d:'+dRisk.toFixed(3)+'</div>'+
          '<div class="gs">tam:'+tamStr+'</div>'+
          (eqStr?'<div class="gs" style="color:var(--muted)">'+eqStr+'</div>':'')+
          (ssStr?'<div class="gs" style="color:var(--muted);font-size:7px">'+ssStr+'</div>':'')+
        '</div>'+
      '</div>'+
    '</div>';
  });
}

// ── Charts with prediction CI ─────────────────────────────────────────────────
function renderCharts(){
  const wrap=el('cw');const id=state.sel;
  if(!id||!state.hist[id]){
    wrap.innerHTML='<div style="color:var(--muted);font-size:11px;padding:20px;text-align:center">Select a service</div>';
    return;
  }
  const h=state.hist[id];
  const pred=state.predTimeline[id]||[];
  wrap.innerHTML=
    '<div class="cr"><div class="cl">Utilisation ρ + Prediction CI — '+esc(id)+'</div><canvas id="c0" style="flex:1"></canvas></div>'+
    '<div class="cr"><div class="cl">Arrival Rate (req/s)</div><canvas id="c1" style="flex:1"></canvas></div>'+
    '<div class="cr"><div class="cl">Collapse Risk &amp; Oscillation Risk</div><canvas id="c2" style="flex:1"></canvas></div>';

  drawSparkWithPred('c0',h.map(x=>x.rho),[0,1.2],
    'rgba(0,212,255,.85)','rgba(0,212,255,.12)',pred);
  drawSpark('c1',h.map(x=>x.arr),null,'rgba(124,58,237,.85)','rgba(124,58,237,.12)');
  drawSpark('c2',h.map(x=>x.collapse),[0,1],'rgba(239,68,68,.85)','rgba(239,68,68,.08)');
  drawSparkLine('c2',h.map(x=>x.osc),[0,1],'rgba(245,158,11,.8)');
}

function drawSparkWithPred(cid,data,yRange,stroke,fill,pred){
  const canvas=document.getElementById(cid);if(!canvas)return;
  const par=canvas.parentElement;
  canvas.width=par.clientWidth||300;canvas.height=Math.max(par.clientHeight||50,30);
  const ctx=canvas.getContext('2d');ctx.clearRect(0,0,canvas.width,canvas.height);
  if(data.length<2)return;
  const W=canvas.width,H=canvas.height;
  const mn=yRange?yRange[0]:Math.min(...data);
  const mx=yRange?yRange[1]:Math.max(...data);
  const rng=Math.max(mx-mn,1e-6);
  const sx=i=>i/(data.length-1)*(W*0.65); // historical portion: 65% of width
  const sy=v=>H-((v-mn)/rng)*(H-4)-2;

  // Draw CI band from prediction points (right 35% of canvas)
  const predStart=W*0.65;
  const predW=W*0.35;
  if(pred&&pred.length>1){
    const spx=k=>predStart+k/(pred.length-1)*predW;
    ctx.beginPath();
    ctx.moveTo(spx(0),sy(pred[0].lo));
    for(let k=1;k<pred.length;k++)ctx.lineTo(spx(k),sy(pred[k].lo));
    for(let k=pred.length-1;k>=0;k--)ctx.lineTo(spx(k),sy(pred[k].hi));
    ctx.closePath();ctx.fillStyle='rgba(0,212,255,.08)';ctx.fill();

    // Prediction centre line
    ctx.beginPath();ctx.moveTo(spx(0),sy(pred[0].rho));
    for(let k=1;k<pred.length;k++)ctx.lineTo(spx(k),sy(pred[k].rho));
    ctx.strokeStyle='rgba(0,212,255,.4)';ctx.lineWidth=1;ctx.setLineDash([3,3]);ctx.stroke();
    ctx.setLineDash([]);

    // Setpoint line
    const spLine=sy(0.70);
    ctx.beginPath();ctx.moveTo(0,spLine);ctx.lineTo(W,spLine);
    ctx.strokeStyle='rgba(16,185,129,.3)';ctx.lineWidth=1;ctx.stroke();
  }

  // Historical sparkline
  ctx.beginPath();ctx.moveTo(sx(0),sy(data[0]));
  for(let i=1;i<data.length;i++){
    const cx=(sx(i-1)+sx(i))/2;
    ctx.bezierCurveTo(cx,sy(data[i-1]),cx,sy(data[i]),sx(i),sy(data[i]));
  }
  ctx.strokeStyle=stroke;ctx.lineWidth=1.5;ctx.setLineDash([]);ctx.stroke();
  ctx.lineTo(sx(data.length-1),H);ctx.lineTo(0,H);ctx.closePath();
  ctx.fillStyle=fill;ctx.fill();

  // Grid
  ctx.strokeStyle='rgba(24,38,64,.9)';ctx.lineWidth=1;
  [.25,.5,.75].forEach(f=>{
    const y=H*(1-f);ctx.beginPath();ctx.moveTo(0,y);ctx.lineTo(W,y);ctx.stroke();
  });
  // Divider between history and prediction
  ctx.strokeStyle='rgba(0,212,255,.2)';ctx.lineWidth=1;ctx.setLineDash([2,4]);
  ctx.beginPath();ctx.moveTo(W*0.65,0);ctx.lineTo(W*0.65,H);ctx.stroke();
  ctx.setLineDash([]);

  // Latest dot
  const lx=sx(data.length-1),ly=sy(data[data.length-1]);
  ctx.beginPath();ctx.arc(lx,ly,2.5,0,Math.PI*2);ctx.fillStyle=stroke;ctx.fill();
}

function drawSpark(cid,data,yRange,stroke,fill){
  const canvas=document.getElementById(cid);if(!canvas)return;
  const par=canvas.parentElement;
  canvas.width=par.clientWidth||300;canvas.height=Math.max(par.clientHeight||50,30);
  const ctx=canvas.getContext('2d');ctx.clearRect(0,0,canvas.width,canvas.height);
  if(data.length<2)return;
  const W=canvas.width,H=canvas.height;
  const mn=yRange?yRange[0]:Math.min(...data);
  const mx=yRange?yRange[1]:Math.max(...data);
  const rng=Math.max(mx-mn,1e-6);
  const sx=i=>i/(data.length-1)*W;
  const sy=v=>H-((v-mn)/rng)*(H-4)-2;
  ctx.beginPath();ctx.moveTo(sx(0),sy(data[0]));
  for(let i=1;i<data.length;i++){const cx=(sx(i-1)+sx(i))/2;ctx.bezierCurveTo(cx,sy(data[i-1]),cx,sy(data[i]),sx(i),sy(data[i]));}
  ctx.strokeStyle=stroke;ctx.lineWidth=1.5;ctx.stroke();
  ctx.lineTo(W,H);ctx.lineTo(0,H);ctx.closePath();ctx.fillStyle=fill;ctx.fill();
  ctx.strokeStyle='rgba(24,38,64,.9)';ctx.lineWidth=1;
  [.25,.5,.75].forEach(f=>{const y=H*(1-f);ctx.beginPath();ctx.moveTo(0,y);ctx.lineTo(W,y);ctx.stroke();});
  const lx=sx(data.length-1),ly=sy(data[data.length-1]);
  ctx.beginPath();ctx.arc(lx,ly,2.5,0,Math.PI*2);ctx.fillStyle=stroke;ctx.fill();
}

function drawSparkLine(cid,data,yRange,stroke){
  const canvas=document.getElementById(cid);if(!canvas)return;
  const W=canvas.width,H=canvas.height;
  const ctx=canvas.getContext('2d');
  const mn=yRange?yRange[0]:Math.min(...data);
  const mx=yRange?yRange[1]:Math.max(...data);
  const rng=Math.max(mx-mn,1e-6);
  const sx=i=>i/(data.length-1)*W;
  const sy=v=>H-((v-mn)/rng)*(H-4)-2;
  ctx.beginPath();ctx.moveTo(sx(0),sy(data[0]));
  for(let i=1;i<data.length;i++){const cx=(sx(i-1)+sx(i))/2;ctx.bezierCurveTo(cx,sy(data[i-1]),cx,sy(data[i]),sx(i),sy(data[i]));}
  ctx.strokeStyle=stroke;ctx.lineWidth=1.2;ctx.stroke();
}

// ── Console with model chain ─────────────────────────────────────────────────
function renderConsole(){
  const con=el('console');const evs=(state.events||[]).slice(0,40);
  if(!evs.length)return;
  const SEV=['INFO','WARN','CRIT'];
  con.innerHTML=evs.map(ev=>{
    const s=SEV[ev.severity]||'INFO';
    const ts=ev.timestamp?new Date(ev.timestamp).toISOString().substr(11,8):'';
    const prio=ev.operational_priority!=null?'<span class="prio-badge">P'+ev.operational_priority+'</span>':'';
    const unc=ev.uncertainty_score>0.3?'<span style="font-size:8px;color:var(--muted);margin-left:4px">±'+pct(ev.uncertainty_score)+'</span>':'';
    const rec=ev.recommendation?'<div class="crec">↳ '+esc(ev.recommendation)+'</div>':'';
    const chain=ev.model_chain?'<div class="cchain">'+esc(ev.model_chain)+'</div>':'';
    return '<div class="cl2 '+s+'">'+
      '<span class="cts">'+ts+'</span>'+prio+
      '<span class="ccat">['+esc(ev.category)+']</span>'+
      '<span class="cmsg">'+esc(ev.description)+'</span>'+unc+
      rec+chain+'</div>';
  }).join('');
}

// ── Runtime metrics panel ─────────────────────────────────────────────────
function renderRuntimeMetrics(){
  const rm=state.runtimeMetrics;
  if(!rm||!rm.avg_modelling_ms)return;
  const stages=[
    ['prune',     rm.avg_prune_ms],
    ['windows',   rm.avg_windows_ms],
    ['topology',  rm.avg_topology_ms],
    ['coupling',  rm.avg_coupling_ms],
    ['modelling', rm.avg_modelling_ms],
    ['optimise',  rm.avg_optimise_ms],
    ['sim',       rm.avg_sim_ms],
    ['reasoning', rm.avg_reasoning_ms],
    ['broadcast', rm.avg_broadcast_ms],
  ];
  const maxMs=Math.max(...stages.map(s=>s[1]||0),0.1);
  const safetyBadge=state.safetyMode?'<span style="color:var(--crit);font-size:8px;margin-left:6px">⚠ SAFETY</span>':'';
  const predictedBadge=(rm.predicted_overrun)?'<span style="color:var(--warn);font-size:8px;margin-left:4px">⚡PRED</span>':'';
  const jStr=state.jitterMs?(state.jitterMs.toFixed(1)+'ms'):'—';
  const predStr=rm.predicted_critical_ms?rm.predicted_critical_ms.toFixed(0)+'ms pred':'';
  let html='<div style="border-top:1px solid var(--border);margin-top:6px;padding-top:6px">'+
    '<div style="font-size:9px;color:var(--muted);text-transform:uppercase;letter-spacing:1px;margin-bottom:4px">'+
    'Runtime Stages'+safetyBadge+predictedBadge+
    '<span style="float:right;font-size:9px;color:var(--muted)">jitter:'+jStr+' '+predStr+' overruns:'+((rm.total_overruns)||0)+'</span></div>';
  stages.forEach(([name,ms])=>{
    if(!ms&&ms!==0)return;
    const w=Math.min((ms/maxMs)*100,100);
    const col=ms>5?'var(--warn)':ms>2?'var(--accent)':'var(--ok)';
    html+='<div style="display:flex;align-items:center;gap:5px;margin-bottom:2px">'+
      '<span style="width:60px;font-size:8px;color:var(--muted);flex-shrink:0">'+name+'</span>'+
      '<div style="flex:1;height:4px;background:var(--border);border-radius:2px;overflow:hidden">'+
        '<div style="width:'+w+'%;height:100%;background:'+col+';border-radius:2px;transition:width .4s"></div>'+
      '</div>'+
      '<span style="width:38px;font-size:8px;color:'+col+';text-align:right;flex-shrink:0">'+ms.toFixed(2)+'ms</span>'+
    '</div>';
  });
  html+='</div>';
  const con=el('console');
  let panel=document.getElementById('rm-panel');
  if(!panel){panel=document.createElement('div');panel.id='rm-panel';con.parentElement.appendChild(panel);}
  panel.innerHTML=html;
  panel.style.padding='0 8px 8px';
  panel.style.flexShrink='0';
}
function pct(v){if(v==null||isNaN(v))return'—';return(v*100).toFixed(1)+'%'}
function ms(v){if(v==null||isNaN(v)||!isFinite(v))return'—';return v.toFixed(0)+'ms'}
function fmt1(v){if(v==null||isNaN(v))return'—';return v.toFixed(1)}
function rhoColor(r){return r>.9?'var(--crit)':r>.75?'var(--warn)':'var(--ok)'}
function chip(l,v,c){return'<div class="mc"><span class="ml">'+l+'</span><span class="mv" style="color:'+c+'">'+v+'</span></div>'}
function esc(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;')}
function el(id){return document.getElementById(id)}

let tCtx2=null;
function switchView(name){
  document.querySelectorAll('.view').forEach(v=>v.classList.remove('active'));
  document.querySelectorAll('.nb').forEach(b=>b.classList.remove('active'));
  const view=document.getElementById('view-'+name);
  if(view)view.classList.add('active');
  document.querySelectorAll('.nb[data-view="'+name+'"]').forEach(b=>b.classList.add('active'));
  // Trigger immediate render for the newly visible tab.
  requestAnimationFrame(()=>{
    if(name==='overview')renderOverview();
    if(name==='topology'){initTopo2();renderTopologyTab();}
    if(name==='stability')renderStabilityTab();
    if(name==='prediction')renderPredictionTab();
    if(name==='optimisation')renderOptimisationTab();
    if(name==='simulation')renderSimulationTab();
    if(name==='telemetry')renderTelemetryTab();
    if(name==='health')renderHealthTab();
  });
}
function initTopo2(){
  const c=document.getElementById('topo2');
  if(c&&!tCtx2){tCtx2=c.getContext('2d');}
}
connect();
setInterval(()=>{if(state.topo)renderTopo();},100);
setInterval(()=>{
  const av=document.querySelector('.view.active');
  if(av&&av.id==='view-topology')renderTopologyTab();
},120);

// ── Scenario Comparison Panel ────────────────────────────────────────────────
// Renders a best/worst/stable fraction bar below the sim badge when scenario data exists.
function renderScenarioComparison(){
  const sc=state.scenarioComp;
  let panel=document.getElementById('sc-panel');
  if(!panel){
    panel=document.createElement('div');
    panel.id='sc-panel';
    // Inject below sim badge inside the topology panel's pb div.
    const tpb=document.querySelector('.panel:not(.span-col) .pb');
    if(tpb)tpb.appendChild(panel);
  }
  if(!sc||!sc.scenario_count){panel.style.display='none';return;}
  panel.style.display='block';
  panel.style.cssText='position:absolute;bottom:28px;left:6px;right:6px;background:var(--surf2);border:1px solid var(--border);border-radius:3px;padding:5px 7px;font-size:9px;';
  const stableColor=sc.stable_scenario_fraction>=0.8?'var(--ok)':sc.stable_scenario_fraction>=0.5?'var(--warn)':'var(--crit)';
  const wcColor=sc.worst_case_collapse>.5?'var(--crit)':sc.worst_case_collapse>.2?'var(--warn)':'var(--ok)';
  panel.innerHTML=
    '<div style="color:var(--muted);letter-spacing:1px;text-transform:uppercase;margin-bottom:3px">'+
      '⚃ Monte-Carlo (n='+sc.scenario_count+')</div>'+
    '<div style="display:flex;gap:10px;flex-wrap:wrap">'+
      '<span>Stable: <b style="color:'+stableColor+'">'+pct(sc.stable_scenario_fraction)+'</b></span>'+
      '<span>Best Collapse: <b style="color:var(--ok)">'+pct(sc.best_case_collapse)+'</b></span>'+
      '<span>Worst: <b style="color:'+wcColor+'">'+pct(sc.worst_case_collapse)+'</b></span>'+
      '<span>SLA p50: <b style="color:var(--warn)">'+pct(sc.median_sla_violation)+'</b></span>'+
      (sc.recovery_convergence_max_ms>0?'<span>Rec: <b>'+sc.recovery_convergence_min_ms.toFixed(0)+'-'+sc.recovery_convergence_max_ms.toFixed(0)+'ms</b></span>':'')+
    '</div>';
}

// ── Safety Level Indicator ───────────────────────────────────────────────────
// Adds a thin coloured bar under the header proportional to safety escalation level.
function renderSafetyBar(){
  let bar=document.getElementById('safety-bar');
  if(!bar){
    bar=document.createElement('div');
    bar.id='safety-bar';
    bar.style.cssText='height:2px;width:100%;position:fixed;top:48px;left:0;right:0;z-index:100;transition:background .5s,opacity .5s';
    document.body.appendChild(bar);
  }
  const lvl=(state.runtimeMetrics&&state.runtimeMetrics.safety_level)||0;
  const colors=['transparent','rgba(245,158,11,.6)','rgba(239,68,68,.7)','rgba(239,68,68,1)'];
  bar.style.background=colors[Math.min(lvl,3)];
  bar.style.boxShadow=lvl>=2?'0 0 8px '+colors[lvl]:'none';
}

// ── Equilibrium Proximity Gauge ──────────────────────────────────────────────
// Shows how close the system is to fixed-point equilibrium collapse.
// Renders as a horizontal bar in the stability panel.
function renderEquilibriumProximity(){
  let panel=document.getElementById('eq-prox');
  if(!panel){
    panel=document.createElement('div');
    panel.id='eq-prox';
    panel.style.cssText='padding:5px 8px;border-top:1px solid var(--border);flex-shrink:0';
    const spb=document.getElementById('sgrid')&&document.getElementById('sgrid').parentElement;
    if(spb)spb.appendChild(panel);
  }
  const fp=state.fixedPointEquil||{};
  const fpC=fp.systemic_collapse_prob||0;
  const fpConv=fp.converged;
  const pSens=fp.perturbation_sensitivity||{};
  const env=state.stabilityEnvelope||{};
  const headroom=env.envelope_headroom;
  const safeMax=env.safe_system_rho_max||0;
  const curMean=env.current_system_rho_mean||0;

  let maxSens=0,maxSensId='';
  Object.entries(pSens).forEach(([id,s])=>{if(s>maxSens){maxSens=s;maxSensId=id;}});
  const barW=Math.min(fpC*100,100);
  const barColor=fpC>.6?'var(--crit)':fpC>.3?'var(--warn)':'var(--ok)';

  // Stability envelope bar
  const envelopeBarUsed=safeMax>0?Math.min(curMean/safeMax*100,110):0;
  const envColor=headroom!=null?(headroom<0?'var(--crit)':headroom<0.1?'var(--warn)':'var(--ok)'):'var(--muted)';

  panel.innerHTML=
    '<div style="font-size:8px;color:var(--muted);text-transform:uppercase;letter-spacing:1px;margin-bottom:3px">'+
      'Equilibrium Proximity'+(fpConv?'':' <span style="color:var(--warn)">*</span>')+'</div>'+
    '<div style="background:var(--border);border-radius:2px;height:4px;margin-bottom:3px">'+
      '<div style="width:'+barW+'%;height:100%;border-radius:2px;background:'+barColor+';transition:width .6s"></div>'+
    '</div>'+
    '<div style="font-size:9px;color:var(--muted);margin-bottom:4px">'+
      'P(systemic collapse)=<b style="color:'+barColor+'">'+pct(fpC)+'</b>'+
      (fp.convergence_rate>0?' ρ(J)=<b>'+(fp.convergence_rate).toFixed(3)+'</b>':'')+
      (fp.stability_margin!=null?' margin=<b style="color:'+(fp.stability_margin>0?'var(--ok)':'var(--crit)')+'">'+fp.stability_margin.toFixed(3)+'</b>':'')+
      (maxSensId?' | crit:<b style="color:var(--warn)">'+esc(maxSensId)+'</b>':'')+
    '</div>'+
    (safeMax>0?
    '<div style="font-size:8px;color:var(--muted);margin-bottom:2px">Stability Envelope: ρ̄=<b>'+curMean.toFixed(2)+'</b> / safe=<b>'+safeMax.toFixed(2)+'</b> headroom=<b style="color:'+envColor+'">'+(headroom!=null?headroom.toFixed(3):'—')+'</b></div>'+
    '<div style="background:var(--border);border-radius:2px;height:3px">'+
      '<div style="width:'+Math.min(envelopeBarUsed,100)+'%;height:100%;border-radius:2px;background:'+envColor+';transition:width .6s"></div>'+
    '</div>':'');
}

// ── Animated Topology Pressure Heatmap ───────────────────────────────────────
// Overlay coloured halos on topology nodes using pressure heatmap data.
// Called by the topology renderer — pulses the halos at high pressure.
function applyHeatmapToNodes(W,H,nodes){
  if(!state.pressureHeatmap||!nodes)return;
  nodes.forEach(n=>{
    const p=state.pressureHeatmap[n.service_id]||0;
    if(p<0.3)return;
    const pos=topoPos[n.service_id];
    if(!pos)return;
    const alpha=Math.min((p-0.3)/0.7*0.6,0.6);
    const radius=18+p*16;
    const col=p>.8?'rgba(239,68,68,':'rgba(245,158,11,';
    const grad=tCtx.createRadialGradient(pos.x,pos.y,0,pos.x,pos.y,radius);
    grad.addColorStop(0,col+alpha+')');
    grad.addColorStop(1,col+'0)');
    tCtx.beginPath();
    tCtx.arc(pos.x,pos.y,radius,0,Math.PI*2);
    tCtx.fillStyle=grad;
    tCtx.fill();
  });
}

// ── Probability Distribution Chart ──────────────────────────────────────────
// Renders a compact probability bar chart for cascade failure probability from sim overlay.
function renderCascadeDistribution(){
  let panel=document.getElementById('prob-panel');
  if(!panel){
    panel=document.createElement('div');
    panel.id='prob-panel';
    panel.style.cssText='padding:5px 8px;border-top:1px solid var(--border);flex-shrink:0;max-height:80px;overflow:hidden';
    const cpb=el('console')&&el('console').parentElement;
    if(cpb)cpb.insertBefore(panel,el('console'));
  }
  const overlay=state.simOverlay||{};
  const cfpMap=overlay.cascade_failure_prob||{};
  const slaMap=overlay.sla_violation_prob||{};
  const ids=Object.keys(cfpMap).filter(id=>cfpMap[id]>0.01);
  if(ids.length===0){panel.style.display='none';return;}
  panel.style.display='block';
  const bars=ids.slice(0,6).map(id=>{
    const cfp=cfpMap[id]||0;
    const sla=slaMap[id]||0;
    const w=Math.min(cfp*100,100);
    const wSla=Math.min(sla*100,100);
    const c=cfp>.4?'var(--crit)':cfp>.15?'var(--warn)':'var(--ok)';
    return '<div style="display:flex;align-items:center;gap:4px;margin-bottom:2px">'+
      '<div style="width:55px;font-size:8px;color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex-shrink:0">'+esc(id)+'</div>'+
      '<div style="flex:1;height:5px;background:var(--border);border-radius:2px;overflow:hidden">'+
        '<div style="width:'+w+'%;height:100%;background:'+c+'"></div>'+
      '</div>'+
      '<div style="width:28px;font-size:8px;color:'+c+';text-align:right;flex-shrink:0">'+pct(cfp)+'</div>'+
      (sla>0.05?'<div style="width:22px;font-size:7px;color:var(--warn);text-align:right;flex-shrink:0">SLA:'+pct(sla)+'</div>':'')+
    '</div>';
  }).join('');
  const age=overlay.sim_tick_age||0;
  panel.innerHTML='<div style="font-size:8px;color:var(--muted);text-transform:uppercase;letter-spacing:1px;margin-bottom:3px">'+
    '⚡ Cascade P(failure)'+(age>0?' <span style="opacity:.5">+'+age+'t</span>':'')+'</div>'+bars;
}

// ── OVERVIEW TAB ─────────────────────────────────────────────────────────────
function renderOverview(){
  const o=state.obj||{};
  const ne=state.netEquilibrium||{};
  const fp=state.fixedPointEquil||{};
  const env=state.stabilityEnvelope||{};
  const score=o.composite_score||0;
  const scoreCol=score>.8?'var(--crit)':score>.5?'var(--warn)':'var(--ok)';
  const headroom=env.envelope_headroom;
  const hCol=headroom==null?'var(--muted)':headroom<0?'var(--crit)':headroom<0.1?'var(--warn)':'var(--ok)';

  const kpis=[
    {l:'Objective Score',v:pct(score),col:scoreCol,sub:'composite risk-latency'},
    {l:'Cascade Risk',v:pct(o.cascade_failure_probability),col:score>.5?'var(--warn)':'var(--muted)',sub:'critical path'},
    {l:'P99 Latency',v:ms(o.predicted_p99_latency_ms),col:'var(--text)',sub:'weighted estimate'},
    {l:'Oscillation Risk',v:pct(o.oscillation_risk),col:o.oscillation_risk>.6?'var(--warn)':'var(--muted)',sub:'two-timescale'},
    {l:'FP Collapse',v:pct(fp.systemic_collapse_prob),col:fp.systemic_collapse_prob>.4?'var(--crit)':'var(--muted)',sub:fp.converged?'converged':'not converged'},
    {l:'Envelope Headroom',v:headroom!=null?headroom.toFixed(3):'—',col:hCol,sub:'safe ρ̄='+((env.safe_system_rho_max||0).toFixed(2))},
    {l:'Net Saturation',v:pct(ne.network_saturation_risk),col:ne.network_saturation_risk>.5?'var(--crit)':'var(--muted)',sub:ne.is_converging?'converging':'diverging'},
    {l:'Trajectory Cost',v:pct(o.trajectory_score),col:o.trajectory_score>.7?'var(--crit)':o.trajectory_score>.4?'var(--warn)':'var(--ok)',sub:'risk-latency path'},
  ];
  const grid=el('kpi-grid');
  if(grid){
    grid.innerHTML=kpis.map(k=>{
      const cls=k.col==='var(--crit)'?'crit':k.col==='var(--warn)'?'warn':'';
      return '<div class="kpi '+cls+'"><div class="kpi-label">'+k.l+'</div>'+
             '<div class="kpi-val" style="color:'+k.col+'">'+k.v+'</div>'+
             '<div class="kpi-sub">'+k.sub+'</div></div>';
    }).join('');
  }

  // Risk priority queue
  const rq=el('ov-riskqueue');
  if(rq){
    const items=state.riskQueue||[];
    if(items.length===0){rq.innerHTML='<div style="color:var(--muted);font-size:9px;padding:4px">No risk data</div>';return;}
    rq.innerHTML=items.slice(0,12).map((item,i)=>{
      const col=item.urgency_class==='critical'?'var(--crit)':item.urgency_class==='warning'?'var(--warn)':item.urgency_class==='elevated'?'var(--accent)':'var(--muted)';
      return '<div style="display:flex;align-items:center;gap:6px;padding:4px 2px;border-bottom:1px solid var(--border2)">'+
        '<span style="font-size:10px;font-weight:700;color:var(--muted);width:14px">'+(i+1)+'</span>'+
        '<span style="font-size:8px;font-weight:700;padding:1px 4px;border-radius:2px;background:'+col+'22;color:'+col+';border:1px solid '+col+'44">'+(item.urgency_class||'nom').toUpperCase().slice(0,4)+'</span>'+
        '<span style="flex:1;font-size:10px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+esc(item.service_id)+'</span>'+
        '<span style="font-size:9px;color:'+col+'">'+pct(item.urgency_score)+'</span>'+
        (item.is_keystone?'<span style="font-size:7px;color:var(--purple)">KEY</span>':'')+
      '</div>';
    }).join('');
  }
}

// ── TOPOLOGY TAB ─────────────────────────────────────────────────────────────
function renderTopologyTab(){
  // Service pressure list
  const tpList=el('tp-slist');
  if(tpList){
    const ids=Object.keys(state.svcs).sort((a,b)=>(state.pressureHeatmap[b]||0)-(state.pressureHeatmap[a]||0));
    tpList.innerHTML=ids.slice(0,20).map(id=>{
      const p=state.pressureHeatmap[id]||0;
      const rho=(state.svcs[id]&&state.svcs[id].queue&&state.svcs[id].queue.utilisation)||0;
      const col=p>.8?'var(--crit)':p>.6?'var(--warn)':'var(--ok)';
      return '<div class="mbar-wrap"><div class="mbar-label">'+esc(id)+'</div>'+
             '<div class="mbar-track"><div class="mbar-fill" style="width:'+Math.min(p*100,100)+'%;background:'+col+'"></div></div>'+
             '<div class="mbar-val" style="color:'+col+'">'+pct(p)+'</div></div>';
    }).join('');
  }

  // Network equilibrium panel
  const teq=el('tp-equil');
  if(teq){
    const ne=state.netEquilibrium||{};
    const fp=state.fixedPointEquil||{};
    const env=state.stabilityEnvelope||{};
    teq.innerHTML=
      kv('System ρ̄',ne.system_rho_mean!=null?(ne.system_rho_mean).toFixed(3):'—')+
      kv('ρ̄ Variance',(ne.system_rho_variance||0).toFixed(4))+
      kv('Equil Delta',(ne.equilibrium_delta||0).toFixed(4))+
      kv('Converging',ne.is_converging?'YES ↓':'NO ↑',ne.is_converging?'var(--ok)':'var(--warn)')+
      kv('Sat Risk',pct(ne.network_saturation_risk))+
      kv('Critical Svc',esc(ne.critical_service_id||'—'))+
      '<div style="height:8px"></div>'+
      kv('FP Converged',fp.converged?'YES ('+fp.converged_iterations+' iters)':'NO',fp.converged?'var(--ok)':'var(--warn)')+
      kv('Spectral Radius',(fp.convergence_rate||0).toFixed(4))+
      kv('Stability Margin',(fp.stability_margin||0).toFixed(4),(fp.stability_margin||0)>0?'var(--ok)':'var(--crit)')+
      kv('FP Systemic P',pct(fp.systemic_collapse_prob))+
      '<div style="height:8px"></div>'+
      kv('Safe Max ρ̄',(env.safe_system_rho_max||0).toFixed(3))+
      kv('Headroom',(env.envelope_headroom!=null?env.envelope_headroom.toFixed(3):'—'),(env.envelope_headroom||0)>0?'var(--ok)':'var(--crit)')+
      kv('Most Vulnerable',esc(env.most_vulnerable_service||'—'));
  }

  // Render topology on topo2 canvas (same logic as main topo but full-panel)
  const c2=document.getElementById('topo2');
  if(c2&&state.topo){
    const par=c2.parentElement;
    c2.width=par.clientWidth||600;c2.height=par.clientHeight||400;
    if(!tCtx2)tCtx2=c2.getContext('2d');
    renderTopoOnCtx(tCtx2,c2.width,c2.height,state.topo.nodes||[],state.topo.edges||[],true);
  }
}

// ── STABILITY TAB ─────────────────────────────────────────────────────────────
function renderStabilityTab(){
  const fp=state.fixedPointEquil||{};
  const env=state.stabilityEnvelope||{};

  // Envelope panel
  const envP=el('stab-envelope');
  if(envP){
    const headroom=env.envelope_headroom||0;
    const safeMax=env.safe_system_rho_max||0;
    const curMean=env.current_system_rho_mean||0;
    const fill=safeMax>0?Math.min(curMean/safeMax*100,120):0;
    const envCol=headroom<0?'var(--crit)':headroom<0.1?'var(--warn)':'var(--ok)';
    envP.innerHTML=
      '<div style="font-size:9px;color:var(--muted);margin-bottom:8px">Current ρ̄ vs safe operating envelope</div>'+
      '<div style="display:flex;justify-content:space-between;font-size:9px;margin-bottom:3px">'+
        '<span>Current: <b style="color:var(--text)">'+curMean.toFixed(3)+'</b></span>'+
        '<span>Safe Max: <b style="color:var(--ok)">'+safeMax.toFixed(3)+'</b></span>'+
      '</div>'+
      '<div style="background:var(--border);border-radius:3px;height:10px;overflow:hidden;margin-bottom:6px">'+
        '<div style="width:'+Math.min(fill,100)+'%;height:100%;background:'+envCol+';border-radius:3px;transition:width .6s"></div>'+
      '</div>'+
      '<div style="font-size:10px;font-weight:700;color:'+envCol+'">Headroom: '+(headroom>=0?'+':'')+headroom.toFixed(3)+'</div>'+
      (env.most_vulnerable_service?'<div style="font-size:8px;color:var(--muted);margin-top:4px">Most vulnerable: <b style="color:var(--warn)">'+esc(env.most_vulnerable_service)+'</b> (Δcollapse: +'+pct(env.worst_perturbation_delta)+')</div>':'');
  }

  // Fixed-point solver panel
  const fpP=el('stab-fp');
  if(fpP){
    const crCol=(fp.convergence_rate||1)>=1?'var(--crit)':'var(--ok)';
    const smCol=(fp.stability_margin||0)>0?'var(--ok)':'var(--crit)';
    fpP.innerHTML=
      kv('Solver Status',fp.converged?'CONVERGED':'NOT CONVERGED',fp.converged?'var(--ok)':'var(--warn)')+
      kv('Iterations',fp.converged_iterations||'—')+
      kv('Spectral Radius ρ(J)',(fp.convergence_rate||0).toFixed(5),crCol)+
      kv('Stability Margin 1-ρ(J)',(fp.stability_margin||0).toFixed(5),smCol)+
      kv('Systemic P(collapse)',pct(fp.systemic_collapse_prob))+
      '<div style="margin-top:6px;font-size:8px;color:var(--muted)">ρ(J)<1: contraction mapping, system stable.<br>ρ(J)≥1: expanding iteration, unstable equilibrium.</div>';
  }

  // Perturbation sensitivity
  const pp=el('stab-perturb');
  if(pp){
    const sens=fp.perturbation_sensitivity||{};
    const entries=Object.entries(sens).sort((a,b)=>b[1]-a[1]).slice(0,12);
    if(entries.length===0){pp.innerHTML='<div style="color:var(--muted);font-size:9px">Run every 5 ticks…</div>';return;}
    pp.innerHTML='<div style="font-size:8px;color:var(--muted);margin-bottom:6px">ΔP(collapse) from 30% capacity loss per service:</div>'+
      entries.map(([id,delta])=>{
        const col=delta>.2?'var(--crit)':delta>.08?'var(--warn)':'var(--muted)';
        return '<div class="mbar-wrap">'+
          '<div class="mbar-label">'+esc(id)+'</div>'+
          '<div class="mbar-track"><div class="mbar-fill" style="width:'+Math.min(delta*400,100)+'%;background:'+col+'"></div></div>'+
          '<div class="mbar-val" style="color:'+col+'">'+pct(delta)+'</div></div>';
      }).join('');
  }
}

// ── PREDICTION TAB ────────────────────────────────────────────────────────────
function renderPredictionTab(){
  // Service selector
  const psl=el('pred-slist');
  if(psl){
    const ids=Object.keys(state.svcs).sort();
    psl.innerHTML=ids.map(id=>{
      const isSel=state.sel===id;
      return '<div style="padding:4px 6px;border-radius:3px;cursor:pointer;margin-bottom:2px;font-size:10px;'+
        (isSel?'background:rgba(0,212,255,.1);color:var(--accent);border:1px solid rgba(0,212,255,.3)':'border:1px solid transparent;color:var(--muted)')+
        '" onclick="state.sel=\''+id+'\';renderPredictionTab()">'+esc(id)+'</div>';
    }).join('');
  }

  // Risk timeline runway
  const rtl=el('pred-rtl');
  if(rtl){
    const timeline=state.riskTimeline||{};
    const ids=Object.keys(timeline).sort((a,b)=>{
      const pa=timeline[a];const pb=timeline[b];
      return (pb[pb.length-1]&&pb[pb.length-1].risk||0)-(pa[pa.length-1]&&pa[pa.length-1].risk||0);
    }).slice(0,10);
    rtl.innerHTML='<div style="font-size:8px;color:var(--muted);margin-bottom:5px">Risk trajectory over prediction horizon (higher=more risk):</div>'+
      ids.map(id=>{
        const pts=timeline[id]||[];
        if(pts.length<2)return '';
        const maxRisk=Math.max(...pts.map(p=>p.risk));
        const col=maxRisk>.7?'var(--crit)':maxRisk>.4?'var(--warn)':'var(--ok)';
        const segs=pts.map((p,i)=>{
          const nextP=pts[i+1];
          const w=nextP?(100/(pts.length-1)):0;
          return '<div style="flex:1;height:100%;background:'+rhoColor(p.rho)+'22;'+
            'border-right:1px solid '+(p.risk>.7?'var(--crit)':p.risk>.4?'var(--warn)':'transparent')+';position:relative">'+
            (p.risk>.5?'<div style="position:absolute;bottom:0;left:0;right:0;height:'+Math.min(p.risk*100,100)+'%;background:'+col+'33"></div>':'')+
            '</div>';
        }).join('');
        return '<div class="rtl-row"><div class="rtl-id">'+esc(id)+'</div>'+
          '<div class="rtl-seg" style="display:flex">'+segs+'</div>'+
          '<div style="font-size:8px;color:'+col+';width:34px;text-align:right">'+pct(maxRisk)+'</div></div>';
      }).join('');
  }
}

// ── OPTIMISATION TAB ──────────────────────────────────────────────────────────
function renderOptimisationTab(){
  const o=state.obj||{};
  const directives=state.directives||{};

  // Global objective panel
  const op=el('opt-obj');
  if(op){
    op.innerHTML=
      kv('Composite Score',pct(o.composite_score),(o.composite_score||0)>.7?'var(--crit)':'var(--ok)')+
      kv('Ref Latency',ms(o.reference_latency_ms))+
      kv('Risk Acceleration',(o.risk_acceleration||0)>0?'+'+((o.risk_acceleration||0).toFixed(4)):((o.risk_acceleration||0).toFixed(4)),(o.risk_acceleration||0)>0.01?'var(--warn)':'var(--ok)')+
      kv('Trajectory Score',pct(o.trajectory_score),(o.trajectory_score||0)>.7?'var(--crit)':o.trajectory_score>.4?'var(--warn)':'var(--ok)')+
      kv('Trend Stability',pct(o.trend_stability_margin))+
      '<div style="margin-top:6px;font-size:8px;color:var(--muted)">Weights: Latency 40% · Cascade 30% · Instability 20% · Osc 10%</div>';
  }

  // Trajectory scores per service
  const tp=el('opt-traj');
  if(tp){
    const ids=Object.keys(directives).sort((a,b)=>(directives[b].trajectory_cost_avg||0)-(directives[a].trajectory_cost_avg||0));
    tp.innerHTML='<div style="font-size:8px;color:var(--muted);margin-bottom:5px">Per-service trajectory cost (risk-latency path):</div>'+
      ids.slice(0,8).map(id=>{
        const d=directives[id]||{};
        const tc=d.trajectory_cost_avg||0;
        const col=tc>.7?'var(--crit)':tc>.4?'var(--warn)':'var(--ok)';
        const conv=d.planner_convergent?'✓':'✗';
        const convCol=d.planner_convergent?'var(--ok)':'var(--warn)';
        return '<div class="mbar-wrap"><div class="mbar-label">'+esc(id)+'</div>'+
               '<div class="mbar-track"><div class="mbar-fill" style="width:'+Math.min(tc*100,100)+'%;background:'+col+'"></div></div>'+
               '<div class="mbar-val" style="color:'+col+'">'+pct(tc)+'</div>'+
               '<span style="font-size:9px;color:'+convCol+';margin-left:4px">'+conv+'</span></div>';
      }).join('');
  }

  // Directives table
  const dt=el('opt-directives');
  if(dt){
    const ids=Object.keys(directives).sort();
    dt.innerHTML='<table class="opt-table"><thead><tr>'+
      '<th>Service</th><th>Scale</th><th>ρ Error</th><th>Planner SF</th><th>Prob Score</th><th>MPC Pred ρ</th><th>Status</th></tr></thead><tbody>'+
      ids.map(id=>{
        const d=directives[id]||{};
        const errCol=(d.error||0)>.15?'var(--crit)':(d.error||0)>.05?'var(--warn)':'var(--ok)';
        const status=d.mpc_overshoot_risk?'OVER':d.mpc_underactuation_risk?'LOW':'OK';
        const statCol=status==='OK'?'var(--ok)':status==='OVER'?'var(--warn)':'var(--crit)';
        return '<tr>'+
          '<td>'+esc(id)+'</td>'+
          '<td style="color:var(--accent)">'+((d.scale_factor||1).toFixed(2))+'×</td>'+
          '<td style="color:'+errCol+'">'+((d.error||0)>0?'+':'')+((d.error||0).toFixed(3))+'</td>'+
          '<td>'+((d.planner_scale_factor||0).toFixed(2))+'×</td>'+
          '<td>'+((d.planner_probabilistic_score||0).toFixed(3))+'</td>'+
          '<td>'+((d.mpc_predicted_rho||0).toFixed(3))+'</td>'+
          '<td style="color:'+statCol+'">'+status+'</td>'+
        '</tr>';
      }).join('')+'</tbody></table>';
  }
}

// ── SIMULATION TAB ────────────────────────────────────────────────────────────
function renderSimulationTab(){
  const sim=state.simResult||null;
  const overlay=state.simOverlay||{};
  const sc=state.scenarioComp||null;

  // Scenarios panel
  const sp=el('sim-scenarios');
  if(sp){
    if(!sc){sp.innerHTML='<div style="color:var(--muted);font-size:9px">Runs every 10 ticks…</div>';}
    else{
      const stCol=sc.stable_scenario_fraction>=.8?'var(--ok)':sc.stable_scenario_fraction>=.5?'var(--warn)':'var(--crit)';
      sp.innerHTML=
        kv('Scenarios Run',sc.scenario_count)+
        kv('Stable Fraction',pct(sc.stable_scenario_fraction),stCol)+
        kv('Best-Case Collapse',pct(sc.best_case_collapse),'var(--ok)')+
        kv('Worst-Case Collapse',pct(sc.worst_case_collapse),'var(--crit)')+
        kv('Median SLA Violation',pct(sc.median_sla_violation))+
        kv('Recovery Range',sc.recovery_convergence_min_ms.toFixed(0)+'–'+sc.recovery_convergence_max_ms.toFixed(0)+'ms');
    }
  }

  // Cascade failure distribution
  const cd=el('sim-cascade');
  if(cd){
    const cfp=overlay.cascade_failure_prob||{};
    const ids=Object.keys(cfp).filter(id=>cfp[id]>0).sort((a,b)=>cfp[b]-cfp[a]);
    if(ids.length===0){cd.innerHTML='<div style="color:var(--muted);font-size:9px">No cascade data</div>';}
    else cd.innerHTML=ids.slice(0,10).map(id=>{
      const p=cfp[id];const col=p>.4?'var(--crit)':p>.15?'var(--warn)':'var(--ok)';
      return '<div class="mbar-wrap"><div class="mbar-label">'+esc(id)+'</div>'+
        '<div class="mbar-track"><div class="mbar-fill" style="width:'+Math.min(p*100,100)+'%;background:'+col+'"></div></div>'+
        '<div class="mbar-val" style="color:'+col+'">'+pct(p)+'</div></div>';
    }).join('');
  }

  // Queue distribution
  const qd=el('sim-qdist');
  if(qd){
    const qdist=overlay.p95_queue_len||{};
    const ids=Object.keys(qdist).sort((a,b)=>qdist[b]-qdist[a]);
    qd.innerHTML=ids.slice(0,10).map(id=>{
      const p95=qdist[id];
      const sfrac=(overlay.saturation_frac&&overlay.saturation_frac[id])||0;
      const col=sfrac>.5?'var(--crit)':sfrac>.2?'var(--warn)':'var(--ok)';
      return '<div style="display:flex;align-items:center;gap:6px;padding:3px 0;border-bottom:1px solid var(--border2)">'+
        '<div style="flex:1;font-size:9px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+esc(id)+'</div>'+
        '<div style="font-size:9px;color:var(--text)">P95: <b>'+p95.toFixed(0)+'</b></div>'+
        '<div style="font-size:9px;color:'+col+'">Sat: '+pct(sfrac)+'</div></div>';
    }).join('');
  }

  // SLA violations
  const sla=el('sim-sla');
  if(sla){
    const slaMap=overlay.sla_violation_prob||{};
    const ids=Object.keys(slaMap).filter(id=>slaMap[id]>0).sort((a,b)=>slaMap[b]-slaMap[a]);
    if(ids.length===0){sla.innerHTML='<div style="color:var(--muted);font-size:9px">No SLA violations observed</div>';}
    else sla.innerHTML=ids.slice(0,10).map(id=>{
      const p=slaMap[id];const col=p>.3?'var(--crit)':p>.1?'var(--warn)':'var(--ok)';
      return '<div class="mbar-wrap"><div class="mbar-label">'+esc(id)+'</div>'+
        '<div class="mbar-track"><div class="mbar-fill" style="width:'+Math.min(p*100,100)+'%;background:'+col+'"></div></div>'+
        '<div class="mbar-val" style="color:'+col+'">'+pct(p)+'</div></div>';
    }).join('');
  }
}

// ── TELEMETRY TAB ─────────────────────────────────────────────────────────────
function renderTelemetryTab(){
  const now=Date.now();

  // Freshness panel
  const fp=el('telem-freshness');
  if(fp){
    const ids=Object.keys(state.svcs).sort();
    fp.innerHTML=ids.map(id=>{
      const b=state.svcs[id]||{};
      const conf=(b.queue&&b.queue.confidence)||0;
      const quality=(b.queue&&b.queue.signal_quality)||'?';
      const col=conf>.7?'var(--ok)':conf>.4?'var(--warn)':'var(--crit)';
      const dotCol=quality==='good'?'var(--ok)':quality==='degraded'?'var(--warn)':'var(--crit)';
      return '<div style="display:flex;align-items:center;gap:6px;padding:3px 0;border-bottom:1px solid var(--border2)">'+
        '<span class="fresh-dot" style="background:'+dotCol+'"></span>'+
        '<span style="flex:1;font-size:9px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+esc(id)+'</span>'+
        '<span style="font-size:8px;color:var(--muted);width:50px;text-align:right">'+quality+'</span>'+
        '<div style="width:60px;height:3px;background:var(--border);border-radius:1px;overflow:hidden">'+
          '<div style="width:'+Math.min(conf*100,100)+'%;height:100%;background:'+col+'"></div>'+
        '</div>'+
        '<span style="font-size:8px;color:'+col+';width:30px;text-align:right">'+pct(conf)+'</span>'+
      '</div>';
    }).join('');
  }

  // Arrival rates
  const rp=el('telem-rates');
  if(rp){
    const ids=Object.keys(state.svcs).sort((a,b)=>{
      const ra=(state.svcs[b]&&state.svcs[b].queue&&state.svcs[b].queue.arrival_rate)||0;
      const rb=(state.svcs[a]&&state.svcs[a].queue&&state.svcs[a].queue.arrival_rate)||0;
      return ra-rb;
    });
    rp.innerHTML=ids.slice(0,12).map(id=>{
      const b=state.svcs[id]||{};
      const λ=(b.queue&&b.queue.arrival_rate)||0;
      const rho=(b.queue&&b.queue.utilisation)||0;
      const maxλ=Math.max(...Object.values(state.svcs).map(s=>(s.queue&&s.queue.arrival_rate)||0),1);
      const col=rho>.85?'var(--crit)':rho>.7?'var(--warn)':'var(--ok)';
      return '<div class="mbar-wrap"><div class="mbar-label">'+esc(id)+'</div>'+
        '<div class="mbar-track"><div class="mbar-fill" style="width:'+Math.min(λ/maxλ*100,100)+'%;background:'+col+'"></div></div>'+
        '<div class="mbar-val" style="color:'+col+'">'+fmt1(λ)+'/s</div></div>';
    }).join('');
  }

  // Degraded signals
  const dp=el('telem-degraded');
  if(dp){
    const deg=state.degraded||[];
    if(deg.length===0){dp.innerHTML='<div style="color:var(--ok);font-size:9px">✓ All signals healthy</div>';}
    else dp.innerHTML='<div style="color:var(--warn);font-size:9px;margin-bottom:6px">'+deg.length+' service(s) with degraded signals:</div>'+
      deg.map(id=>'<div style="padding:3px 6px;border:1px solid var(--warn);border-radius:3px;margin-bottom:3px;font-size:9px;color:var(--warn)">⚠ '+esc(id)+'</div>').join('');
  }
}

// ── HEALTH TAB ────────────────────────────────────────────────────────────────
function renderHealthTab(){
  const rm=state.runtimeMetrics||{};
  const stgs=[
    ['prune',rm.avg_prune_ms],['windows',rm.avg_windows_ms],['topology',rm.avg_topology_ms],
    ['coupling',rm.avg_coupling_ms],['modelling',rm.avg_modelling_ms],['optimise',rm.avg_optimise_ms],
    ['sim',rm.avg_sim_ms],['reasoning',rm.avg_reasoning_ms],['broadcast',rm.avg_broadcast_ms]
  ];
  const maxMs=Math.max(...stgs.map(s=>s[1]||0),1);
  const predMs=rm.predicted_critical_ms||0;

  const hp=el('health-stages');
  if(hp){
    hp.innerHTML='<div style="font-size:8px;color:var(--muted);margin-bottom:5px">EWMA per-stage cost (ms): predicted critical='+predMs.toFixed(0)+'ms'+(rm.predicted_overrun?'  <span style="color:var(--warn)">⚡ OVERRUN PREDICTED</span>':'')+'</div>'+
      stgs.map(([name,val])=>{
        const v=val||0;const col=v>100?'var(--crit)':v>50?'var(--warn)':'var(--ok)';
        return '<div class="mbar-wrap"><div class="mbar-label">'+name+'</div>'+
          '<div class="mbar-track"><div class="mbar-fill" style="width:'+Math.min(v/maxMs*100,100)+'%;background:'+col+'"></div></div>'+
          '<div class="mbar-val" style="color:'+col+'">'+v.toFixed(1)+'ms</div></div>';
      }).join('');
  }

  const sp=el('health-safety');
  if(sp){
    const lvl=rm.safety_level||0;
    const lvlCols=['var(--ok)','var(--warn)','var(--crit)','var(--crit)'];
    const lvlLabels=['NOMINAL','ELEVATED','HIGH','CRITICAL'];
    sp.innerHTML=
      '<div style="font-size:20px;font-weight:700;color:'+lvlCols[lvl]+'">LEVEL '+lvl+': '+lvlLabels[lvl]+'</div>'+
      '<div style="margin-top:8px">'+[0,1,2,3].map(i=>{
        const on=lvl>=i;
        return '<div style="display:flex;align-items:center;gap:8px;margin-bottom:4px">'+
          '<div style="width:10px;height:10px;border-radius:2px;background:'+
            (on?lvlCols[i]:'var(--border)')+'"></div>'+
          '<span style="font-size:9px;color:'+(on?lvlCols[i]:'var(--muted)')+'">Level '+i+': '+
            ['Nominal — full operation','Elevated — simFreq 7, persist normal',
             'High — skip persist, simFreq 10','Critical — skip persist+pred, simFreq 15'][i]+
          '</span></div>';
      }).join('')+'</div>';
  }

  const op=el('health-overrun');
  if(op){
    op.innerHTML=
      kv('Total Overruns',rm.total_overruns||0)+
      kv('Consec Overruns',rm.consec_overruns||0)+
      kv('Predicted Critical',predMs.toFixed(1)+'ms')+
      kv('Predicted Overrun',rm.predicted_overrun?'YES':'NO',rm.predicted_overrun?'var(--warn)':'var(--ok)')+
      kv('Jitter',ms(state.jitterMs))+
      kv('Safety Mode',state.safetyMode?'ACTIVE':'INACTIVE',state.safetyMode?'var(--crit)':'var(--ok)');
  }
}

// ── Shared helper: key-value row ──────────────────────────────────────────────
function kv(label,val,col){
  return '<div style="display:flex;justify-content:space-between;align-items:baseline;'+
    'padding:2px 0;border-bottom:1px solid var(--border2);font-size:9px">'+
    '<span style="color:var(--muted)">'+label+'</span>'+
    '<span style="font-weight:600;color:'+(col||'var(--text)')+'">'+val+'</span></div>';
}

// ── Topology render on arbitrary context ─────────────────────────────────────
function renderTopoOnCtx(ctx,W,H,nodes,edges,usePos2){
  // Same physics as main renderTopo but uses global topoPos
  ctx.clearRect(0,0,W,H);
  // Pressure heatmap halos
  nodes.forEach(n=>{
    const p=state.pressureHeatmap[n.service_id]||0;
    if(p<0.25)return;
    const pos=topoPos[n.service_id];if(!pos)return;
    const r=16+p*20;
    const col=p>.8?'rgba(239,68,68,':'rgba(245,158,11,';
    const alpha=Math.min((p-0.25)/0.75*0.5,0.5);
    const g=ctx.createRadialGradient(pos.x,pos.y,0,pos.x,pos.y,r);
    g.addColorStop(0,col+alpha+')');g.addColorStop(1,col+'0)');
    ctx.beginPath();ctx.arc(pos.x,pos.y,r,0,Math.PI*2);ctx.fillStyle=g;ctx.fill();
  });
  // Edges
  edges.forEach(e=>{
    const s=topoPos[e.source],t=topoPos[e.target];
    if(!s||!t)return;
    const cp=state.topo&&state.topo.critical_path;
    const isCrit=cp&&cp.nodes&&cp.nodes.includes(e.source)&&cp.nodes.includes(e.target);
    ctx.beginPath();ctx.moveTo(s.x,s.y);ctx.lineTo(t.x,t.y);
    ctx.strokeStyle=isCrit?'rgba(239,68,68,.7)':'rgba(24,36,64,.9)';
    ctx.lineWidth=isCrit?2:1.5;ctx.stroke();
  });
  // Nodes
  nodes.forEach(n=>{
    const pos=topoPos[n.service_id];if(!pos)return;
    const rho=n.normalised_load||0;
    const col=rho>.9?'#ef4444':rho>.7?'#f59e0b':'#10b981';
    ctx.beginPath();ctx.arc(pos.x,pos.y,7,0,Math.PI*2);
    ctx.fillStyle=col+'22';ctx.fill();
    ctx.strokeStyle=col;ctx.lineWidth=1.5;ctx.stroke();
    ctx.fillStyle='rgba(226,232,240,.9)';ctx.font='7px monospace';ctx.textAlign='center';
    ctx.fillText(n.service_id.slice(0,8),pos.x,pos.y+16);
  });
}


</script>
</body>
</html>
`)
