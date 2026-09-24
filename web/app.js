"use strict";

const state = {
  cur: "/",
  auth: sessionStorage.getItem("twd.auth") || "",
  user: sessionStorage.getItem("twd.user") || "",
  lang: localStorage.getItem("twd.lang") || ((navigator.language || "zh").toLowerCase().startsWith("zh") ? "zh" : "en"),
  theme: localStorage.getItem("twd.theme") || "auto",
};

const I18N = {
  zh: {
    subtitle: "文件管理",
    btnPassword: "改密码",
    btnLogout: "退出",
    btnUp: "上级目录",
    btnRefresh: "刷新",
    btnMkdir: "新建文件夹",
    btnCreate: "新建文件",
    btnUpload: "上传",
    colName: "名称",
    colSize: "大小",
    colTime: "修改时间",
    colActions: "操作",
    emptyTitle: "这个目录是空的",
    emptySub: "上传文件，或新建一个文件夹开始整理",
    cancel: "取消",
    save: "保存",
    create: "创建",
    ok: "确定",
    createTitle: "新建文件",
    fileName: "文件名",
    content: "内容",
    value: "内容",
    createNamePh: "例如 notes.txt",
    createBodyPh: "可留空",
    changePassword: "修改密码",
    oldPassword: "旧密码",
    newPassword: "新密码",
    theme: "主题",
    username: "用户名",
    password: "密码",
    signIn: "登录",
    loginSubtitle: "登录以管理文件",
    loginUserPh: "admin",
    loginPassPh: "••••••••",
    download: "下载",
    edit: "编辑",
    rename: "重命名",
    delete: "删除",
    editTitle: "编辑 {path}",
    mkdirTitle: "新建文件夹 — 名称",
    renameTitle: "重命名",
    promptInput: "输入",
    loginUser: "登录 — 用户名",
    loginPass: "登录 — 密码",
    needLogin: "需要登录才能使用",
    badLogin: "用户名或密码错误",
    unauthorized: "未登录或登录已失效",
    requestFail: "请求失败 ({code})",
    fillFileName: "请填写文件名",
    fillPasswords: "请填写旧密码和新密码",
    passwordChanged: "密码已修改",
    confirmDelete: "确定删除 {name}？",
  },
  en: {
    subtitle: "Files",
    btnPassword: "Password",
    btnLogout: "Sign out",
    btnUp: "Parent folder",
    btnRefresh: "Refresh",
    btnMkdir: "New folder",
    btnCreate: "New file",
    btnUpload: "Upload",
    colName: "Name",
    colSize: "Size",
    colTime: "Modified",
    colActions: "Actions",
    emptyTitle: "This folder is empty",
    emptySub: "Upload files or create a folder to get started",
    cancel: "Cancel",
    save: "Save",
    create: "Create",
    ok: "OK",
    createTitle: "New file",
    fileName: "File name",
    content: "Content",
    value: "Value",
    createNamePh: "e.g. notes.txt",
    createBodyPh: "Optional",
    changePassword: "Change password",
    oldPassword: "Current password",
    newPassword: "New password",
    theme: "Theme",
    username: "Username",
    password: "Password",
    signIn: "Sign in",
    loginSubtitle: "Sign in to manage files",
    loginUserPh: "admin",
    loginPassPh: "••••••••",
    download: "Download",
    edit: "Edit",
    rename: "Rename",
    delete: "Delete",
    editTitle: "Edit {path}",
    mkdirTitle: "New folder — name",
    renameTitle: "Rename",
    promptInput: "Input",
    loginUser: "Sign in — username",
    loginPass: "Sign in — password",
    needLogin: "Sign in required",
    badLogin: "Incorrect username or password",
    unauthorized: "Not signed in or session expired",
    requestFail: "Request failed ({code})",
    fillFileName: "File name is required",
    fillPasswords: "Enter current and new password",
    passwordChanged: "Password updated",
    confirmDelete: "Delete {name}?",
  },
};

function t(key, vars) {
  const lang = state.lang === "en" ? "en" : "zh";
  let s = (I18N[lang] && I18N[lang][key]) || I18N.zh[key] || key;
  if (vars) {
    for (const k of Object.keys(vars)) {
      s = s.replace("{" + k + "}", vars[k]);
    }
  }
  return s;
}

function applyI18n() {
  document.documentElement.lang = state.lang === "en" ? "en" : "zh-CN";
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    el.textContent = t(el.getAttribute("data-i18n"));
  });
  document.querySelectorAll("[data-i18n-title]").forEach((el) => {
    const v = t(el.getAttribute("data-i18n-title"));
    el.title = v;
    el.setAttribute("aria-label", v);
  });
  document.querySelectorAll("[data-i18n-placeholder]").forEach((el) => {
    el.placeholder = t(el.getAttribute("data-i18n-placeholder"));
  });
  const langLabel = $("langLabel");
  if (langLabel) langLabel.textContent = state.lang === "en" ? "中文" : "EN";
}

function resolveTheme() {
  if (state.theme === "dark" || state.theme === "light") return state.theme;
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function applyTheme() {
  const mode = resolveTheme();
  document.documentElement.setAttribute("data-theme", mode);
  localStorage.setItem("twd.theme", state.theme);
  const use = document.querySelector("#themeIcon use");
  if (use) use.setAttribute("href", mode === "dark" ? "#i-sun" : "#i-moon");
}

function $(id) {
  return document.getElementById(id);
}

function showError(msg) {
  const el = $("error");
  if (!msg) {
    el.hidden = true;
    el.textContent = "";
    return;
  }
  el.hidden = false;
  el.textContent = msg;
}

function joinPath(dir, name) {
  if (!name) return dir;
  if (dir === "/") return "/" + name;
  return dir.replace(/\/+$/, "") + "/" + name;
}

function parentPath(p) {
  if (!p || p === "/") return "/";
  const cleaned = p.replace(/\/+$/, "");
  const i = cleaned.lastIndexOf("/");
  if (i <= 0) return "/";
  return cleaned.slice(0, i);
}

function baseName(p) {
  const cleaned = String(p || "").replace(/\/+$/, "");
  const i = cleaned.lastIndexOf("/");
  return i < 0 ? cleaned : cleaned.slice(i + 1);
}

function toBasic(user, pass) {
  const bytes = new TextEncoder().encode(user + ":" + pass);
  let bin = "";
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
  return btoa(bin);
}

function promptText(title, initial) {
  return new Promise((resolve) => {
    const dlg = $("dlgPrompt");
    const v = $("promptValue");
    const s = $("promptSecret");
    $("promptTitle").textContent = title;
    v.hidden = false;
    s.hidden = true;
    v.value = initial == null ? "" : initial;
    s.value = "";
    const finish = (val) => {
      $("promptOk").removeEventListener("click", onOk);
      $("promptCancel").removeEventListener("click", onCancel);
      v.removeEventListener("keydown", onKey);
      dlg.close();
      resolve(val);
    };
    const onOk = () => finish(v.value);
    const onCancel = () => finish(null);
    const onKey = (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        onOk();
      }
    };
    $("promptOk").addEventListener("click", onOk);
    $("promptCancel").addEventListener("click", onCancel);
    v.addEventListener("keydown", onKey);
    dlg.showModal();
    v.focus();
    v.select();
  });
}

function promptPassword(title, initial) {
  return new Promise((resolve) => {
    const dlg = $("dlgPrompt");
    const v = $("promptValue");
    const s = $("promptSecret");
    $("promptTitle").textContent = title;
    v.hidden = true;
    s.hidden = false;
    v.value = "";
    s.value = initial == null ? "" : initial;
    const finish = (val) => {
      $("promptOk").removeEventListener("click", onOk);
      $("promptCancel").removeEventListener("click", onCancel);
      s.removeEventListener("keydown", onKey);
      dlg.close();
      resolve(val);
    };
    const onOk = () => finish(s.value);
    const onCancel = () => finish(null);
    const onKey = (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        onOk();
      }
    };
    $("promptOk").addEventListener("click", onOk);
    $("promptCancel").addEventListener("click", onCancel);
    s.addEventListener("keydown", onKey);
    dlg.showModal();
    s.focus();
  });
}

async function api(pathname, opts) {
  // Relative "api/..." (no leading slash) so reverse-proxy subpaths work.
  const options = Object.assign({}, opts || {});
  const headers = Object.assign({}, options.headers || {});
  if (state.auth) headers.Authorization = "Basic " + state.auth;
  if (options.body && typeof options.body === "object" && !(options.body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(options.body);
  }
  options.headers = headers;
  const res = await fetch(pathname, options);
  if (res.status === 401) {
    state.auth = "";
    sessionStorage.removeItem("twd.auth");
    sessionStorage.removeItem("twd.user");
    showLoginView();
    const err = new Error(t("unauthorized"));
    err.status = 401;
    throw err;
  }
  const ct = res.headers.get("Content-Type") || "";
  if (!res.ok) {
    let msg = t("requestFail", { code: res.status });
    if (ct.includes("application/json")) {
      try {
        const j = await res.json();
        if (j && j.error) msg = j.error;
      } catch (_) {}
    }
    const err = new Error(msg);
    err.status = res.status;
    throw err;
  }
  if (ct.includes("application/json")) return res.json();
  return res;
}

function showLoginView() {
  const login = $("loginView");
  const app = $("appView");
  if (login) login.hidden = false;
  if (app) app.hidden = true;
  const err = $("loginError");
  if (!err) return;
}

function showLoginError(msg) {
  const el = $("loginError");
  if (!el) return;
  if (!msg) {
    el.hidden = true;
    el.textContent = "";
    return;
  }
  el.hidden = false;
  el.textContent = msg;
}

function showAppView() {
  const login = $("loginView");
  const app = $("appView");
  if (login) login.hidden = true;
  if (app) app.hidden = false;
}

function saveAuth(user, passB64) {
  state.user = user;
  state.auth = passB64;
  sessionStorage.setItem("twd.user", user);
  sessionStorage.setItem("twd.auth", passB64);
}

async function tryLogin(user, pass) {
  saveAuth(user, toBasic(user, pass));
  try {
    await api("api/list?path=" + encodeURIComponent("/"));
    showLoginError("");
    return true;
  } catch (e) {
    state.auth = "";
    sessionStorage.removeItem("twd.auth");
    sessionStorage.removeItem("twd.user");
    if (e.status === 401) {
      showLoginError(t("badLogin"));
    } else {
      showLoginError(e.message || t("requestFail", { code: e.status || 0 }));
    }
    return false;
  }
}

function wireLogin() {
  const form = $("loginForm");
  if (!form) return;
  let busy = false;
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    if (busy) return;
    const userEl = $("loginUser");
    const passEl = $("loginPass");
    const user = ((userEl && userEl.value) || "").trim();
    const pass = (passEl && passEl.value) || "";
    if (!user || !pass) {
      showLoginError(t("badLogin"));
      return;
    }
    busy = true;
    showLoginError("");
    tryLogin(user, pass)
      .then((ok) => {
        if (ok) {
          showAppView();
          return load().catch((err) => showError(err.message));
        }
      })
      .catch((err) => {
        showLoginError((err && err.message) || "error");
      })
      .then(() => {
        busy = false;
      });
  });
}

function fmtSize(n) {
  if (n == null || isNaN(n)) return "-";
  if (n < 1024) return n + " B";
  const units = ["KB", "MB", "GB", "TB"];
  let x = n / 1024;
  let u = 0;
  while (x >= 1024 && u < units.length - 1) {
    x /= 1024;
    u++;
  }
  return x.toFixed(1) + " " + units[u];
}

function fmtTime(t) {
  if (!t) return "-";
  const d = new Date(t);
  if (isNaN(d.getTime())) return "-";
  const p = (n) => String(n).padStart(2, "0");
  return (
    d.getFullYear() +
    "-" +
    p(d.getMonth() + 1) +
    "-" +
    p(d.getDate()) +
    " " +
    p(d.getHours()) +
    ":" +
    p(d.getMinutes())
  );
}

async function load() {
  $("pathLabel").textContent = state.cur;
  const items = await api("api/list?path=" + encodeURIComponent(state.cur));
  const list = Array.isArray(items) ? items.slice() : [];
  list.sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
  const tbody = $("rows");
  tbody.textContent = "";
  for (const it of list) {
    tbody.appendChild(renderRow(it));
  }
  const empty = $("emptyState");
  const table = $("listing");
  if (empty) empty.hidden = list.length > 0;
  if (table) table.hidden = list.length === 0;
}

function renderRow(it) {
  const tr = document.createElement("tr");

  const tdName = document.createElement("td");
  const wrap = document.createElement("div");
  wrap.className = "name-cell";
  const mark = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  mark.setAttribute("class", "row-icon " + (it.is_dir ? "is-dir" : "is-file"));
  mark.setAttribute("aria-hidden", "true");
  const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
  use.setAttribute("href", it.is_dir ? "#i-folder" : "#i-file");
  mark.appendChild(use);
  wrap.appendChild(mark);
  if (it.is_dir) {
    const a = document.createElement("a");
    a.className = "dir";
    a.textContent = it.name;
    a.addEventListener("click", () => {
      state.cur = it.path;
      load().catch((e) => showError(e.message));
    });
    wrap.appendChild(a);
  } else {
    const span = document.createElement("span");
    span.className = "file-name";
    span.textContent = it.name;
    wrap.appendChild(span);
  }
  tdName.appendChild(wrap);
  tr.appendChild(tdName);

  const tdSize = document.createElement("td");
  tdSize.className = "size";
  tdSize.textContent = it.is_dir ? "—" : fmtSize(it.size);
  tr.appendChild(tdSize);

  const tdTime = document.createElement("td");
  tdTime.className = "time";
  tdTime.textContent = fmtTime(it.mod_time);
  tr.appendChild(tdTime);

  const tdAct = document.createElement("td");
  tdAct.className = "actions";
  const actions = [];
  if (!it.is_dir) {
    actions.push(["i-download", "download", () => download(it.path, it.name), ""]);
    actions.push(["i-pencil", "edit", () => openEdit(it.path), ""]);
  }
  actions.push(["i-rename", "rename", () => doRename(it), ""]);
  actions.push(["i-trash", "delete", () => doDelete(it), "danger"]);
  for (const [icon, labelKey, fn, kind] of actions) {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "btn sm" + (kind ? " " + kind : "");
    b.title = t(labelKey);
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("class", "icon");
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", "#" + icon);
    svg.appendChild(use);
    b.appendChild(svg);
    const label = document.createElement("span");
    label.textContent = t(labelKey);
    b.appendChild(label);
    b.addEventListener("click", () => {
      Promise.resolve(fn()).catch((e) => showError(e.message));
    });
    tdAct.appendChild(b);
  }
  tr.appendChild(tdAct);
  return tr;
}

async function download(path, name) {
  showError("");
  const res = await api("api/download?path=" + encodeURIComponent(path));
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name || baseName(path);
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

function openEdit(path) {
  showError("");
  return (async () => {
    const data = await api("api/read?path=" + encodeURIComponent(path));
    $("editTitle").textContent = t("editTitle", { path });
    $("editBody").value = (data && data.content) || "";
    const dlg = $("dlgEdit");
    await new Promise((resolve) => {
      const onSave = async () => {
        try {
          await api("api/write", {
            method: "POST",
            body: { path: path, content: $("editBody").value },
          });
          cleanup();
          dlg.close();
          showError("");
          resolve();
        } catch (e) {
          showError(e.message);
        }
      };
      const onCancel = () => {
        cleanup();
        dlg.close();
        resolve();
      };
      function cleanup() {
        $("editSave").removeEventListener("click", onSave);
        $("editCancel").removeEventListener("click", onCancel);
      }
      $("editSave").addEventListener("click", onSave);
      $("editCancel").addEventListener("click", onCancel);
      dlg.showModal();
    });
    await load();
  })();
}

async function doMkdir() {
  showError("");
  const name = await promptText(t("mkdirTitle"));
  if (name === null || name === "") return;
  await api("api/mkdir", {
    method: "POST",
    body: { path: joinPath(state.cur, name) },
  });
  await load();
}

async function doCreate() {
  showError("");
  const dlg = $("dlgCreate");
  const nameEl = $("createName");
  const bodyEl = $("createBody");
  nameEl.value = "";
  bodyEl.value = "";
  await new Promise((resolve) => {
    const onSave = async () => {
      const name = nameEl.value.trim();
      if (!name) {
        showError(t("fillFileName"));
        nameEl.focus();
        return;
      }
      const path = joinPath(state.cur, name);
      try {
        await api("api/create", {
          method: "POST",
          body: { path: path, content: bodyEl.value },
        });
        cleanup();
        dlg.close();
        showError("");
        resolve();
      } catch (e) {
        showError(e.message);
      }
    };
    const onCancel = () => {
      cleanup();
      dlg.close();
      resolve();
    };
    const onKey = (e) => {
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        onSave();
      }
    };
    function cleanup() {
      $("createSave").removeEventListener("click", onSave);
      $("createCancel").removeEventListener("click", onCancel);
      dlg.removeEventListener("keydown", onKey);
    }
    $("createSave").addEventListener("click", onSave);
    $("createCancel").addEventListener("click", onCancel);
    dlg.addEventListener("keydown", onKey);
    dlg.showModal();
    nameEl.focus();
  });
  await load();
}

async function doRename(it) {
  showError("");
  const name = await promptText(t("renameTitle"), it.name);
  if (name === null || name === "" || name === it.name) return;
  await api("api/rename", {
    method: "POST",
    body: { from: it.path, to: joinPath(state.cur, name) },
  });
  await load();
}

async function doDelete(it) {
  showError("");
  if (!confirm(t("confirmDelete", { name: it.name }))) return;
  await api("api/delete?path=" + encodeURIComponent(it.path), {
    method: "DELETE",
  });
  await load();
}

async function doUpload() {
  showError("");
  const input = $("fileInput");
  input.value = "";
  input.click();
  await new Promise((resolve) => {
    input.onchange = () => resolve();
  });
  const files = input.files;
  if (!files || !files.length) return;
  for (const file of files) {
    const fd = new FormData();
    fd.append("file", file, file.name);
    await api("api/upload?path=" + encodeURIComponent(state.cur), {
      method: "POST",
      body: fd,
    });
  }
  await load();
}

async function doChangePassword() {
  showError("");
  const dlg = $("dlgPassword");
  $("oldPw").value = "";
  $("newPw").value = "";
  await new Promise((resolve) => {
    const onSave = async () => {
      const oldPw = $("oldPw").value;
      const newPw = $("newPw").value;
      if (!oldPw || !newPw) {
        showError(t("fillPasswords"));
        return;
      }
      try {
        await api("api/password", {
          method: "POST",
          body: { old_password: oldPw, new_password: newPw },
        });
        state.auth = toBasic(state.user, newPw);
        cleanup();
        dlg.close();
        showError(t("passwordChanged"));
        resolve();
      } catch (e) {
        showError(e.message);
      }
    };
    const onCancel = () => {
      cleanup();
      dlg.close();
      resolve();
    };
    function cleanup() {
      $("pwSave").removeEventListener("click", onSave);
      $("pwCancel").removeEventListener("click", onCancel);
    }
    $("pwSave").addEventListener("click", onSave);
    $("pwCancel").addEventListener("click", onCancel);
    dlg.showModal();
    $("oldPw").focus();
  });
}

async function doLogout() {
  showError("");
  state.auth = "";
  state.user = "";
  state.cur = "/";
  sessionStorage.removeItem("twd.auth");
  sessionStorage.removeItem("twd.user");
  const rows = $("rows");
  if (rows) rows.textContent = "";
  const pathLabel = $("pathLabel");
  if (pathLabel) pathLabel.textContent = state.cur;
  showLoginView();
  showLoginError("");
  const u = $("loginUser");
  const p = $("loginPass");
  if (u) u.value = "";
  if (p) p.value = "";
  if (u) u.focus();
}

function wire() {
  const bind = (id, fn) => {
    const el = $(id);
    if (el) el.addEventListener("click", fn);
  };
  bind("btnUp", () => {
    state.cur = parentPath(state.cur);
    load().catch((e) => showError(e.message));
  });
  bind("btnRefresh", () => {
    load().catch((e) => showError(e.message));
  });
  bind("btnUpload", () => {
    doUpload().catch((e) => showError(e.message));
  });
  bind("btnMkdir", () => {
    doMkdir().catch((e) => showError(e.message));
  });
  bind("btnCreate", () => {
    doCreate().catch((e) => showError(e.message));
  });
  bind("btnPassword", () => {
    doChangePassword().catch((e) => showError(e.message));
  });
  bind("btnLogout", () => {
    doLogout().catch((e) => showError(e.message));
  });
  bind("btnLang", () => {
    state.lang = state.lang === "en" ? "zh" : "en";
    localStorage.setItem("twd.lang", state.lang);
    applyI18n();
  });
  bind("btnTheme", () => {
    const mode = resolveTheme();
    state.theme = mode === "dark" ? "light" : "dark";
    applyTheme();
  });
  if (window.matchMedia) {
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
      if (state.theme === "auto") applyTheme();
    });
  }
}

async function boot() {
  applyTheme();
  applyI18n();
  wire();
  wireLogin();
  if (!state.auth) {
    showLoginView();
    const u = $("loginUser");
    if (u) {
      u.value = "admin";
      u.focus();
    }
    return;
  }
  try {
    await load();
    showAppView();
    return;
  } catch (e) {
    if (e.status === 401) {
      showLoginView();
      return;
    }
    showError(e.message);
    showAppView();
  }
}

boot();
