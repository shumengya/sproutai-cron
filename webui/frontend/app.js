const API = "/api";
const REFRESH_INTERVAL = 15000;

const els = {
  taskBody: document.getElementById("task-body"),
  taskCards: document.getElementById("task-cards"),
  stats: document.getElementById("stats"),
  summaryText: document.getElementById("summary-text"),
  lastRefresh: document.getElementById("last-refresh"),
  filter: document.getElementById("filter"),
  toast: document.getElementById("toast"),
  logDialog: document.getElementById("log-dialog"),
  logTitle: document.getElementById("log-title"),
  logSubtitle: document.getElementById("log-subtitle"),
  logContent: document.getElementById("log-content"),
  sidebar: document.getElementById("sidebar"),
  overlay: document.getElementById("overlay"),
};

let allTasks = [];
let currentLogTaskId = null;
let refreshTimer = null;

const escapeHtml = (str) =>
  String(str ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));

function formatBytes(bytes) {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i += 1;
  }
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function nowText() {
  const d = new Date();
  const pad = (n) => String(n).padStart(2, "0");
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function showToast(message, isError = false) {
  els.toast.hidden = false;
  els.toast.textContent = message;
  els.toast.classList.toggle("is-error", isError);
  clearTimeout(showToast._timer);
  showToast._timer = setTimeout(() => {
    els.toast.hidden = true;
  }, 2800);
}

async function api(path, options = {}) {
  const res = await fetch(`${API}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.detail || data.message || `请求失败 (${res.status})`);
  }
  return data;
}

function statusMarkup(task) {
  if (task.running) {
    return '<span class="status run"><span class="dot"></span>运行中</span>';
  }
  if (task.enabled) {
    return '<span class="status on"><span class="dot"></span>已开启</span>';
  }
  return '<span class="status off"><span class="dot"></span>已关闭</span>';
}

function scheduleMarkup(task) {
  return task.schedule
    ? `<span class="schedule-code">${escapeHtml(task.schedule)}</span>`
    : '<span class="mono">—</span>';
}

function logMarkup(task) {
  const size = formatBytes(task.log_size);
  const updated = task.log_updated
    ? `<span class="muted">${escapeHtml(task.log_updated)}</span>`
    : '<span class="muted">无记录</span>';
  return `<div class="log-meta">${size}</div>${updated}`;
}

function actionButtons(task) {
  const toggleLabel = task.enabled ? "关闭" : "开启";
  const toggleClass = task.enabled ? "btn-danger" : "btn-success";
  return `
    <div class="actions">
      <button type="button" class="btn ${toggleClass}" data-action="toggle" data-id="${escapeHtml(task.task_id)}">${toggleLabel}</button>
      <button type="button" class="btn" data-action="run" data-id="${escapeHtml(task.task_id)}" ${task.running ? "disabled" : ""}>运行</button>
      <button type="button" class="btn" data-action="sync" data-id="${escapeHtml(task.task_id)}">同步</button>
      <button type="button" class="btn" data-action="log" data-id="${escapeHtml(task.task_id)}">日志</button>
    </div>
  `;
}

function nameMarkup(task) {
  return `
    <div class="task-name">${escapeHtml(task.task_id)}</div>
    ${task.description ? `<div class="task-desc">${escapeHtml(task.description)}</div>` : ""}
  `;
}

function renderStats(tasks) {
  const enabled = tasks.filter((t) => t.enabled).length;
  const running = tasks.filter((t) => t.running).length;
  const off = tasks.length - enabled;
  els.stats.innerHTML = `
    <div class="meta-item">
      <span class="label">任务总数</span>
      <span class="value">${tasks.length}</span>
    </div>
    <div class="meta-item is-accent">
      <span class="label">已开启</span>
      <span class="value">${enabled}</span>
    </div>
    <div class="meta-item">
      <span class="label">已关闭</span>
      <span class="value">${off}</span>
    </div>
    <div class="meta-item is-run">
      <span class="label">运行中</span>
      <span class="value">${running}</span>
    </div>
  `;
  els.summaryText.textContent = running
    ? `共 ${tasks.length} 个任务，${enabled} 个已开启，${running} 个正在运行`
    : `共 ${tasks.length} 个任务，${enabled} 个已开启`;
}

function renderTasks(tasks) {
  if (!tasks.length) {
    els.taskBody.innerHTML =
      '<tr><td colspan="5" class="empty">没有匹配的任务</td></tr>';
    els.taskCards.innerHTML = '<div class="empty">没有匹配的任务</div>';
    return;
  }

  els.taskBody.innerHTML = tasks
    .map(
      (task) => `
      <tr>
        <td>${statusMarkup(task)}</td>
        <td>${nameMarkup(task)}</td>
        <td>${scheduleMarkup(task)}</td>
        <td>${logMarkup(task)}</td>
        <td>${actionButtons(task)}</td>
      </tr>
    `
    )
    .join("");

  els.taskCards.innerHTML = tasks
    .map(
      (task) => `
      <article class="task-card">
        <div class="task-card-head">
          <div class="name">${nameMarkup(task)}</div>
          ${statusMarkup(task)}
        </div>
        <div class="task-card-body">
          <div>
            <span class="field-label">调度</span>
            ${scheduleMarkup(task)}
          </div>
          <div>
            <span class="field-label">日志</span>
            <span class="log-meta">${formatBytes(task.log_size)}</span>
            <span class="log-meta muted">${task.log_updated ? escapeHtml(task.log_updated) : "无记录"}</span>
          </div>
        </div>
        ${actionButtons(task)}
      </article>
    `
    )
    .join("");
}

function applyFilter() {
  const q = els.filter.value.trim().toLowerCase();
  const filtered = q
    ? allTasks.filter((t) =>
        t.task_id.toLowerCase().includes(q) ||
        (t.description || "").toLowerCase().includes(q)
      )
    : allTasks;
  renderTasks(filtered);
}

async function loadTasks() {
  try {
    const tasks = await api("/tasks");
    allTasks = tasks;
    renderStats(tasks);
    applyFilter();
    els.lastRefresh.textContent = `已刷新 ${nowText()}`;
  } catch (err) {
    els.taskBody.innerHTML = `<tr><td colspan="5" class="empty">${escapeHtml(err.message)}</td></tr>`;
    els.taskCards.innerHTML = `<div class="empty">${escapeHtml(err.message)}</div>`;
    els.lastRefresh.textContent = `刷新失败 ${nowText()}`;
    showToast(err.message, true);
  }
}

async function handleAction(action, taskId) {
  try {
    if (action === "log") {
      await openLog(taskId);
      return;
    }
    if (action === "run") {
      const res = await api(`/tasks/${encodeURIComponent(taskId)}/run`, { method: "POST" });
      showToast(res.ok === false ? res.message : `已启动 ${taskId}`);
    } else if (action === "sync") {
      const res = await api(`/tasks/${encodeURIComponent(taskId)}/sync-cron`, { method: "POST" });
      showToast(res.message || `已同步 ${taskId} 的 cron`);
    } else if (action === "toggle") {
      await api(`/tasks/${encodeURIComponent(taskId)}/toggle`, { method: "POST" });
      showToast(`已切换 ${taskId} 状态`);
    }
    await loadTasks();
  } catch (err) {
    showToast(err.message, true);
  }
}

async function openLog(taskId) {
  currentLogTaskId = taskId;
  els.logTitle.textContent = taskId;
  els.logSubtitle.textContent = "加载日志…";
  els.logContent.textContent = "";
  els.logDialog.showModal();
  await refreshLog();
}

async function refreshLog() {
  if (!currentLogTaskId) return;
  try {
    const data = await api(`/tasks/${encodeURIComponent(currentLogTaskId)}/log?lines=300`);
    const lineCount = data.content ? data.content.split("\n").length : 0;
    els.logSubtitle.textContent = `最近 ${lineCount} 行`;
    els.logContent.textContent = data.content || "";
    els.logContent.scrollTop = els.logContent.scrollHeight;
  } catch (err) {
    els.logSubtitle.textContent = err.message;
    showToast(err.message, true);
  }
}

function bindActions(root) {
  root.addEventListener("click", (event) => {
    const btn = event.target.closest("[data-action]");
    if (!btn) return;
    handleAction(btn.dataset.action, btn.dataset.id);
  });
}

function closeSidebar() {
  els.sidebar.classList.remove("open");
  els.overlay.hidden = true;
}

document.getElementById("btn-refresh").addEventListener("click", loadTasks);
document.getElementById("btn-refresh-top").addEventListener("click", loadTasks);
document.getElementById("btn-log-refresh").addEventListener("click", refreshLog);
document.getElementById("btn-log-close").addEventListener("click", () => els.logDialog.close());
els.filter.addEventListener("input", applyFilter);

document.getElementById("menu-toggle").addEventListener("click", () => {
  els.sidebar.classList.add("open");
  els.overlay.hidden = false;
});
els.overlay.addEventListener("click", closeSidebar);

els.logDialog.addEventListener("click", (event) => {
  if (event.target === els.logDialog) els.logDialog.close();
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    if (els.logDialog.open) els.logDialog.close();
    else if (els.sidebar.classList.contains("open")) closeSidebar();
  }
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
    event.preventDefault();
    els.filter.focus();
  }
});

bindActions(document.body);

loadTasks();
refreshTimer = setInterval(loadTasks, REFRESH_INTERVAL);
window.addEventListener("beforeunload", () => clearInterval(refreshTimer));
