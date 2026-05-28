const API = '';

async function fetchJSON(path) {
  const resp = await fetch(API + path);
  if (!resp.ok) return null;
  return resp.json();
}

// ── Theme toggle (#7) ─────────────────────────────────

const themeBtn = document.getElementById('theme-btn');
function setTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);
  themeBtn.textContent = theme === 'dark' ? '☀️' : '🌙';
  localStorage.setItem('theme', theme);
}
themeBtn.addEventListener('click', () => {
  const current = document.documentElement.getAttribute('data-theme');
  setTheme(current === 'dark' ? 'light' : 'dark');
});
setTheme(localStorage.getItem('theme') || 'dark');

// ── Tab switching ─────────────────────────────────────

function switchTab(tabName) {
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
  document.querySelector(`[data-tab="${tabName}"]`).classList.add('active');
  document.getElementById(tabName).classList.add('active');
}

document.querySelectorAll('.tab').forEach(tab => {
  tab.addEventListener('click', () => switchTab(tab.dataset.tab));
});

// ── Pipeline progress (#9) ────────────────────────────

const pipelineBar = document.getElementById('pipeline-bar');
const runBtn = document.getElementById('run-btn');
let pollInterval = null;

runBtn.addEventListener('click', async () => {
  runBtn.disabled = true;
  runBtn.textContent = 'A correr...';
  pipelineBar.style.display = '';
  updatePipelineStep('strava');

  await fetch(API + '/api/trigger', { method: 'POST' });

  pollInterval = setInterval(async () => {
    const status = await fetchJSON('/api/status');
    if (!status) return;
    if (status.running) {
      updatePipelineStep(status.step || 'strava');
    } else {
      clearInterval(pollInterval);
      pipelineBar.style.display = 'none';
      runBtn.disabled = false;
      runBtn.textContent = 'Run Now';
      loadDashboard();
      loadPlan();
      loadHealth();
    }
  }, 1500);
});

function updatePipelineStep(currentStep) {
  document.querySelectorAll('.pipe-step').forEach(el => {
    el.classList.remove('active', 'done');
    const steps = ['strava', 'strava-detail', 'claude', 'chart', 'telegram'];
    const curIdx = steps.indexOf(currentStep);
    const elIdx = steps.indexOf(el.dataset.step);
    if (elIdx < curIdx) el.classList.add('done');
    if (elIdx === curIdx) el.classList.add('active');
  });
}

// ── Markdown to HTML ──────────────────────────────────

function md(text) {
  if (!text) return '';
  text = text.replace(/((?:^\|.+\|$\n?)+)/gm, (tableBlock) => {
    const rows = tableBlock.trim().split('\n').filter(r => r.trim());
    if (rows.length < 2) return tableBlock;
    const parseRow = r => r.split('|').slice(1, -1).map(c => c.trim());
    const headers = parseRow(rows[0]);
    const startIdx = rows[1].match(/^[\s|:-]+$/) ? 2 : 1;
    let html = '<table class="md-table"><thead><tr>';
    headers.forEach(h => { html += `<th>${h}</th>`; });
    html += '</tr></thead><tbody>';
    for (let i = startIdx; i < rows.length; i++) {
      const cells = parseRow(rows[i]);
      html += '<tr>';
      cells.forEach(c => { html += `<td>${c.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')}</td>`; });
      html += '</tr>';
    }
    html += '</tbody></table>';
    return html;
  });
  return text
    .replace(/^### (.+)$/gm, '<h3>$1</h3>')
    .replace(/^## (.+)$/gm, '<h2>$1</h2>')
    .replace(/^# (.+)$/gm, '<h1>$1</h1>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/^- (.+)$/gm, '<li>$1</li>')
    .replace(/(<li>.*<\/li>)/gs, '<ul>$1</ul>')
    .replace(/\n{2,}/g, '</p><p>')
    .replace(/\n/g, '<br>')
    .replace(/^(.+)$/, '<p>$1</p>');
}

function formatDuration(seconds) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${String(s).padStart(2, '0')}`;
}

function formatDistance(meters) {
  return (meters / 1000).toFixed(2);
}

function formatDate(dateStr) {
  const d = new Date(dateStr);
  return d.toLocaleDateString('pt-PT', { day: 'numeric', month: 'short', year: 'numeric' });
}

function timeAgo(dateStr) {
  const d = new Date(dateStr);
  const now = new Date();
  const diff = Math.floor((now - d) / 1000);
  if (diff < 60) return 'agora';
  if (diff < 3600) return `${Math.floor(diff/60)}min`;
  if (diff < 86400) return `${Math.floor(diff/3600)}h`;
  return `${Math.floor(diff/86400)}d`;
}

// ── Dashboard Tab ─────────────────────────────────────

async function loadDashboard() {
  const [activity, report, activities] = await Promise.all([
    fetchJSON('/api/activities/latest'),
    fetchJSON('/api/reports/latest'),
    fetchJSON('/api/activities?limit=20'),
  ]);
  renderDashboard(activity, report);
  renderHistory(activities || []);
}

function renderDashboard(activity, report) {
  const panel = document.getElementById('dashboard-content');
  if (!activity) {
    panel.innerHTML = `<div class="empty-state"><div class="icon">🏃</div><p>Ainda sem corridas. Carrega em "Run Now".</p></div>`;
    return;
  }

  const z2pct = activity.splits
    ? Math.round(activity.splits.filter(s => s.avg_hr >= 115 && s.avg_hr <= 135).length / activity.splits.length * 100) : 0;

  // Goal tracking (#13): find max continuous run distance from laps
  const goalKm = 5;
  let maxContinuousKm = 0;
  if (activity.laps && activity.laps.length > 0) {
    let current = 0;
    const paceToSec = p => { if (!p) return 9999; const pts = p.split(':'); return parseInt(pts[0]) * 60 + parseInt(pts[1]); };
    for (const l of activity.laps) {
      if (paceToSec(l.pace) < 660 && l.moving_time <= 300) { // running pace, not warmup
        current += l.distance / 1000;
      } else {
        if (current > maxContinuousKm) maxContinuousKm = current;
        current = 0;
      }
    }
    if (current > maxContinuousKm) maxContinuousKm = current;
  }

  panel.innerHTML = `
    <div class="card">
      <div class="card-title">
        ${activity.name} — ${formatDate(activity.date)}${activity.plan_session ? ` · ${activity.plan_session}` : ''}
        <a href="https://www.strava.com/activities/${activity.strava_id}" target="_blank" class="strava-link" title="Ver no Strava">🔗</a>
      </div>
      <div class="hero-stats">
        <div class="stat"><div class="stat-value">${formatDistance(activity.distance)} km</div><div class="stat-label">Distância</div></div>
        <div class="stat"><div class="stat-value">${formatDuration(activity.moving_time)}</div><div class="stat-label">Tempo</div></div>
        <div class="stat"><div class="stat-value">${activity.avg_pace}/km</div><div class="stat-label">Ritmo</div></div>
        <div class="stat"><div class="stat-value">${Math.round(activity.avg_hr)}</div><div class="stat-label">FC Média</div></div>
      </div>
      <div class="chips">
        <span class="chip ${z2pct >= 60 ? 'good' : 'warn'}">Zona 2: ${z2pct}%</span>
        <span class="chip ${activity.max_hr <= 165 ? 'good' : 'warn'}">FC Max: ${Math.round(activity.max_hr)}</span>
      </div>
    </div>

    ${maxContinuousKm > 0 ? `
    <div class="card">
      <div class="card-title">Objetivo: Correr ${goalKm}km Contínuos</div>
      <div class="progress-bar"><div class="progress-fill" style="width:${Math.min(100, (maxContinuousKm/goalKm)*100)}%"></div></div>
      <div class="progress-label">
        <span>Max contínuo: ${maxContinuousKm.toFixed(2)} km</span>
        <span>${Math.min(100, Math.round((maxContinuousKm/goalKm)*100))}%</span>
      </div>
    </div>` : ''}

    ${activity.splits && activity.splits.length > 0 ? `
    <div class="card"><div class="card-title">Splits — Ritmo & FC</div><div class="chart-container"><canvas id="splits-chart"></canvas></div></div>` : ''}

    ${activity.laps && activity.laps.length > 1 ? `
    <div class="card"><div class="card-title">Etapas (Laps) — Ritmo & FC</div><div class="chart-container"><canvas id="laps-chart"></canvas></div></div>` : ''}

    <div class="card">
      <div class="card-title">Zonas de FC</div>
      <div class="hr-zones-container">
        <div class="hr-donut-wrap"><canvas id="hr-donut"></canvas></div>
        <div class="hr-legend" id="hr-legend"></div>
      </div>
    </div>

    ${report ? `<div class="card"><div class="card-title">Relatório de Coaching</div><div class="report-text">${md(report.report_text)}</div></div>` : ''}
  `;

  if (activity.laps && activity.laps.length > 1) renderLapsChart(activity.laps);
  if (activity.splits && activity.splits.length > 0) renderSplitsChart(activity.splits);
  renderHRZones(activity.hr_zones);
}

function renderSplitsChart(splits) {
  const ctx = document.getElementById('splits-chart');
  if (!ctx) return;
  const labels = splits.map(s => `Km ${s.kilometer}`);
  const paceData = splits.map(s => s.avg_speed > 0 ? 1000 / s.avg_speed : 0);
  const hrData = splits.map(s => s.avg_hr);
  const bgColors = splits.map(s => s.avg_hr > 0 && s.avg_hr < 110 ? 'rgba(76,175,80,0.7)' : 'rgba(66,165,245,0.7)');
  new Chart(ctx, { type: 'bar', data: { labels, datasets: [
    { label: 'Ritmo (s/km)', data: paceData, backgroundColor: bgColors, borderColor: bgColors, borderWidth: 1, yAxisID: 'y', order: 2 },
    { label: 'FC (bpm)', data: hrData, type: 'line', borderColor: 'rgba(255,152,0,1)', backgroundColor: 'rgba(255,152,0,0.1)', borderWidth: 2, pointRadius: 4, fill: false, yAxisID: 'y1', order: 1 },
  ]}, options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { labels: { color: 'var(--text)' } } },
    scales: { x: { ticks: { color: '#aaa' }, grid: { color: 'rgba(255,255,255,0.1)' } },
      y: { position: 'left', title: { display: true, text: 'Ritmo (s/km)', color: '#aaa' }, ticks: { color: '#aaa', callback: v => { const m = Math.floor(v/60); const s = Math.round(v%60); return `${m}:${String(s).padStart(2,'0')}`; } }, grid: { color: 'rgba(255,255,255,0.1)' } },
      y1: { position: 'right', title: { display: true, text: 'FC (bpm)', color: '#aaa' }, ticks: { color: '#aaa' }, grid: { display: false } } } } });
}

function renderLapsChart(laps) {
  const ctx = document.getElementById('laps-chart');
  if (!ctx) return;
  const labels = laps.map(l => `${l.index}`);
  const paceToSec = p => { if (!p) return 0; const pts = p.split(':'); return parseInt(pts[0]) * 60 + parseInt(pts[1]); };
  const paceData = laps.map(l => paceToSec(l.pace));
  const hrData = laps.map(l => l.avg_hr || 0);
  const durations = laps.map(l => l.moving_time);
  const bgColors = laps.map((l, i) => {
    const sec = paceToSec(l.pace);
    if (i === 0 || i === laps.length - 1 || l.moving_time > 240) return 'rgba(76,175,80,0.7)';
    if (sec < 570) return 'rgba(66,165,245,0.7)';
    return 'rgba(255,152,0,0.7)';
  });
  new Chart(ctx, { type: 'bar', data: { labels, datasets: [
    { label: 'Ritmo (s/km)', data: paceData, backgroundColor: bgColors, borderColor: bgColors, borderWidth: 1, yAxisID: 'y', order: 2 },
    { label: 'FC (bpm)', data: hrData, type: 'line', borderColor: 'rgba(244,67,54,1)', backgroundColor: 'rgba(244,67,54,0.1)', borderWidth: 2, pointRadius: 3, fill: false, yAxisID: 'y1', order: 1 },
  ]}, options: { responsive: true, maintainAspectRatio: false,
    plugins: { legend: { labels: { color: 'var(--text)' } }, tooltip: { callbacks: { afterLabel: (c) => c.datasetIndex === 0 ? `Duração: ${formatDuration(durations[c.dataIndex])}` : '' } } },
    scales: { x: { title: { display: true, text: 'Etapa', color: '#aaa' }, ticks: { color: '#aaa' }, grid: { color: 'rgba(255,255,255,0.1)' } },
      y: { position: 'left', title: { display: true, text: 'Ritmo (min/km)', color: '#aaa' }, ticks: { color: '#aaa', callback: v => { const m = Math.floor(v/60); const s = Math.round(v%60); return `${m}:${String(s).padStart(2,'0')}`; } }, grid: { color: 'rgba(255,255,255,0.1)' } },
      y1: { position: 'right', title: { display: true, text: 'FC (bpm)', color: '#aaa' }, ticks: { color: '#aaa' }, grid: { display: false } } } } });
}

function renderHRZones(hrZones) {
  const canvas = document.getElementById('hr-donut');
  const legend = document.getElementById('hr-legend');
  if (!canvas || !legend) return;
  const zoneConfig = [
    { label: 'Z1 Repouso', color: '#4caf50' }, { label: 'Z2 Aeróbio', color: '#42a5f5' },
    { label: 'Z3 Limiar', color: '#ff9800' }, { label: 'Z4 Intenso', color: '#f44336' }, { label: 'Z5 Máximo', color: '#9c27b0' },
  ];
  const zones = (hrZones && hrZones.length > 0) ? hrZones : [];
  if (!zones.length) return;
  const data = zones.map(z => z.percent);
  const colors = zones.map((_, i) => zoneConfig[i]?.color || '#888');
  new Chart(canvas, { type: 'doughnut', data: { labels: zones.map((z, i) => zoneConfig[i]?.label || z.name),
    datasets: [{ data, backgroundColor: colors, borderColor: 'var(--surface)', borderWidth: 3, hoverBorderWidth: 0 }] },
    options: { responsive: true, maintainAspectRatio: true, cutout: '65%',
      plugins: { legend: { display: false }, tooltip: { callbacks: { label: (c) => { const z = zones[c.dataIndex]; return ` ${c.label}: ${z.percent}% (${formatDuration(z.seconds)})`; } } } } } });
  legend.innerHTML = zones.map((z, i) => {
    const cfg = zoneConfig[i] || { label: z.name, color: '#888' };
    const bpm = z.max > 900 ? `${z.min}+` : `${z.min}–${z.max}`;
    const time = z.seconds > 0 ? formatDuration(z.seconds) : '–';
    return `<div class="hr-legend-item"><span class="hr-legend-dot" style="background:${cfg.color}"></span><span class="hr-legend-label">${cfg.label}</span><span class="hr-legend-bpm">${bpm}</span><span class="hr-legend-time">${time}</span><span class="hr-legend-pct" style="color:${cfg.color}">${z.percent}%</span></div>`;
  }).join('');
}

// ── History ───────────────────────────────────────────

function renderHistory(activities, filterWeek) {
  const container = document.getElementById('history');
  let filtered = activities;
  if (filterWeek) filtered = activities.filter(a => a.plan_week === filterWeek);
  if (!filtered.length) { container.innerHTML = filterWeek ? `<div class="card"><p style="color:var(--text-muted);padding:8px">Sem corridas na semana ${filterWeek}.</p></div>` : ''; return; }

  container.innerHTML = `
    <div class="card">
      <div class="card-title">Histórico de Corridas ${filterWeek ? `— Semana ${filterWeek} <button class="btn-clear-filter" id="clear-filter">Limpar filtro</button>` : ''}</div>
      <table class="history-table"><thead><tr>
        <th>Data</th><th>Sessão</th><th>Plano</th><th>Distância</th><th>Tempo</th><th>Ritmo</th><th>FC</th>
      </tr></thead><tbody>
        ${filtered.map(a => `
          <tr class="history-row" data-activity-id="${a.id}" style="cursor:pointer" title="Clica para ver o relatório">
            <td>${formatDate(a.date)}</td>
            <td>${a.name} <a href="https://www.strava.com/activities/${a.strava_id}" target="_blank" class="strava-link" onclick="event.stopPropagation()">🔗</a></td>
            <td>${a.plan_session || '—'}</td>
            <td>${formatDistance(a.distance)} km</td>
            <td>${formatDuration(a.moving_time)}</td>
            <td>${a.avg_pace}/km</td>
            <td>${Math.round(a.avg_hr)} bpm</td>
          </tr>
          <tr class="report-row" id="report-${a.id}" style="display:none">
            <td colspan="7"><div class="report-expand loading"><div class="spinner"></div></div></td>
          </tr>`).join('')}
      </tbody></table>
    </div>`;

  container.querySelectorAll('.history-row').forEach(row => {
    row.addEventListener('click', () => toggleReport(row.dataset.activityId));
  });

  const clearBtn = document.getElementById('clear-filter');
  if (clearBtn) clearBtn.addEventListener('click', () => renderHistory(allActivities));
}

let allActivities = [];

async function toggleReport(activityId) {
  const reportRow = document.getElementById(`report-${activityId}`);
  if (!reportRow) return;
  if (reportRow.style.display !== 'none') { reportRow.style.display = 'none'; return; }
  reportRow.style.display = '';
  const cell = reportRow.querySelector('td');
  const report = await fetchJSON(`/api/reports/${activityId}`);
  cell.innerHTML = report
    ? `<div class="report-expand report-text">${md(report.report_text)}</div>`
    : `<div class="report-expand" style="color:var(--text-muted);padding:12px">Sem relatório para esta corrida.</div>`;
}

// ── Plan Tab ──────────────────────────────────────────

async function loadPlan() {
  const planStatus = await fetchJSON('/api/plan/status');
  renderPlan(planStatus);
}

function sessionIcon(s) { return { done: '✅', missed: '❌', upcoming: '⬜', na: '➖' }[s] || '⬜'; }
function sessionTypeIcon(t) { return t === 'run' ? '🏃' : '💪'; }

function renderPlan(plan) {
  const panel = document.getElementById('plan');
  if (!plan) { panel.innerHTML = '<div class="empty-state"><p>Plano não disponível.</p></div>'; return; }

  panel.innerHTML = `
    <div class="card plan-progress">
      <div class="card-title">${plan.name || 'Plano de Treino'}${plan.goal ? ` — ${plan.goal}` : ''}</div>
      <div class="progress-bar"><div class="progress-fill" style="width:${plan.progress}%"></div></div>
      <div class="progress-label"><span>Semana ${plan.current_week} de ${plan.total_weeks}</span><span>${plan.progress}%</span></div>
    </div>
    <div class="week-grid">
      ${plan.weeks.map(w => `
        <div class="week-card ${w.status === 'current' ? 'week-current' : ''} ${w.status === 'done' ? 'week-clickable' : ''}" ${w.status === 'done' ? `data-week="${w.week}"` : ''}>
          <div class="week-header">
            <span class="week-title">Semana ${w.week}</span>
            <span class="week-status ${w.status}">
              ${w.status === 'done' ? '✅ Concluído' : w.status === 'current' ? '🔵 Em curso' : '⬜ A vir'}
            </span>
          </div>
          <div class="session-list">
            ${w.sessions.map(s => `
              <div class="session-item ${s.status}">
                ${s.type === 'strength'
                  ? `<input type="checkbox" class="strength-check" data-week="${w.week}" ${s.status === 'done' ? 'checked' : ''} title="Marcar como feito">`
                  : `<span class="session-icon">${sessionIcon(s.status)}</span>`}
                <span class="session-type">${sessionTypeIcon(s.type)}</span>
                <span class="session-day">${s.day}</span>
                <span class="session-desc">${s.description}</span>
                ${s.activity_id ? `<span class="session-link" data-id="${s.activity_id}" title="Ver relatório">📊</span>` : ''}
              </div>`).join('')}
          </div>
        </div>`).join('')}
    </div>`;

  // Strength checkboxes
  panel.querySelectorAll('.strength-check').forEach(cb => {
    cb.addEventListener('change', async (e) => {
      e.stopPropagation();
      await fetch(`${API}/api/plan/toggle-strength?week=${cb.dataset.week}`, { method: 'POST' });
      loadPlan();
    });
  });

  // Click completed week → filter history (#8)
  panel.querySelectorAll('.week-clickable').forEach(el => {
    el.style.cursor = 'pointer';
    el.addEventListener('click', (e) => {
      if (e.target.closest('.strength-check, .session-link')) return;
      const week = parseInt(el.dataset.week);
      switchTab('dashboard');
      renderHistory(allActivities, week);
      document.getElementById('history').scrollIntoView({ behavior: 'smooth' });
    });
  });

  // Click 📊 → show report
  panel.querySelectorAll('.session-link').forEach(el => {
    el.style.cursor = 'pointer';
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      switchTab('dashboard');
      toggleReport(el.dataset.id);
      const row = document.getElementById(`report-${el.dataset.id}`);
      if (row) setTimeout(() => row.scrollIntoView({ behavior: 'smooth' }), 300);
    });
  });
}

// ── Health/System Tab (#12) ───────────────────────────

async function loadHealth() {
  const [health, events] = await Promise.all([
    fetchJSON('/api/health/detail'),
    fetchJSON('/api/events'),
  ]);
  renderHealth(health, events);
}

function renderHealth(health, events) {
  const panel = document.getElementById('health');
  if (!health) { panel.innerHTML = '<div class="empty-state"><p>Sem dados.</p></div>'; return; }

  const tokenExpiry = health.strava_token_expires ? new Date(health.strava_token_expires) : null;
  const tokenOk = tokenExpiry && tokenExpiry > new Date();
  const lastRun = health.last_run && health.last_run !== '0001-01-01T00:00:00Z' ? timeAgo(health.last_run) : 'nunca';

  panel.innerHTML = `
    <div class="card">
      <div class="card-title">Estado do Sistema</div>
      <div class="health-grid">
        <div class="health-item">
          <span class="health-icon">${tokenOk ? '🟢' : '🔴'}</span>
          <div><strong>Token Strava</strong><br><span class="health-detail">${tokenExpiry ? `Expira: ${tokenExpiry.toLocaleString('pt-PT')}` : 'Não definido'}</span></div>
        </div>
        <div class="health-item">
          <span class="health-icon">${health.last_result === 'success' ? '🟢' : health.last_result === 'error' ? '🔴' : '⚪'}</span>
          <div><strong>Último pipeline</strong><br><span class="health-detail">${lastRun} — ${health.last_result || 'sem execuções'}</span></div>
        </div>
        <div class="health-item">
          <span class="health-icon">📅</span>
          <div><strong>Plano</strong><br><span class="health-detail">Semana ${health.plan_week} de ${health.plan_total_weeks}</span></div>
        </div>
        <div class="health-item">
          <span class="health-icon">${health.running ? '⏳' : '💤'}</span>
          <div><strong>Pipeline</strong><br><span class="health-detail">${health.running ? `A correr (${health.step || '...'})` : 'Inativo'}</span></div>
        </div>
      </div>
      ${health.last_error ? `<div class="health-error"><strong>Último erro:</strong> ${health.last_error}</div>` : ''}
    </div>

    <div class="card">
      <div class="card-title">Registo de Eventos</div>
      ${events && events.length > 0 ? `
      <div class="events-list">
        ${events.map(e => `
          <div class="event-item ${e.type}">
            <span class="event-icon">${e.type === 'success' ? '✅' : e.type === 'error' ? '❌' : e.type === 'nudge' ? '💬' : 'ℹ️'}</span>
            <span class="event-msg">${e.message}</span>
            <span class="event-time">${timeAgo(e.created_at)}</span>
            ${e.detail ? `<div class="event-detail">${e.detail}</div>` : ''}
          </div>`).join('')}
      </div>` : '<p style="color:var(--text-muted)">Sem eventos registados.</p>'}
    </div>`;
}

// ── Init ──────────────────────────────────────────────

async function init() {
  const [, , activities] = await Promise.all([
    loadDashboard(),
    loadPlan(),
    fetchJSON('/api/activities?limit=50'),
  ]);
  allActivities = activities || [];
  loadHealth();
}

init();
