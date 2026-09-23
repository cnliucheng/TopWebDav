# TopWebDav

极简 WebDAV + 网页文件管理，单账号，单二进制，部署在反向代理后面。

## 功能

- WebDAV（`/dav/`），可用 Finder / 资源管理器 / Cyberduck / rclone 挂载
- 网页：列目录、上传、下载、新建文件、新建文件夹、文本编辑、重命名、删除、改密码
- 路径限制在数据目录内（含符号链接检查），密码 bcrypt 存储

## 构建

```bash
go build -o topwebdav .
```

## 运行

```bash
./topwebdav -config ./config.json
# 监听默认 127.0.0.1:8080，可用 -listen 或 TOPWEBDAV_LISTEN 覆盖
```

首次启动自动创建 `config.json` 与 `data/`，账号 `admin` / `admin`，请立刻改密。

## WebDAV 挂载示例

```bash
rclone config  # type webdav, URL http://127.0.0.1:8080/dav, user/pass
# 或
cadaver http://127.0.0.1:8080/dav/
```

## 部署（反代后面）

1. 拷贝 `topwebdav` 到 `/opt/topwebdav/`
2. 安装 `deploy/topwebdav.service` 到 `/etc/systemd/system/` 并 `systemctl enable --now topwebdav`
3. 反代参考 `deploy/nginx.conf.example`（务必 HTTPS）
4. 端口被占用时改 `config.json` 的 `listen` 并同步反代

## 配置

| 字段 | 说明 |
|------|------|
| `listen` | 监听地址，默认 `127.0.0.1:8080` |
| `data_dir` | 文件根目录（权限 0700） |
| `username` | 登录名 |
| `password_hash` | bcrypt 哈希 |

监听优先级：命令行 `-listen` > 环境变量 `TOPWEBDAV_LISTEN` > `config.json`。
