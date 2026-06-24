const API = "/api";
const REFRESH_INTERVAL = 15000;

const els = {
  runtimes: document.getElementById("runtimes"),
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
  editDialog: document.getElementById("edit-dialog"),
  editForm: document.getElementById("edit-form"),
  editTitle: document.getElementById("edit-title"),
  editDescription: document.getElementById("edit-description"),
  editMinute: document.getElementById("edit-minute"),
  editHour: document.getElementById("edit-hour"),
  editDom: document.getElementById("edit-dom"),
  editMonth: document.getElementById("edit-month"),
  editDow: document.getElementById("edit-dow"),
  editPreview: document.getElementById("edit-preview"),
  cronPresets: document.getElementById("cron-presets"),
  editTagListEl: document.getElementById("edit-tags"),
  editTagInput: document.getElementById("edit-tag-input"),
};

let allTasks = [];
let currentLogTaskId = null;
let currentEditTaskId = null;
let editTagList = [];
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
    const detail = data.detail;
    const msg = Array.isArray(detail)
      ? detail.map((d) => d.msg || d).join("; ")
      : detail || data.message || `请求失败 (${res.status})`;
    throw new Error(msg);
  }
  return data;
}

function statusMarkup(task) {
  if (task.running) {
    return '<span class="badge badge-run"><span class="dot"></span>运行中</span>';
  }
  if (task.enabled) {
    return '<span class="badge badge-on"><span class="dot"></span>已开启</span>';
  }
  return '<span class="badge badge-off"><span class="dot"></span>已关闭</span>';
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
      <button type="button" class="btn" data-action="edit" data-id="${escapeHtml(task.task_id)}">修改</button>
      <button type="button" class="btn" data-action="sync" data-id="${escapeHtml(task.task_id)}">同步</button>
      <button type="button" class="btn" data-action="log" data-id="${escapeHtml(task.task_id)}">日志</button>
    </div>
  `;
}

const CRON_FIELD_KEYS = ["editMinute", "editHour", "editDom"];

function parseCron(expr) {
  const parts = (expr || "0 8 * * *").trim().split(/\s+/);
  return {
    minute: parts[0] ?? "0",
    hour: parts[1] ?? "8",
    dom: parts[2] ?? "*",
    month: parts[3] ?? "*",
    dow: parts[4] ?? "*",
  };
}

const CRON_DOW_OPTIONS = [
  ["*", "每"],
  ["0", "周日"],
  ["1", "周一"],
  ["2", "周二"],
  ["3", "周三"],
  ["4", "周四"],
  ["5", "周五"],
  ["6", "周六"],
];

const CRON_MONTH_OPTIONS = [
  ["*", "每"],
  ...Array.from({ length: 12 }, (_, i) => [String(i + 1), `${i + 1}月`]),
];

function resetSelectOptions(selectEl, options, value) {
  selectEl.innerHTML = options.map(
    ([val, label]) => `<option value="${val}">${label}</option>`
  ).join("");
  if ([...selectEl.options].some((opt) => opt.value === value)) {
    selectEl.value = value;
    return;
  }
  const opt = document.createElement("option");
  opt.value = value;
  opt.textContent = value;
  opt.selected = true;
  selectEl.appendChild(opt);
}

function resetDowSelect(value) {
  resetSelectOptions(els.editDow, CRON_DOW_OPTIONS, value);
}

function resetMonthSelect(value) {
  resetSelectOptions(els.editMonth, CRON_MONTH_OPTIONS, value);
}

function fillCronForm(expr) {
  const { minute, hour, dom, month, dow } = parseCron(expr);
  els.editMinute.value = minute;
  els.editHour.value = hour;
  els.editDom.value = dom;
  resetMonthSelect(month);
  resetDowSelect(dow);
}

function buildScheduleFromForm() {
  const values = CRON_FIELD_KEYS.map((key) => {
    const raw = els[key].value.trim();
    return raw === "" ? "*" : raw;
  });
  values.push(els.editMonth.value.trim() || "*");
  values.push(els.editDow.value.trim() || "*");
  return values.join(" ");
}

function updateEditPreview() {
  try {
    els.editPreview.textContent = buildScheduleFromForm() || "—";
  } catch {
    els.editPreview.textContent = "—";
  }
}

const MAX_TASK_TAGS = 4;

const RUNTIME_LABELS = {
  python: "Python",
  javascript: "JavaScript",
  bash: "Bash",
  powershell: "PowerShell",
};

function runtimeLabel(task) {
  return RUNTIME_LABELS[task.runtime] || task.runtime || "Python";
}

function displayTags(task) {
  const tags = Array.isArray(task.tags) ? task.tags.filter(Boolean) : [];
  if (tags.length) return tags.slice(0, MAX_TASK_TAGS);
  return [runtimeLabel(task)];
}

function tagsMarkup(task) {
  return displayTags(task)
    .map((tag) => `<span class="task-tag">${escapeHtml(tag)}</span>`)
    .join("");
}

function nameMarkup(task) {
  return `
    <div class="task-name-row">
      <span class="task-name">${escapeHtml(task.task_id)}</span>
      ${tagsMarkup(task)}
    </div>
    ${task.description ? `<div class="task-desc">${escapeHtml(task.description)}</div>` : ""}
  `;
}

function renderRuntimes(items) {
  els.runtimes.innerHTML = items
    .map((item) => {
      const version = item.available
        ? escapeHtml(item.version || "—")
        : '<span class="runtime-missing">无</span>';
      const stateClass = item.available ? "is-ok" : "is-missing";
      return `
        <div class="runtime-card ${stateClass}">
          <span class="runtime-name">${escapeHtml(item.name)}</span>
          <span class="runtime-version">${version}</span>
        </div>
      `;
    })
    .join("");
}

async function loadRuntimes() {
  try {
    const items = await api("/runtimes");
    renderRuntimes(items);
  } catch {
    els.runtimes.innerHTML = '<div class="runtime-card is-missing"><span class="runtime-name">运行时</span><span class="runtime-version"><span class="runtime-missing">检测失败</span></span></div>';
  }
}

function renderStats(tasks) {
  const enabled = tasks.filter((t) => t.enabled).length;
  const running = tasks.filter((t) => t.running).length;
  const off = tasks.length - enabled;
  els.stats.innerHTML = `
    <div class="stat-card">
      <span class="stat-label">任务总数</span>
      <span class="stat-value">${tasks.length}</span>
    </div>
    <div class="stat-card stat-accent">
      <span class="stat-label">已开启</span>
      <span class="stat-value">${enabled}</span>
    </div>
    <div class="stat-card">
      <span class="stat-label">已关闭</span>
      <span class="stat-value">${off}</span>
    </div>
    <div class="stat-card stat-run">
      <span class="stat-label">运行中</span>
      <span class="stat-value">${running}</span>
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
        <div class="task-card-meta">
          <div class="meta-block">
            <span class="field-label">调度</span>
            ${scheduleMarkup(task)}
          </div>
          <div class="meta-block">
            <span class="field-label">日志</span>
            <div class="log-meta">${formatBytes(task.log_size)}</div>
            <div class="log-meta muted">${task.log_updated ? escapeHtml(task.log_updated) : "无记录"}</div>
          </div>
        </div>
        <div class="task-card-actions">${actionButtons(task)}</div>
      </article>
    `
    )
    .join("");
}

function applyFilter() {
  const q = els.filter.value.trim().toLowerCase();
  const filtered = q
    ? allTasks.filter((t) => {
        const tagText = displayTags(t).join(" ").toLowerCase();
        return (
          t.task_id.toLowerCase().includes(q) ||
          (t.description || "").toLowerCase().includes(q) ||
          tagText.includes(q)
        );
      })
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
    await loadRuntimes();
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
    if (action === "edit") {
      openEdit(taskId);
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

function renderEditTags() {
  if (!els.editTagListEl) return;
  els.editTagListEl.innerHTML = editTagList
    .map(
      (tag, index) => `
      <span class="task-tag is-editable">
        ${escapeHtml(tag)}
        <button type="button" class="tag-remove" data-index="${index}" aria-label="移除 ${escapeHtml(tag)}">×</button>
      </span>
    `
    )
    .join("");
  const atLimit = editTagList.length >= MAX_TASK_TAGS;
  if (els.editTagInput) els.editTagInput.disabled = atLimit;
  const addBtn = document.getElementById("btn-tag-add");
  if (addBtn) addBtn.disabled = atLimit;
}

function addEditTag() {
  if (!els.editTagInput) return;
  const value = els.editTagInput.value.trim();
  if (!value) return;
  if (editTagList.length >= MAX_TASK_TAGS) {
    showToast(`最多 ${MAX_TASK_TAGS} 个标签`, true);
    return;
  }
  if (editTagList.includes(value)) {
    showToast("标签已存在", true);
    return;
  }
  editTagList.push(value);
  els.editTagInput.value = "";
  renderEditTags();
}

function removeEditTag(index) {
  if (index < 0 || index >= editTagList.length) return;
  editTagList.splice(index, 1);
  renderEditTags();
}

function openEdit(taskId) {
  const task = allTasks.find((t) => t.task_id === taskId);
  if (!task) {
    showToast("任务不存在", true);
    return;
  }

  currentEditTaskId = taskId;
  els.editTitle.textContent = taskId;
  els.editDescription.value = task.description || "";
  editTagList = [...displayTags(task)];
  if (els.editTagInput) els.editTagInput.value = "";
  renderEditTags();
  fillCronForm(task.schedule || "0 8 * * *");
  updateEditPreview();
  els.editDialog.showModal();
}

async function saveEdit(event) {
  event.preventDefault();
  if (!currentEditTaskId) return;

  const schedule = buildScheduleFromForm();
  if (!schedule || schedule.split(/\s+/).length !== 5) {
    showToast("请填写完整的分、时、日、月、周", true);
    return;
  }

  try {
    els.btnEditSave.disabled = true;
    const res = await api(`/tasks/${encodeURIComponent(currentEditTaskId)}/schedule`, {
      method: "PATCH",
      body: JSON.stringify({
        description: els.editDescription.value.trim(),
        schedule,
        tags: editTagList,
      }),
    });
    els.editDialog.close();
    showToast(res.message || "已保存");
    await loadTasks();
  } catch (err) {
    showToast(err.message, true);
  } finally {
    els.btnEditSave.disabled = false;
  }
}

function bindActions(root) {
  root.addEventListener("click", (event) => {
    const btn = event.target.closest("[data-action]");
    if (!btn) return;
    handleAction(btn.dataset.action, btn.dataset.id);
  });
}

document.getElementById("btn-refresh-top").addEventListener("click", loadTasks);
document.getElementById("btn-log-refresh").addEventListener("click", refreshLog);
document.getElementById("btn-log-close").addEventListener("click", () => els.logDialog.close());
document.getElementById("btn-edit-close").addEventListener("click", () => els.editDialog.close());
document.getElementById("btn-edit-cancel").addEventListener("click", () => els.editDialog.close());
els.editForm.addEventListener("submit", saveEdit);
els.editForm.addEventListener("click", (event) => {
  if (event.target.closest("#btn-tag-add")) {
    event.preventDefault();
    addEditTag();
    return;
  }
  const removeBtn = event.target.closest(".tag-remove");
  if (removeBtn) {
    event.preventDefault();
    removeEditTag(Number(removeBtn.dataset.index));
  }
});
els.editForm.addEventListener("keydown", (event) => {
  if (event.target !== els.editTagInput || event.key !== "Enter") return;
  event.preventDefault();
  addEditTag();
});
CRON_FIELD_KEYS.forEach((key) => els[key].addEventListener("input", updateEditPreview));
els.editMonth.addEventListener("change", updateEditPreview);
els.editDow.addEventListener("change", updateEditPreview);
els.cronPresets.addEventListener("click", (event) => {
  const btn = event.target.closest("[data-preset]");
  if (!btn) return;
  fillCronForm(btn.dataset.preset);
  updateEditPreview();
});
els.btnEditSave = document.getElementById("btn-edit-save");
els.filter.addEventListener("input", applyFilter);

els.logDialog.addEventListener("click", (event) => {
  if (event.target === els.logDialog) els.logDialog.close();
});
els.editDialog.addEventListener("click", (event) => {
  if (event.target === els.editDialog) els.editDialog.close();
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    if (els.logDialog.open) els.logDialog.close();
    else if (els.editDialog.open) els.editDialog.close();
    return;
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
