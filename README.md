# TopWebDav

极简 WebDAV + 网页文件管理。单账号、单二进制，适合挂在反向代理后面自用。

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## 功能

- **WebDAV**（`/dav/`）：可用 Finder、资源管理器、Cyberduck、rclone 等挂载
- **网页文件管理**：列目录、上传、下载、新建文件（名+内容）、新建文件夹、文本编辑、重命名、删除
- **单账号**：bcrypt 存密码，支持改密
- **界面**：中英双语、暗黑模式、SF 风格图标
- **安全**：路径限制在数据根内（含符号链接检查），配置文件 `0600`

## 快速开始

```bash
go build -o topwebdav .
./topwebdav -config ./config.json
```

首次启动会生成 `config.json` 与 `data/`。默认账号 **admin / admin**，登录后请立刻改密。

网页：`http://127.0.0.1:8080/`  
WebDAV：`http://127.0.0.1:8080/dav/`

### 命令行

| 参数 | 说明 |
|------|------|
| `-config` | 配置文件路径，默认 `./config.json` |
| `-listen` | 监听地址，覆盖配置与环境变量 |

监听优先级：`-listen` > `TOPWEBDAV_LISTEN` > `config.json`（默认 `127.0.0.1:8080`）。

### 配置 `config.json`

```json
{
  "listen": "127.0.0.1:8080",
  "data_dir": "./data",
  "username": "admin",
  "password_hash": "$2a$10$..."
}
```

## WebDAV 挂载

```bash
rclone config
# type: webdav
# url:  http://127.0.0.1:8080/dav
# user / pass: 你的账号

# 或
cadaver http://127.0.0.1:8080/dav/
```

## 部署（Nginx 反代）

1. 将 `topwebdav` 放到例如 `/opt/topwebdav/`
2. 安装 `deploy/topwebdav.service` 到 `/etc/systemd/system/`，`systemctl enable --now topwebdav`
3. 反代参考 `deploy/nginx.conf.example`（请使用 HTTPS）
4. 端口冲突时修改 `config.json` 的 `listen`，并同步反代 upstream

## 开发

```bash
go test ./...
go run . -listen 127.0.0.1:8080
```

前端为 `web/` 下的静态文件（`index.html` / `style.css` / `app.js`），由 Go `embed` 打进二进制。

## License

[MIT](LICENSE)
