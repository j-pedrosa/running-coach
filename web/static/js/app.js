const API = '';

async function fetchJSON(path) {
  const resp = await fetch(API + path);
  if (!resp.ok) return null;
  return resp.json();
}

async function fetchText(path) {
  const resp = await fetch(API + path);
  if (!resp.ok) return null;
  return resp.text();
}

// Tab switching
document.querySelectorAll('.tab').forEach(tab => {
  tab.addEventListener('click', () => {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
    tab.classList.add('active');
    document.getElementById(tab.dataset.tab).classList.add('active');
  });
});

// Run Now button
const runBtn = document.getElementById('run-btn');
runBtn.addEventListener('click', async () => {
  runBtn.disabled = true;
  runBtn.textContent = 'A correr...';

  await fetch(API + '/api/trigger', { method: 'POST' });

  const poll = setInterval(async () => {
    const status = await fetchJSON('/api/status');
    if (status && !status.running) {
      clearInterval(poll);
      runBtn.disabled = false;
      runBtn.textContent = 'Run Now';
      loadDashboard();
      loadPlan();
    }
  }, 3000);
});

// Markdown to HTML (lightweight)
function md(text) {
  if (!text) return '';
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

// ── Dashboard Tab ──────────────────────────────────────

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
    panel.innerHTML = `
      <div class="empty-state">
        <div class="icon">🏃</div>
        <p>Ainda sem corridas registadas.</p>
        <p>Carrega em "Run Now" para buscar a última atividade do Strava.</p>
      </div>`;
    return;
  }

  const z2pct = activity.splits
    ? Math.round(activity.splits.filter(s => s.avg_hr >= 115 && s.avg_hr <= 135).length / activity.splits.length * 100)
    : 0;

  panel.innerHTML = `
    <div class="card">
      <div class="card-title">
        ${activity.name} — ${formatDate(activity.date)}${activity.plan_session ? ` · ${activity.plan_session}` : ''}
        <a href="https://www.strava.com/activities/${activity.strava_id}" target="_blank" class="strava-link" title="Ver no Strava">🔗</a>
      </div>
      <div class="hero-stats">
        <div class="stat">
          <div class="stat-value">${formatDistance(activity.distance)} km</div>
          <div class="stat-label">Distância</div>
        </div>
        <div class="stat">
          <div class="stat-value">${formatDuration(activity.moving_time)}</div>
          <div class="stat-label">Tempo</div>
        </div>
        <div class="stat">
          <div class="stat-value">${activity.avg_pace}/km</div>
          <div class="stat-label">Ritmo</div>
        </div>
        <div class="stat">
          <div class="stat-value">${Math.round(activity.avg_hr)}</div>
          <div class="stat-label">FC Média</div>
        </div>
      </div>
      <div class="chips">
        <span class="chip ${z2pct >= 60 ? 'good' : 'warn'}">Zona 2: ${z2pct}%</span>
        <span class="chip ${activity.max_hr <= 165 ? 'good' : 'warn'}">FC Max: ${Math.round(activity.max_hr)}</span>
      </div>
    </div>

    ${activity.splits && activity.splits.length > 0 ? `
    <div class="card">
      <div class="card-title">Splits — Ritmo & FC</div>
      <div class="chart-container">
        <canvas id="splits-chart"></canvas>
      </div>
    </div>
    ` : ''}

    ${activity.laps && activity.laps.length > 1 ? `
    <div class="card">
      <div class="card-title">Etapas (Laps) — Ritmo & FC</div>
      <div class="chart-container">
        <canvas id="laps-chart"></canvas>
      </div>
    </div>
    ` : ''}

    <div class="card">
      <div class="card-title">Zonas de FC</div>
      <div class="hr-zones-container">
        <div class="hr-donut-wrap">
          <canvas id="hr-donut"></canvas>
        </div>
        <div class="hr-legend" id="hr-legend"></div>
      </div>
    </div>

    ${report ? `
    <div class="card">
      <div class="card-title">Relatório de Coaching</div>
      <div class="report-text">${md(report.report_text)}</div>
    </div>
    ` : ''}
  `;

  if (activity.laps && activity.laps.length > 1) {
    renderLapsChart(activity.laps);
  }
  if (activity.splits && activity.splits.length > 0) {
    renderSplitsChart(activity.splits);
  }
  renderHRZones(activity.hr_zones, activity.splits);
}

function renderSplitsChart(splits) {
  const ctx = document.getElementById('splits-chart');
  if (!ctx) return;

  const labels = splits.map(s => `Km ${s.kilometer}`);
  const paceData = splits.map(s => s.avg_speed > 0 ? 1000 / s.avg_speed : 0);
  const hrData = splits.map(s => s.avg_hr);
  const bgColors = splits.map(s => s.avg_hr > 0 && s.avg_hr < 110
    ? 'rgba(76, 175, 80, 0.7)'
    : 'rgba(66, 165, 245, 0.7)');

  new Chart(ctx, {
    type: 'bar',
    data: {
      labels,
      datasets: [
        {
          label: 'Ritmo (s/km)',
          data: paceData,
          backgroundColor: bgColors,
          borderColor: bgColors,
          borderWidth: 1,
          yAxisID: 'y',
          order: 2,
        },
        {
          label: 'FC (bpm)',
          data: hrData,
          type: 'line',
          borderColor: 'rgba(255, 152, 0, 1)',
          backgroundColor: 'rgba(255, 152, 0, 0.1)',
          borderWidth: 2,
          pointRadius: 4,
          fill: false,
          yAxisID: 'y1',
          order: 1,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { labels: { color: '#e0e0e0' } },
      },
      scales: {
        x: {
          ticks: { color: '#aaa' },
          grid: { color: 'rgba(255,255,255,0.1)' },
        },
        y: {
          position: 'left',
          title: { display: true, text: 'Ritmo (s/km)', color: '#aaa' },
          ticks: {
            color: '#aaa',
            callback: v => {
              const m = Math.floor(v / 60);
              const s = Math.round(v % 60);
              return `${m}:${String(s).padStart(2, '0')}`;
            },
          },
          grid: { color: 'rgba(255,255,255,0.1)' },
        },
        y1: {
          position: 'right',
          title: { display: true, text: 'FC (bpm)', color: '#aaa' },
          ticks: { color: '#aaa' },
          grid: { display: false },
        },
      },
    },
  });
}

function renderLapsChart(laps) {
  const ctx = document.getElementById('laps-chart');
  if (!ctx) return;

  const labels = laps.map(l => `${l.index}`);
  // Convert pace "mm:ss" to seconds for Y axis
  const paceToSec = p => {
    if (!p) return 0;
    const parts = p.split(':');
    return parseInt(parts[0]) * 60 + parseInt(parts[1]);
  };
  const paceData = laps.map(l => paceToSec(l.pace));
  const hrData = laps.map(l => l.avg_hr || 0);
  const durations = laps.map(l => l.moving_time);

  // Color: green=warmup/cooldown (first & last, or >4min), blue=run (<9:30/km), orange=walk (>=9:30/km)
  const bgColors = laps.map((l, i) => {
    const sec = paceToSec(l.pace);
    if (i === 0 || i === laps.length - 1 || l.moving_time > 240) {
      return 'rgba(76, 175, 80, 0.7)';  // green — warmup/cooldown
    }
    if (sec < 570) { // faster than 9:30/km
      return 'rgba(66, 165, 245, 0.7)';  // blue — run
    }
    return 'rgba(255, 152, 0, 0.7)';     // orange — walk
  });

  new Chart(ctx, {
    type: 'bar',
    data: {
      labels,
      datasets: [
        {
          label: 'Ritmo (s/km)',
          data: paceData,
          backgroundColor: bgColors,
          borderColor: bgColors,
          borderWidth: 1,
          yAxisID: 'y',
          order: 2,
        },
        {
          label: 'FC (bpm)',
          data: hrData,
          type: 'line',
          borderColor: 'rgba(244, 67, 54, 1)',
          backgroundColor: 'rgba(244, 67, 54, 0.1)',
          borderWidth: 2,
          pointRadius: 3,
          fill: false,
          yAxisID: 'y1',
          order: 1,
        },
      ],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { labels: { color: '#e0e0e0' } },
        tooltip: {
          callbacks: {
            afterLabel: (ctx) => {
              if (ctx.datasetIndex === 0) {
                return `Duração: ${formatDuration(durations[ctx.dataIndex])}`;
              }
              return '';
            },
          },
        },
      },
      scales: {
        x: {
          title: { display: true, text: 'Etapa', color: '#aaa' },
          ticks: { color: '#aaa' },
          grid: { color: 'rgba(255,255,255,0.1)' },
        },
        y: {
          position: 'left',
          title: { display: true, text: 'Ritmo (min/km)', color: '#aaa' },
          ticks: {
            color: '#aaa',
            callback: v => {
              const m = Math.floor(v / 60);
              const s = Math.round(v % 60);
              return `${m}:${String(s).padStart(2, '0')}`;
            },
          },
          grid: { color: 'rgba(255,255,255,0.1)' },
        },
        y1: {
          position: 'right',
          title: { display: true, text: 'FC (bpm)', color: '#aaa' },
          ticks: { color: '#aaa' },
          grid: { display: false },
        },
      },
    },
  });
}

function renderHRZones(hrZones) {
  const canvas = document.getElementById('hr-donut');
  const legend = document.getElementById('hr-legend');
  if (!canvas || !legend) return;

  const zoneConfig = [
    { label: 'Z1 Repouso',  color: '#4caf50' },
    { label: 'Z2 Aeróbio',  color: '#42a5f5' },
    { label: 'Z3 Limiar',   color: '#ff9800' },
    { label: 'Z4 Intenso',  color: '#f44336' },
    { label: 'Z5 Máximo',   color: '#9c27b0' },
  ];

  const zones = (hrZones && hrZones.length > 0) ? hrZones : [];
  if (!zones.length) return;

  const data = zones.map(z => z.percent);
  const colors = zones.map((_, i) => zoneConfig[i]?.color || '#888');

  new Chart(canvas, {
    type: 'doughnut',
    data: {
      labels: zones.map((z, i) => zoneConfig[i]?.label || z.name),
      datasets: [{
        data,
        backgroundColor: colors,
        borderColor: '#1a1a2e',
        borderWidth: 3,
        hoverBorderWidth: 0,
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: true,
      cutout: '65%',
      plugins: {
        legend: { display: false },
        tooltip: {
          callbacks: {
            label: (ctx) => {
              const z = zones[ctx.dataIndex];
              return ` ${ctx.label}: ${z.percent}% (${formatDuration(z.seconds)})`;
            },
          },
        },
      },
    },
  });

  // Custom legend with time
  legend.innerHTML = zones.map((z, i) => {
    const cfg = zoneConfig[i] || { label: z.name, color: '#888' };
    const bpm = z.max > 900 ? `${z.min}+` : `${z.min}–${z.max}`;
    const time = z.seconds > 0 ? formatDuration(z.seconds) : '–';
    return `
      <div class="hr-legend-item">
        <span class="hr-legend-dot" style="background:${cfg.color}"></span>
        <span class="hr-legend-label">${cfg.label}</span>
        <span class="hr-legend-bpm">${bpm}</span>
        <span class="hr-legend-time">${time}</span>
        <span class="hr-legend-pct" style="color:${cfg.color}">${z.percent}%</span>
      </div>`;
  }).join('');
}

// ── History with click-to-expand report (#4) ──────────

function renderHistory(activities) {
  const container = document.getElementById('history');
  if (!activities.length) {
    container.innerHTML = '';
    return;
  }

  container.innerHTML = `
    <div class="card">
      <div class="card-title">Histórico de Corridas</div>
      <table class="history-table">
        <thead>
          <tr>
            <th>Data</th>
            <th>Sessão</th>
            <th>Plano</th>
            <th>Distância</th>
            <th>Tempo</th>
            <th>Ritmo</th>
            <th>FC</th>
          </tr>
        </thead>
        <tbody>
          ${activities.map(a => `
            <tr class="history-row" data-activity-id="${a.id}" style="cursor:pointer" title="Clica para ver o relatório">
              <td>${formatDate(a.date)}</td>
              <td>${a.name} <a href="https://www.strava.com/activities/${a.strava_id}" target="_blank" class="strava-link" onclick="event.stopPropagation()" title="Ver no Strava">🔗</a></td>
              <td>${a.plan_session || '—'}</td>
              <td>${formatDistance(a.distance)} km</td>
              <td>${formatDuration(a.moving_time)}</td>
              <td>${a.avg_pace}/km</td>
              <td>${Math.round(a.avg_hr)} bpm</td>
            </tr>
            <tr class="report-row" id="report-${a.id}" style="display:none">
              <td colspan="7">
                <div class="report-expand loading"><div class="spinner"></div></div>
              </td>
            </tr>`).join('')}
        </tbody>
      </table>
    </div>`;

  // Click handlers
  container.querySelectorAll('.history-row').forEach(row => {
    row.addEventListener('click', () => toggleReport(row.dataset.activityId));
  });
}

async function toggleReport(activityId) {
  const reportRow = document.getElementById(`report-${activityId}`);
  if (!reportRow) return;

  if (reportRow.style.display !== 'none') {
    reportRow.style.display = 'none';
    return;
  }

  reportRow.style.display = '';
  const cell = reportRow.querySelector('td');

  const report = await fetchJSON(`/api/reports/${activityId}`);
  if (report) {
    cell.innerHTML = `<div class="report-expand report-text">${md(report.report_text)}</div>`;
  } else {
    cell.innerHTML = `<div class="report-expand" style="color:var(--text-muted);padding:12px">Sem relatório para esta corrida.</div>`;
  }
}

// ── Plan Tab (dynamic, #3 + #5) ───────────────────────

async function loadPlan() {
  const planStatus = await fetchJSON('/api/plan/status');
  renderPlan(planStatus);
}

function sessionIcon(status) {
  switch (status) {
    case 'done': return '✅';
    case 'missed': return '❌';
    case 'upcoming': return '⬜';
    case 'na': return '➖';
    default: return '⬜';
  }
}

function sessionTypeIcon(type) {
  return type === 'run' ? '🏃' : '💪';
}

function renderPlan(plan) {
  const panel = document.getElementById('plan');

  if (!plan) {
    panel.innerHTML = '<div class="empty-state"><p>Plano de treino não disponível.</p></div>';
    return;
  }

  panel.innerHTML = `
    <div class="card plan-progress">
      <div class="card-title">Progresso do Plano</div>
      <div class="progress-bar">
        <div class="progress-fill" style="width:${plan.progress}%"></div>
      </div>
      <div class="progress-label">
        <span>Semana ${plan.current_week} de ${plan.total_weeks}</span>
        <span>${plan.progress}%</span>
      </div>
    </div>

    <div class="week-grid">
      ${plan.weeks.map(w => `
        <div class="week-card ${w.status === 'current' ? 'week-current' : ''}">
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
                  : `<span class="session-icon">${sessionIcon(s.status)}</span>`
                }
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
    cb.addEventListener('change', async () => {
      const week = cb.dataset.week;
      await fetch(`${API}/api/plan/toggle-strength?week=${week}`, { method: 'POST' });
      loadPlan(); // refresh
    });
  });

  // Click on 📊 to switch to dashboard and show report
  panel.querySelectorAll('.session-link').forEach(el => {
    el.style.cursor = 'pointer';
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      // Switch to dashboard tab
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
      document.querySelector('[data-tab="dashboard"]').classList.add('active');
      document.getElementById('dashboard').classList.add('active');
      // Toggle report in history
      toggleReport(el.dataset.id);
      // Scroll to it
      const reportRow = document.getElementById(`report-${el.dataset.id}`);
      if (reportRow) setTimeout(() => reportRow.scrollIntoView({ behavior: 'smooth' }), 300);
    });
  });
}

// ── Init ──────────────────────────────────────────────

loadDashboard();
loadPlan();
