'use strict';

const state = { groups: [], apps: [], selectedGroupId: null, ws: null };

// ── API ────────────────────────────────────────────────────────────────────
async function api(method, path, body) {
  const opts = { method, headers: { 'Content-Type': 'application/json' } };
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch('/api' + path, opts);
  if (!res.ok) throw new Error(await res.text());
  if (res.status === 204) return null;
  return res.json();
}

// ── Bootstrap ──────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('btn-new-group').addEventListener('click', showCreateGroupModal);
  document.getElementById('group-list').addEventListener('click', (e) => {
    const item = e.target.closest('.group-item');
    if (item && item.dataset.id) selectGroup(item.dataset.id);
  });
  document.getElementById('modal-close').addEventListener('click', closeModal);
  document.getElementById('modal-overlay').addEventListener('click', e => {
    if (e.target === document.getElementById('modal-overlay')) closeModal();
  });
  connectWS();
  loadStats();
  loadGroups();
});

// ── Groups ─────────────────────────────────────────────────────────────────
async function loadGroups() {
  try { const d = await api('GET', '/groups'); state.groups = Array.isArray(d) ? d : []; }
  catch { state.groups = []; }
  renderGroupList();
}

function renderGroupList() {
  const el = document.getElementById('group-list');
  if (!state.groups || !state.groups.length) {
    el.innerHTML = '<div style="padding:12px 16px;color:#8892aa;font-size:13px">No groups yet</div>';
    return;
  }
  el.innerHTML = state.groups.map(g => `
    <div class="group-item ${g.id === state.selectedGroupId ? 'active' : ''}" data-id="${g.id}">
      <div class="group-name">${esc(g.name)}</div>
      ${g.description ? `<div class="group-desc">${esc(g.description)}</div>` : ''}
    </div>
  `).join('');
}

async function selectGroup(id) {
  state.selectedGroupId = id;
  renderGroupList();
  const grp = state.groups.find(g => g.id === id);
  if (!grp) { clearAppsPanel(); return; }
  try { await loadApps(id); } catch { state.apps = []; }
  renderAppsPanel(grp);
}

function showCreateGroupModal() {
  openModal('New Group', `
    <div class="form-group"><label>Name</label><input id="f-group-name" placeholder="e.g. site-a" /></div>
    <div class="form-group"><label>Description</label><input id="f-group-desc" placeholder="Optional" /></div>
  `, async () => {
    const name = document.getElementById('f-group-name').value.trim();
    if (!name) return;
    await api('POST', '/groups', { name, description: document.getElementById('f-group-desc').value });
    closeModal();
    await loadGroups();
  }, 'Create');
}

function showEditGroupModal(groupId) {
  const grp = state.groups.find(g => g.id === groupId);
  if (!grp) return;
  openModal('Edit Group', `
    <div class="form-group"><label>Name</label><input id="f-group-name" value="${esc(grp.name)}" /></div>
    <div class="form-group"><label>Description</label><input id="f-group-desc" value="${esc(grp.description || '')}" /></div>
  `, async () => {
    await api('PUT', '/groups/' + groupId, {
      name: document.getElementById('f-group-name').value.trim(),
      description: document.getElementById('f-group-desc').value,
    });
    closeModal();
    await loadGroups();
    if (state.selectedGroupId === groupId) {
      const updated = state.groups.find(g => g.id === groupId);
      if (updated) renderAppsPanel(updated);
    }
  }, 'Save');
}

async function deleteGroup(id) {
  if (!confirm('Delete this group and all its apps?')) return;
  await api('DELETE', '/groups/' + id).catch(e => alert(e.message));
  state.selectedGroupId = null;
  clearAppsPanel();
  await loadGroups();
}

// ── Apps ───────────────────────────────────────────────────────────────────
async function loadApps(groupId) {
  try { const d = await api('GET', '/groups/' + groupId + '/apps'); state.apps = Array.isArray(d) ? d : []; }
  catch { state.apps = []; }
}

function renderAppsPanel(grp) {
  if (!grp) return;
  document.getElementById('content').innerHTML = `
    <div class="card">
      <div class="card-header">
        <h2>${esc(grp.name)} — Apps</h2>
        <button class="btn btn-primary" onclick="showCreateAppModal('${grp.id}')">+ New App</button>
        <button class="btn btn-secondary" style="margin-left:4px" onclick="showEditGroupModal('${grp.id}')">Edit Group</button>
        <button class="btn btn-danger" style="margin-left:4px" onclick="deleteGroup('${grp.id}')">Delete Group</button>
      </div>
      <div class="card-body" id="apps-grid-container">${renderAppsGrid(grp.id)}</div>
    </div>
    <div class="card">
      <div class="card-header"><h2>Live Transfers</h2></div>
      <div style="overflow-x:auto">
        <table id="feed-table">
          <thead><tr><th>Time</th><th>App</th><th>Object Key</th><th>Size</th><th>Status</th><th>Duration</th></tr></thead>
          <tbody id="feed-body"></tbody>
        </table>
      </div>
    </div>`;
}

function renderAppsGrid(groupId) {
  if (!state.apps || !state.apps.length)
    return '<div class="empty-state"><strong>No apps yet</strong><p>Create an app to start receiving webhooks.</p></div>';
  return state.apps.map(app => `
    <div class="app-card">
      <h3>${esc(app.name)} ${app.enabled ? '' : '<span style="color:#999;font-weight:400">(disabled)</span>'}</h3>
      <div class="app-meta">${esc(app.description || '')}</div>
      <div style="font-size:12px;color:#555;margin-bottom:10px">
        <b>Src:</b> ${esc(app.src?.endpoint || '')} / ${esc(app.src?.bucket || '')}<br>
        <b>Dst:</b> ${esc(app.dst?.endpoint || '')} / ${esc(app.dst?.bucket || '')}
      </div>
      <div class="app-card-actions">
        <button class="btn btn-secondary" onclick="showWebhookInfo('${app.id}')">Webhook URL</button>
        <button class="btn btn-secondary" onclick="showEditAppModal('${app.id}')">Edit</button>
        <button class="btn btn-secondary" onclick="toggleApp('${app.id}', ${!app.enabled})">${app.enabled ? 'Disable' : 'Enable'}</button>
        <button class="btn btn-danger" onclick="deleteApp('${app.id}', '${groupId}')">Delete</button>
      </div>
    </div>
  `).join('');
}

function clearAppsPanel() {
  document.getElementById('content').innerHTML = `
    <div class="empty-state" style="margin-top:60px">
      <strong>Select a group</strong>
      <p>Choose a group from the left sidebar or create a new one.</p>
    </div>`;
}

function appFormHTML(app) {
  const s = app?.src || {}; const d = app?.dst || {};
  const isEdit = !!app;
  return `
    <div class="form-group"><label>App Name</label><input id="f-app-name" value="${esc(app?.name || '')}" placeholder="e.g. sensor-data" /></div>
    <div class="form-group"><label>Description</label><input id="f-app-desc" value="${esc(app?.description || '')}" placeholder="Optional" /></div>
    <div class="form-section-label">Source MinIO</div>
    <div class="form-row">
      <div class="form-group"><label>Endpoint</label><input id="f-src-endpoint" value="${esc(s.endpoint || '')}" placeholder="minio-src:9000" /></div>
      <div class="form-group"><label>Bucket</label><input id="f-src-bucket" value="${esc(s.bucket || '')}" /></div>
    </div>
    <div class="form-row">
      <div class="form-group"><label>Access Key</label><input id="f-src-access" value="${esc(s.access_key || '')}" /></div>
      <div class="form-group"><label>Secret Key</label><input id="f-src-secret" type="password" placeholder="${isEdit ? 'Leave blank to keep current' : ''}" /></div>
    </div>
    <div class="form-row">
      <div class="form-group"><label>Region</label><input id="f-src-region" value="${esc(s.region || 'us-east-1')}" /></div>
      <div class="form-group"><label>Use SSL</label><select id="f-src-ssl"><option value="false" ${!s.use_ssl ? 'selected' : ''}>No</option><option value="true" ${s.use_ssl ? 'selected' : ''}>Yes</option></select></div>
    </div>
    <div class="form-section-label">Destination MinIO</div>
    <div class="form-row">
      <div class="form-group"><label>Endpoint</label><input id="f-dst-endpoint" value="${esc(d.endpoint || '')}" placeholder="minio-dst:9000" /></div>
      <div class="form-group"><label>Bucket</label><input id="f-dst-bucket" value="${esc(d.bucket || '')}" /></div>
    </div>
    <div class="form-row">
      <div class="form-group"><label>Access Key</label><input id="f-dst-access" value="${esc(d.access_key || '')}" /></div>
      <div class="form-group"><label>Secret Key</label><input id="f-dst-secret" type="password" placeholder="${isEdit ? 'Leave blank to keep current' : ''}" /></div>
    </div>
    <div class="form-row">
      <div class="form-group"><label>Region</label><input id="f-dst-region" value="${esc(d.region || 'us-east-1')}" /></div>
      <div class="form-group"><label>Use SSL</label><select id="f-dst-ssl"><option value="false" ${!d.use_ssl ? 'selected' : ''}>No</option><option value="true" ${d.use_ssl ? 'selected' : ''}>Yes</option></select></div>
    </div>`;
}

function collectAppForm() {
  const srcSecret = document.getElementById('f-src-secret').value;
  const dstSecret = document.getElementById('f-dst-secret').value;
  const body = {
    name: document.getElementById('f-app-name').value.trim(),
    description: document.getElementById('f-app-desc').value,
    src: {
      endpoint: document.getElementById('f-src-endpoint').value.trim(),
      access_key: document.getElementById('f-src-access').value,
      bucket: document.getElementById('f-src-bucket').value.trim(),
      region: document.getElementById('f-src-region').value || 'us-east-1',
      use_ssl: document.getElementById('f-src-ssl').value === 'true',
    },
    dst: {
      endpoint: document.getElementById('f-dst-endpoint').value.trim(),
      access_key: document.getElementById('f-dst-access').value,
      bucket: document.getElementById('f-dst-bucket').value.trim(),
      region: document.getElementById('f-dst-region').value || 'us-east-1',
      use_ssl: document.getElementById('f-dst-ssl').value === 'true',
    },
  };
  if (srcSecret) body.src.secret_key = srcSecret;
  if (dstSecret) body.dst.secret_key = dstSecret;
  return body;
}

function showCreateAppModal(groupId) {
  openModal('New App', appFormHTML(null), async () => {
    const body = collectAppForm();
    if (!body.name) return;
    body.src.secret_key = document.getElementById('f-src-secret').value;
    body.dst.secret_key = document.getElementById('f-dst-secret').value;
    await api('POST', '/groups/' + groupId + '/apps', body).catch(e => alert(e.message));
    closeModal();
    await reloadCurrentGroup();
  }, 'Create App');
}

function showEditAppModal(appId) {
  const app = state.apps.find(a => a.id === appId);
  if (!app) return;
  openModal('Edit App', appFormHTML(app), async () => {
    const body = collectAppForm();
    await api('PUT', '/apps/' + appId, body).catch(e => { alert(e.message); return; });
    closeModal();
    await reloadCurrentGroup();
  }, 'Save');
}

async function showWebhookInfo(appId) {
  const data = await api('GET', '/apps/' + appId + '/webhook-url').catch(e => { alert(e.message); return null; });
  if (!data) return;
  const webhookURL = `${location.origin}/webhook/${getSelectedGroupName()}/${getAppName(appId)}`;
  openModal('Webhook Configuration', `
    <p style="margin-bottom:14px;color:#555">Configure your source MinIO to send webhook events to this URL:</p>
    <div class="form-group"><label>Webhook URL</label><div class="code-box">${esc(webhookURL)}</div></div>
    <div class="form-group" style="margin-top:12px"><label>Authorization Token</label>
      <div class="code-box">${esc(data.webhook_secret)}</div>
    </div>
    <p style="margin-top:12px;font-size:12px;color:#888">
      <code>mc admin config set &lt;alias&gt; notify_webhook:&lt;id&gt; endpoint="${esc(webhookURL)}" auth_token="${esc(data.webhook_secret)}"</code>
    </p>
  `, null, null);
}

function getSelectedGroupName() {
  const g = state.groups.find(g => g.id === state.selectedGroupId);
  return g ? g.name : '';
}
function getAppName(appId) {
  const a = state.apps.find(a => a.id === appId);
  return a ? a.name : '';
}

async function toggleApp(appId, enabled) {
  await api('PUT', '/apps/' + appId, { enabled }).catch(e => alert(e.message));
  await reloadCurrentGroup();
}

async function deleteApp(appId, groupId) {
  if (!confirm('Delete this app?')) return;
  await api('DELETE', '/apps/' + appId).catch(e => alert(e.message));
  await reloadCurrentGroup();
}

async function reloadCurrentGroup() {
  if (!state.selectedGroupId) return;
  await loadApps(state.selectedGroupId);
  const grp = state.groups.find(g => g.id === state.selectedGroupId);
  if (grp) renderAppsPanel(grp);
}

// ── Stats ──────────────────────────────────────────────────────────────────
async function loadStats() {
  const stats = await api('GET', '/stats').catch(() => null);
  if (!stats) return;
  updateStats(stats);
}

function updateStats(stats) {
  document.getElementById('stat-total').textContent  = stats.total_transfers  ?? 0;
  document.getElementById('stat-success').textContent = stats.success_count   ?? 0;
  document.getElementById('stat-failed').textContent  = stats.failed_count    ?? 0;
  document.getElementById('stat-pending').textContent = (stats.pending_count ?? 0) + (stats.in_progress_count ?? 0);
  document.getElementById('stat-bytes').textContent   = fmtBytes(stats.total_bytes ?? 0);
}

// ── WebSocket ──────────────────────────────────────────────────────────────
function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(`${proto}//${location.host}/ws`);
  state.ws = ws;

  ws.onopen = () => setWSDot('connected', 'Connected');
  ws.onclose = () => { setWSDot('', 'Disconnected'); setTimeout(connectWS, 3000); };
  ws.onerror = () => setWSDot('error', 'Error');

  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.type === 'ping') { ws.send(JSON.stringify({ type: 'pong' })); return; }
    if (msg.type === 'stats:update') { updateStats(msg.payload); return; }
    if (msg.type.startsWith('transfer:')) { prependFeedRow(msg); return; }

    // Real-time CRUD updates
    if (msg.type.startsWith('group:')) {
      loadGroups().then(() => {
        if (msg.type === 'group:deleted' && msg.payload?.id === state.selectedGroupId) {
          state.selectedGroupId = null;
          clearAppsPanel();
        } else if (state.selectedGroupId) {
          const grp = state.groups.find(g => g.id === state.selectedGroupId);
          if (grp) renderAppsPanel(grp);
        }
      });
      return;
    }
    if (msg.type.startsWith('app:')) {
      const gid = msg.payload?.group_id;
      if (gid === state.selectedGroupId) reloadCurrentGroup();
      return;
    }
  };
}

function setWSDot(cls, label) {
  document.getElementById('ws-dot').className = cls;
  document.getElementById('ws-label').textContent = label;
}

function prependFeedRow(msg) {
  const tbody = document.getElementById('feed-body');
  if (!tbody) return;
  const p = msg.payload || {};
  const status = msg.type === 'transfer:queued' ? 'pending'
    : msg.type === 'transfer:started' ? 'in_progress'
    : msg.type === 'transfer:completed' ? 'success' : 'failed';
  const row = document.createElement('tr');
  row.innerHTML = `
    <td>${fmtTime(msg.timestamp)}</td>
    <td>${esc(p.app_id || '–')}</td>
    <td style="max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(p.object_key || '–')}</td>
    <td>${p.bytes ? fmtBytes(p.bytes) : '–'}</td>
    <td><span class="badge badge-${status}">${status.replace('_', ' ')}</span></td>
    <td>${p.duration_ms != null ? p.duration_ms.toFixed(0) + ' ms' : '–'}</td>`;
  tbody.insertBefore(row, tbody.firstChild);
  while (tbody.children.length > 200) tbody.removeChild(tbody.lastChild);
}

// ── Modal ──────────────────────────────────────────────────────────────────
function openModal(title, bodyHTML, onConfirm, confirmLabel) {
  document.getElementById('modal-header').querySelector('h3').textContent = title;
  document.getElementById('modal-body').innerHTML = bodyHTML;
  const footer = document.getElementById('modal-footer');
  footer.innerHTML = '';
  if (onConfirm && confirmLabel) {
    const btn = document.createElement('button');
    btn.className = 'btn btn-primary'; btn.textContent = confirmLabel; btn.onclick = onConfirm;
    footer.appendChild(btn);
  }
  const cancel = document.createElement('button');
  cancel.className = 'btn btn-secondary'; cancel.textContent = onConfirm ? 'Cancel' : 'Close'; cancel.onclick = closeModal;
  footer.appendChild(cancel);
  document.getElementById('modal-overlay').classList.add('open');
}
function closeModal() { document.getElementById('modal-overlay').classList.remove('open'); }

// ── Utilities ──────────────────────────────────────────────────────────────
function esc(s) { return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;'); }
function fmtBytes(n) {
  if (n == null || n === 0) return '0 B';
  const u = ['B','KB','MB','GB','TB']; let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return n.toFixed(i === 0 ? 0 : 1) + ' ' + u[i];
}
function fmtTime(ts) { return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }); }
