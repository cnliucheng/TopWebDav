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
- [开发](#开发)
- [License](#license)

## 功能

- **WebDAV**（`/dav/`）：可用 Finder、资源管理器、Cyberduck、rclone 等挂载
- **网页文件管理**：列目录、上传、下载、新建文件（名+内容）、新建文件夹、文本编辑、重命名、删除
- **单账号**：bcrypt 存密码，支持改密
- **界面**：中英双语、暗黑模式、SF 风格图标
- **安全**：路径限制在数据根内（含符号链接检查），配置文件 `0600`

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
| Go 版本 | 列表中可用版本（源码构建建议 ≥ 1.22） |
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
server.go
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
  "password_hash": "$2a$10$..."
}
```

| 字段 | 说明 |
|------|------|
| `listen` | 监听地址 |
| `data_dir` | 文件根目录（0700） |
| `username` | 登录名 |
| `password_hash` | bcrypt 哈希 |

## 开发

```bash
go test ./...
go run . -listen 127.0.0.1:8080
```

前端在 `web/`（`index.html` / `style.css` / `app.js`），由 Go `embed` 打入二进制。

## License

[MIT](LICENSE)
