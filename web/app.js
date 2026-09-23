"use strict";

const state = {
  cur: "/",
  auth: "",
  user: "",
};

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
    const err = new Error("未登录或登录已失效");
    err.status = 401;
    throw err;
  }
  const ct = res.headers.get("Content-Type") || "";
  if (!res.ok) {
    let msg = "请求失败 (" + res.status + ")";
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

async function ensureLogin() {
  while (!state.auth) {
    const user = await promptText("登录 — 用户名", state.user || "admin");
    if (user === null) {
      showError("需要登录才能使用");
      return false;
    }
    const pass = await promptPassword("登录 — 密码");
    if (pass === null) {
      showError("需要登录才能使用");
      return false;
    }
    state.user = user;
    state.auth = toBasic(user, pass);
    try {
      await api("/api/list?path=" + encodeURIComponent("/"));
      showError("");
      return true;
    } catch (e) {
      state.auth = "";
      if (e.status === 401) {
        showError("用户名或密码错误");
        continue;
      }
      showError(e.message);
      return false;
    }
  }
  return true;
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
  const items = await api("/api/list?path=" + encodeURIComponent(state.cur));
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
}

function renderRow(it) {
  const tr = document.createElement("tr");

  const tdName = document.createElement("td");
  if (it.is_dir) {
    const a = document.createElement("a");
    a.className = "dir";
    a.textContent = it.name;
    a.addEventListener("click", () => {
      state.cur = it.path;
      load().catch((e) => showError(e.message));
    });
    tdName.appendChild(a);
  } else {
    tdName.textContent = it.name;
  }
  tr.appendChild(tdName);

  const tdSize = document.createElement("td");
  tdSize.textContent = it.is_dir ? "-" : fmtSize(it.size);
  tr.appendChild(tdSize);

  const tdTime = document.createElement("td");
  tdTime.textContent = fmtTime(it.mod_time);
  tr.appendChild(tdTime);

  const tdAct = document.createElement("td");
  tdAct.className = "actions";
  const actions = [];
  if (!it.is_dir) {
    actions.push(["下载", () => download(it.path, it.name)]);
    actions.push(["编辑", () => openEdit(it.path)]);
  }
  actions.push(["重命名", () => doRename(it)]);
  actions.push(["删除", () => doDelete(it)]);
  for (const [label, fn] of actions) {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = label;
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
  const res = await api("/api/download?path=" + encodeURIComponent(path));
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
    const data = await api("/api/read?path=" + encodeURIComponent(path));
    $("editTitle").textContent = "编辑 " + path;
    $("editBody").value = (data && data.content) || "";
    const dlg = $("dlgEdit");
    await new Promise((resolve) => {
      const onSave = async () => {
        try {
          await api("/api/write", {
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
  const name = await promptText("新建文件夹 — 名称");
  if (name === null || name === "") return;
  await api("/api/mkdir", {
    method: "POST",
    body: { path: joinPath(state.cur, name) },
  });
  await load();
}

async function doCreate() {
  showError("");
  const name = await promptText("新建文件 — 名称");
  if (name === null || name === "") return;
  const path = joinPath(state.cur, name);
  await api("/api/create", {
    method: "POST",
    body: { path: path, content: "" },
  });
  await load();
  await openEdit(path);
}

async function doRename(it) {
  showError("");
  const name = await promptText("重命名", it.name);
  if (name === null || name === "" || name === it.name) return;
  await api("/api/rename", {
    method: "POST",
    body: { from: it.path, to: joinPath(state.cur, name) },
  });
  await load();
}

async function doDelete(it) {
  showError("");
  if (!confirm("确定删除 " + it.name + "？")) return;
  await api("/api/delete?path=" + encodeURIComponent(it.path), {
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
    await api("/api/upload?path=" + encodeURIComponent(state.cur), {
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
        showError("请填写旧密码和新密码");
        return;
      }
      try {
        await api("/api/password", {
          method: "POST",
          body: { old_password: oldPw, new_password: newPw },
        });
        state.auth = toBasic(state.user, newPw);
        cleanup();
        dlg.close();
        showError("密码已修改");
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
  state.cur = "/";
  $("rows").textContent = "";
  $("pathLabel").textContent = state.cur;
  if (await ensureLogin()) {
    try {
      await load();
    } catch (e) {
      showError(e.message);
    }
  }
}

function wire() {
  $("btnUp").addEventListener("click", () => {
    state.cur = parentPath(state.cur);
    load().catch((e) => showError(e.message));
  });
  $("btnRefresh").addEventListener("click", () => {
    load().catch((e) => showError(e.message));
  });
  $("btnUpload").addEventListener("click", () => {
    doUpload().catch((e) => showError(e.message));
  });
  $("btnMkdir").addEventListener("click", () => {
    doMkdir().catch((e) => showError(e.message));
  });
  $("btnCreate").addEventListener("click", () => {
    doCreate().catch((e) => showError(e.message));
  });
  $("btnPassword").addEventListener("click", () => {
    doChangePassword().catch((e) => showError(e.message));
  });
  $("btnLogout").addEventListener("click", () => {
    doLogout().catch((e) => showError(e.message));
  });
}

async function boot() {
  wire();
  try {
    await load();
    return;
  } catch (e) {
    if (e.status !== 401 && state.auth) {
      showError(e.message);
      return;
    }
  }
  if (await ensureLogin()) {
    try {
      await load();
    } catch (e) {
      showError(e.message);
    }
  }
}

boot();
