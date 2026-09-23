# TopWebDav 设计规格

日期：2026-09-23
状态：待用户审阅

## 目标

为 Linux 服务器提供一个**极简、易部署**的文件服务：

1. 真正的 **WebDAV 协议**（RFC 4918 常用子集），可用 Finder / 资源管理器 / Cyberduck / rclone 等客户端挂载
2. **网页文件管理**界面：浏览器内完成日常文件操作
3. **单账号**认证，支持修改密码
4. 单进程 Go 二进制，部署到已有反向代理后面即可

明确不做：多用户/配额、文件预览/缩略图、分享链接、回收站、全文搜索、TLS 终止、豪华 UI。

## 范围与约束

| 项 | 决定 |
|----|------|
| 服务形态 | WebDAV 协议 + 网页界面，共用文件树与账号 |
| 部署 | 裸机 Linux，监听本地端口，已有反向代理对外 |
| 用户模型 | 单账号，仅支持改密码 |
| 语言/运行时 | Go，单二进制，静态资源 `embed` |
| UI | 极简列表 + 工具栏，无装饰性设计 |
| WebDAV 实现 | `golang.org/x/net/webdav` |
| 监听 | 配置文件指定，默认 `127.0.0.1:8080`，可改 |

## 架构

单进程 HTTP 服务，两类入口：

1. **`/dav/*`** — WebDAV，映射到数据根目录；外部客户端用 Basic Auth
2. **`/api/*` 与 `/`** — REST API + 内嵌前端（`web/index.html`、`web/style.css`、`web/app.js`）

文件统一落在 `data_dir`（默认 `./data`）。账号与监听配置在同目录 `config.json`。

```text
浏览器 ──┐
         ├── 反向代理 ──► topwebdav (127.0.0.1:8080)
WebDAV ──┘                    │
                              ├── /dav/*  → x/net/webdav → data/
                              ├── /api/*  → REST        → data/
                              └── /*      → embed 静态页
                              读写 config.json（账号哈希）
```

两条入口共用同一 Basic Auth 与同一文件根。

## 组件

| 文件 | 职责 |
|------|------|
| `main.go` | 读配置、组装路由、启动监听 |
| `config.go` | `config.json` 读写与首次生成 |
| `auth.go` | Basic Auth、bcrypt 校验、改密码 |
| `dav.go` | 挂载 `x/net/webdav` 到 `/dav/` |
| `api.go` | 网页 REST 处理器 |
| `web/` | `index.html`、`style.css`、`app.js`，经 `embed` 打包 |

### 配置 `config.json`

```json
{
  "listen": "127.0.0.1:8080",
  "data_dir": "./data",
  "username": "admin",
  "password_hash": "$2a$10$..."
}
```

- 首次启动若不存在：创建 `config.json`（默认 `admin` / `admin`）与 `data_dir`，启动日志提示尽快改密
- 启动参数：`-config`（配置路径，默认 `./config.json`）、`-listen`（覆盖监听地址）
- 监听优先级：命令行 `-listen` > 环境变量 `TOPWEBDAV_LISTEN` > `config.json`
- 端口被占用：启动打印明确错误后退出，由使用者改端口并同步反代 upstream

### 密码

- 存储 bcrypt 哈希，不明文
- 仅支持「当前账号改密码」，无注册、无多用户管理界面

## 网页功能

| 功能 | 说明 |
|------|------|
| 列目录 | 名称、大小、是否目录、修改时间 |
| 上传 | multipart，可多选 |
| 下载 | 单文件下载 |
| 新建文件 | 输入文件名，创建空文本文件 |
| 新建文件夹 | 输入目录名 |
| 编辑 | 仅文本类文件；`<textarea>` 编辑后保存 |
| 重命名/移动 | 目标路径可改 |
| 删除 | 文件或目录 |
| 改密码 | 改当前账号密码 |

编辑约束：

- 仅文本（扩展名白名单或内容嗅探为文本）
- 单文件编辑上限 1MB，超出提示改用下载
- 不做语法高亮、多标签、协同编辑

UI 结构：登录（Basic Auth）→ 路径/面包屑 → 工具栏（上传、新建文件、新建文件夹、刷新）→ 文件列表（行操作：下载、编辑、重命名、删除）→ 改密码入口。

## REST API

均需 Basic Auth。`path` 为相对数据根的路径，以 `/` 开头。

| 方法 | 路径 | 作用 | 成功 | 主要错误 |
|------|------|------|------|----------|
| GET | `/api/list?path=/` | 列目录 | 200 JSON 数组 | 400/404 |
| POST | `/api/mkdir` | 新建目录 | 200 | 400/409 |
| POST | `/api/create` | 新建空文件（或带初始内容） | 200 | 400/409 |
| POST | `/api/upload?path=/` | 上传（multipart） | 200 | 400/409/413 |
| GET | `/api/download?path=` | 下载 | 200 流 | 400/404 |
| GET | `/api/read?path=` | 读文本（编辑用） | 200 JSON `{content}` | 400/404/413/415 |
| POST | `/api/write` | 保存文本 | 200 | 400/404/413/415 |
| DELETE | `/api/delete?path=` | 删除 | 200 | 400/403/404 |
| POST | `/api/rename` | 重命名/移动 | 200 | 400/404/409 |
| POST | `/api/password` | 改密码 | 200 | 401 |

请求/响应体为 JSON（上传除外）。错误响应统一：`{"error": "人类可读原因"}`。

主要请求体字段：

- `mkdir` / `create`：`{"path": "/dir/name"}`
- `create` 可选：`{"path": "/a.txt", "content": ""}`
- `write`：`{"path": "/a.txt", "content": "..."}`
- `rename`：`{"from": "/old", "to": "/new"}`
- `password`：`{"old_password": "...", "new_password": "..."}`

### 列表项字段

`name`、`path`、`is_dir`、`size`、`mod_time`（RFC3339）。

## WebDAV

- 挂载前缀：`/dav/` → `data_dir`
- 方法：GET、HEAD、PUT、DELETE、MKCOL、COPY、MOVE、PROPFIND、PROPPATCH、OPTIONS、LOCK、UNLOCK
- 锁：使用 `webdav.NewMemLS()` 内存锁系统（进程重启后锁丢失，单账号场景可接受）
- 认证：与网页相同的 Basic Auth 单账号

## 路径安全

- 所有 `path` 在服务端净化：拒绝空字节、拒绝 `..` 逃逸、清理多余 `/`
- 解析后的绝对路径必须仍在 `data_dir` 之下，否则 400
- 文件名禁止路径分隔符（重命名/新建时目标名仅允许单段合法文件名）

## 错误处理

| 场景 | 行为 |
|------|------|
| 认证失败 | 401 + `WWW-Authenticate: Basic realm="topwebdav"` |
| 路径非法/逃逸 | 400 |
| 不存在 | 404 |
| 目标已存在 | 409 |
| 删除非空目录 | 409，JSON 说明 `directory not empty` |
| 非文本编辑 / 超过 1MB | 415 / 413 |
| 端口占用、数据目录不可写 | 启动日志明确报错后退出 |

网页将 `error` 字段直接展示（`alert` 或顶部条即可）。

## 测试

1. **单元测试**：路径净化、认证、bcrypt 改密、文件 CRUD 语义
2. **`httptest`**：REST 全表（成功 + 主要错误码）
3. **WebDAV 冒烟**：`rclone` 或 `cadaver` 手工挂载（上传、下载、MKCOL、DELETE）
4. **网页黄金路径**：浏览器登录 → 上传 → 新建文件/文件夹 → 编辑保存 → 下载 → 重命名 → 删除 → 改密码

不引入 E2E 测试框架。

## 部署

```text
/opt/topwebdav/topwebdav      # 二进制
/opt/topwebdav/config.json    # 配置（含密码哈希）
/opt/topwebdav/data/          # 文件根
```

systemd 示例：

```ini
[Unit]
Description=TopWebDav
After=network.target

[Service]
WorkingDirectory=/opt/topwebdav
ExecStart=/opt/topwebdav/topwebdav -config /opt/topwebdav/config.json
Restart=on-failure
User=www-data

[Install]
WantedBy=multi-user.target
```

Nginx 反代提示：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    client_max_body_size 2g;   # 按需调整上传上限
}
```

- TLS 由已有反代负责，应用不做 TLS
- 仓库内提供 `deploy/topwebdav.service` 与 `deploy/nginx.conf.example`
- 端口冲突时改 `config.json` 的 `listen` 并更新反代

## 前端文件布局（工作目录可预览）

```text
web/index.html
web/style.css
web/app.js
```

开发时可直接用浏览器打开 `web/index.html` 查看结构；生产由 Go `embed` 提供同一套文件。联调时以真实 API 为准（同源或本地起服务）。

## 实现后的验收清单

- [ ] WebDAV 客户端可挂载并完成上传/下载/建删目录
- [ ] 网页完成列目录/上传/下载/新建文件/新建文件夹/编辑/重命名/删除/改密码
- [ ] 路径无法逃逸 `data_dir`
- [ ] 错误密码 401，改密后旧密码失效
- [ ] 端口被占用时启动报错可读
- [ ] 仓库含 systemd 与 Nginx 示例
