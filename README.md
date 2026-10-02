# TopWebDav

极简 WebDAV + 网页文件管理。单账号、单二进制，适合挂在反向代理或 1Panel 后面自用。

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## 目录

- [功能](#功能)
- [快速开始](#快速开始)
- [部署方式](#部署方式)
  - [方式一：预编译二进制（推荐）](#方式一预编译二进制推荐)
  - [方式二：1Panel 部署](#方式二1panel-部署)
  - [方式三：systemd + Nginx 反代](#方式三systemd--nginx-反代)
  - [源码清单（在线编译时上传什么）](#源码清单在线编译时上传什么)
- [WebDAV 挂载](#webdav-挂载)
- [配置说明](#配置说明)
- [安全与已知取舍](#安全与已知取舍)
- [开发](#开发)
- [License](#license)

## 功能

- **WebDAV**（`/dav/`）：可用 Finder、资源管理器、Cyberduck、rclone 等挂载
- **网页文件管理**：列目录、上传、下载、新建文件（名+内容）、新建文件夹、文本编辑、重命名、删除；上传和下载都有进度条。大文件由浏览器下载管理器保存，网页进度条显示服务端已传输的字节数，反向代理缓存可能使其略早于浏览器实际落盘
- **单账号**：bcrypt 存密码，支持改密
- **界面**：中英双语、暗黑模式、SF 风格图标
- **安全**：WebDAV 与网页 API 一致地限制在数据根内（含符号链接检查），登录失败限速（同 IP 5 次失败后 429，指数退避至 15 分钟，正确密码不受冷却影响），单次上传默认 4 GiB 上限（`max_upload_mb`），配置文件 `0600`

## 快速开始

在已安装 Go 的机器上：

```bash
git clone https://github.com/cnliucheng/TopWebDav.git
cd TopWebDav
go build -o topwebdav .
./topwebdav -config ./config.json
```

首次启动会生成 `config.json` 与 `data/`。默认账号 **admin / admin**，登录后请立刻改密。

- 网页：`http://127.0.0.1:8080/`
- WebDAV：`http://127.0.0.1:8080/dav/`

## 部署方式

### 方式一：预编译二进制（推荐）

适合第一次使用 Go、或不想在服务器上编译的用户。前端已打进二进制，**服务器只需上传 1 个文件**。

**1. 本机交叉编译 Linux 版**

```bash
GOOS=linux GOARCH=amd64 go build -o topwebdav .
```

**2. 上传到服务器（只需这些）**

| 文件 | 是否必须 | 说明 |
|------|----------|------|
| `topwebdav` | 必须 | 上一步编译出的程序 |
| `config.json` | 可不传 | 首次运行自动生成 |

目录示例：

```text
/opt/topwebdav/
  topwebdav
  config.json   # 可选
  data/         # 自动生成，文件数据
```

**3. 启动**

```bash
chmod +x /opt/topwebdav/topwebdav
cd /opt/topwebdav
./topwebdav -listen 0.0.0.0:8080
```

反代后面、只给本机反代用时，可改为 `-listen 127.0.0.1:8080`。

---

### 方式二：1Panel 部署

1Panel 的「运行环境」中的 **Go** 基于容器，需提供**可构建的源码**或**可执行文件**。参考官方文档：[Go 运行环境](https://docs.fit2cloud.com/1panel/user_manual/websites/golang)、[创建网站](https://docs.fit2cloud.com/1panel/user_manual/websites/website-create)。

**1. 上传文件**

- 若使用预编译二进制：只上传 `topwebdav`（见 [源码清单](#源码清单在线编译时上传什么) 中的二进制说明）
- 若在线编译：上传 [源码清单](#源码清单在线编译时上传什么) 中列出的文件

目录示例：`/opt/topwebdav/`。

**2. 创建 Go 运行环境**

**网站 → 运行环境 → Go → 创建运行环境**：

| 项 | 建议值 |
|----|--------|
| Go 版本 | 列表中可用版本（源码构建需要 ≥ 1.26） |
| 运行目录 | `/opt/topwebdav` |
| 启动命令 | 见下方 |

二进制方式：

```bash
./topwebdav -listen 0.0.0.0:8080
```

源码方式：

```bash
go build -o topwebdav . && ./topwebdav -listen 0.0.0.0:8080
```

**务必使用 `0.0.0.0:8080`**，否则容器内 `127.0.0.1` 无法被 OpenResty 反代访问。创建后查看运行环境**日志**，出现 `topwebdav listening on ...` 即成功。

**3. 创建网站**

**网站 → 创建网站 → 运行环境**：

- 类型：Go，选择上一步的运行环境  
- 端口：`8080`（与启动命令一致）  
- 主域名：你的域名  
- 启用 HTTPS：申请或选择证书  
- 大文件上传：在网站配置中调大请求体/上传限制  

访问：网页 `https://域名/`，WebDAV `https://域名/dav/`。

**4. 数据备份**

备份运行目录下的 `config.json` 与 `data/` 即可。

---

### 方式三：systemd + Nginx 反代

适合不用面板、直接管理 Linux 的场景。

**1. 准备程序**

按 [方式一](#方式一预编译二进制推荐) 交叉编译，或在服务器上源码构建，放到 `/opt/topwebdav/topwebdav`。

**2. 安装 systemd 服务**

可直接使用仓库中的 [`deploy/topwebdav.service`](deploy/topwebdav.service)：

```bash
cp deploy/topwebdav.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now topwebdav
systemctl status topwebdav
```

**3. Nginx 反向代理**

参考 [`deploy/nginx.conf.example`](deploy/nginx.conf.example)，代理到 `http://127.0.0.1:8080`，并开启 HTTPS。注意配置 `client_max_body_size` 以支持较大上传。

防火墙/安全组只开放 80/443，不要对公网开放 8080。

---

### 源码清单（在线编译时上传什么）

在 1Panel Go 运行环境里执行 `go build` 时，**不要上传整个 Git 仓库**，只需下列文件：

```text
go.mod
go.sum
main.go
config.go
auth.go
path.go
api.go
dav.go
davfs.go
server.go
download.go
web/index.html
web/style.css
web/app.js
```

**不要上传**：`docs/`、`deploy/`、`.git/`、`data/`、本机的 `config.json`（服务器首次运行会生成）。

## WebDAV 挂载

```bash
rclone config
# type: webdav
# url:  https://你的域名/dav   （或 http://127.0.0.1:8080/dav）
# user / pass: 你的账号

# 或
cadaver https://你的域名/dav/
```

## 配置说明

命令行：

| 参数 | 说明 |
|------|------|
| `-config` | 配置文件路径，默认 `./config.json` |
| `-listen` | 监听地址，覆盖配置与环境变量 |

监听优先级：`-listen` > `TOPWEBDAV_LISTEN` > `config.json`（默认 `127.0.0.1:8080`）。

`config.json`：

```json
{
  "listen": "127.0.0.1:8080",
  "data_dir": "./data",
  "username": "admin",
  "password_hash": "$2a$10$...",
  "max_upload_mb": 4096
}
```

| 字段 | 说明 |
|------|------|
| `listen` | 监听地址 |
| `data_dir` | 文件根目录（0700） |
| `username` | 登录名 |
| `password_hash` | bcrypt 哈希 |
| `max_upload_mb` | 单次上传上限（MiB），适用于网页上传与 WebDAV 写入；缺省 4096，`0` 表示不限制 |

## 安全与已知取舍

- **路径限制**：网页 API 与 WebDAV 都只允许访问 `data_dir` 内的路径；输入中的 `..` 直接拒绝，符号链接先解析真实路径再做包含性检查。WebDAV 路径逃逸返回 404，网页 API 返回路径错误。
- **登录限速**：同一个来源 IP 连续 5 次密码错误后开始返回 429（携带 `Retry-After`），冷却时间 30s 起逐次加倍，上限 15 分钟；登录成功即清零。冷却期内错误密码与无凭据请求仍返回 429 并续期，但**正确密码始终能登录**（冷却期内每个 IP 每秒最多校验一次密码，防止攻击者借机消耗 CPU）——因此持续制造登录失败无法把真实用户长期锁在门外。按 `RemoteAddr` 计数，因此挂在反向代理后时对所有客户端生效（相当于全局限速），公网部署仍建议配合反代层的 fail2ban/限流。
- **上传上限**：单次上传（网页 multipart POST、WebDAV PUT/POST 请求体）默认上限 4 GiB，超出返回 413 / 写入失败；由 `config.json` 的 `max_upload_mb` 控制，`0` 为不限制。反代层（如 Nginx `client_max_body_size`）的限制仍然生效且通常应设得更小。
- **浏览器端凭据**：网页界面没有会话机制，登录后把 Basic Auth 凭据存在当前标签页的 `sessionStorage`，直到手动退出或标签页会话结束。请务必走 HTTPS。大文件下载使用有效期 10 分钟的随机下载凭据，改密后失效；退出网页不会立即撤销已签发的下载链接，下载 URL 应避免记录或分享。
- **改密码的影响**：改密即时生效；执行改密的浏览器标签会更新凭据，其他已登录标签和 WebDAV 客户端需要重新登录或更新密码。

## 开发

```bash
go test ./...
go run . -listen 127.0.0.1:8080
```

前端在 `web/`（`index.html` / `style.css` / `app.js`），由 Go `embed` 打入二进制。

## License

[MIT](LICENSE)
