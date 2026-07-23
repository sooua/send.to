<div align="center">

# send.to

**命令行一键文件分享 —— 需要的时候还有一个干净的 Web 界面。**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://golang.org/)
[![Node](https://img.shields.io/badge/Node-22%2B-339933?logo=node.js)](https://nodejs.org/)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker)](./Dockerfile)
[![CI](https://img.shields.io/badge/CI-tested-brightgreen)](./.github/workflows/test.yml)

[English](./README.md) · [简体中文](./README.zh-CN.md)

</div>

---

## 为什么选 send.to

你手里有个文件，别人需要它。你想要一个可以丢到聊天、粘到 CI 日志、五分钟后在生产机器上 `curl` 拿走、然后自动消失的链接 —— `send.to` 就干这一件事，也只干这一件事。

```bash
curl --upload-file ./build.tar.gz https://send.to/build.tar.gz
# → https://send.to/aB3cD4eF/build.tar.gz

curl https://send.to/aB3cD4eF/build.tar.gz -o build.tar.gz
```

一个静态 Go 二进制。一个 52 MB 的 Docker 镜像。无数据库。无账号。文件完全属于你自己。

---

## 目录

- [功能特性](#功能特性)
- [快速开始](#快速开始)
- [使用示例](#使用示例)
- [HTTP API](#http-api)
- [配置](#配置)
- [部署](#部署)
- [架构](#架构)
- [性能](#性能)
- [常见问题](#常见问题)
- [贡献指南](#贡献指南)
- [许可证](#许可证)

---

## 功能特性

| | |
|---|---|
| **单一静态二进制** | `CGO_ENABLED=0` 编译，以 `scratch` 镜像 + 非 root UID 10001 运行 |
| **可插拔存储** | 本地文件系统、S3（含 Minio / DO Spaces）、Google Drive、Storj |
| **服务端加密** | 通过 `X-Encrypt-Password` 请求头触发 OpenPGP AES-256 加密 |
| **客户端友好** | 兼容 `curl`、`wget`、HTTPie、PowerShell，或任意 HTTP 客户端 |
| **自动过期** | 单文件 `Max-Days` / `Max-Downloads` 头部 + 定时清理 |
| **病毒扫描** | 可选 ClamAV 预扫描和 VirusTotal 上报 |
| **TLS** | 自带证书，或通过 Let's Encrypt 自动签发 |
| **鉴权** | HTTP Basic、htpasswd、IP 白 / 黑名单 |
| **安全加固** | 严格 CSP、COOP/CORP、HSTS、slow-loris 超时、常数时间密码比较、按 IP 限流 |
| **优雅关闭** | SIGINT/SIGTERM → `Shutdown(ctx)` → 正在进行的上传完整收尾 |
| **JSON API** | 带 `Accept: application/json` 即返回下载链接、删除链接、大小、过期时间 |
| **可观测性** | `/health`（JSON）与 `/metrics`（Prometheus 文本格式） |
| **现代 Web UI** | Astro 5 + React 19 + Tailwind 4：多文件队列、粘贴上传、单文件 ETA、失败重试、有效期 / 下载次数 / 加密密码选项、二维码，以及保存删除链接的本地上传历史 |

---

## 快速开始

三种方式任选其一，最终都是同一个服务跑在 <http://localhost:18080>。

### Docker Compose（推荐）

只需本地装好 **Docker**。

```bash
git clone https://github.com/sooua/send.to && cd send.to
./scripts/docker.sh up
```

就这么简单。这个脚本会：

1. 如果没有 `.env`，从 `.env.example` 复制一份（你可以改来覆盖默认值）。
2. 运行 `docker compose up -d --build`。
3. 轮询 `/health.html` 最多 30 秒，健康后把 URL 打印出来。

| 命令                           | 作用                                                |
| ------------------------------ | --------------------------------------------------- |
| `./scripts/docker.sh up`       | 后台构建并启动                                      |
| `./scripts/docker.sh down`     | 停止并移除容器（数据卷保留）                        |
| `./scripts/docker.sh purge`    | 停止并**彻底删除数据卷**（会先问你）                |
| `./scripts/docker.sh logs`     | 跟踪容器日志                                        |

镜像约 52 MB，非 root 运行、只读根文件系统、无 shell、内置 `HEALTHCHECK`。

### 原生部署（不用 Docker）

需要 **Go 1.25+** 和 **Node 22+**。

<details>
<summary><b>Linux / macOS / WSL</b></summary>

```bash
./scripts/deploy.sh              # 前台运行，Ctrl+C 触发优雅关闭
./scripts/deploy.sh --daemon     # 后台运行，日志在 build/sendto.log
./scripts/deploy.sh --stop       # 停止后台实例
PORT=9000 ./scripts/deploy.sh    # 覆盖端口
```

</details>

<details>
<summary><b>Windows（PowerShell）</b></summary>

```powershell
.\scripts\deploy.ps1                 # 前台运行
.\scripts\deploy.ps1 -Daemon         # 后台运行
.\scripts\deploy.ps1 -Stop           # 停止
$env:PORT="9000"; .\scripts\deploy.ps1
```

</details>

所有产物都在 `./build/` 和 `./data/` 下 —— 卸载只需 `rm -rf build data web/dist web/node_modules`。

### 源码构建

```bash
make build                        # Go 二进制 + Web 前端
./send.to --provider local --basedir ./data --web-path ./web/dist
```

或者 `make dev`：Go 服务和 Astro 开发服务并行热重载。

---

## 使用示例

### 上传

```bash
# 普通上传
curl --upload-file ./notes.md https://send.to/notes.md

# 加密上传（服务端只存密文）
curl -H "X-Encrypt-Password: s3cret" \
     --upload-file ./notes.md https://send.to/notes.md

# 限制最多 5 次下载，7 天后过期
curl -H "Max-Downloads: 5" -H "Max-Days: 7" \
     --upload-file ./notes.md https://send.to/notes.md

# 一次上传多个文件（multipart）
curl -F file1=@a.txt -F file2=@b.txt https://send.to/
```

### 下载

```bash
curl https://send.to/<token>/notes.md -o notes.md

# 下载时解密
curl -H "X-Decrypt-Password: s3cret" \
     https://send.to/<token>/notes.md -o notes.md

# 断点续传（Range 请求）
curl -C - -o big.iso https://send.to/<token>/big.iso
```

### 打包下载

```bash
# 把多个已存储文件合并为单个流
curl https://send.to/(tokenA/a.txt,tokenB/b.txt).zip -o bundle.zip
curl https://send.to/(tokenA/a.txt,tokenB/b.txt).tar.gz -o bundle.tgz
```

### 删除

上传响应里的 `X-Url-Delete` 头就是删除 URL，用 `DELETE` 方法请求它即可。

```bash
curl -X DELETE https://send.to/<token>/notes.md/<deletion-token>
```

### Shell 小工具

把这段贴到 `~/.bashrc` / `~/.zshrc`：

```bash
send() { curl --progress-bar --upload-file "$1" "https://send.to/$(basename "$1")"; }
# 用法：   send ./report.pdf
```

---

## HTTP API

| 方法     | 路径                                | 用途                         |
| -------- | ----------------------------------- | ---------------------------- |
| `PUT`    | `/{filename}`                       | 上传单个文件                 |
| `POST`   | `/`                                 | Multipart 上传（单个或多个） |
| `GET`    | `/{token}/{filename}`               | 下载或预览                   |
| `HEAD`   | `/{token}/{filename}`               | 只取元数据                   |
| `GET`    | `/({files}).{zip,tar,tar.gz}`       | 将多个文件打包下载           |
| `DELETE` | `/{token}/{filename}/{delToken}`    | 删除文件                     |
| `PUT`    | `/{filename}/scan`                  | ClamAV 扫描（需鉴权 + 限流） |
| `PUT`    | `/{filename}/virustotal`            | 上传到 VirusTotal（需鉴权 + 限流） |
| `GET`    | `/health` · `/health.html`          | 健康检查（`Accept: application/json` 返回 JSON） |
| `GET`    | `/metrics`                          | Prometheus 指标              |
| `GET`    | `/qr?url=`                          | 生成本站分享链接的二维码 PNG |

### 请求头 / 响应头

| 请求头                  | 用途                                                 |
| ----------------------- | ---------------------------------------------------- |
| `Max-Days`              | 多少天后自动过期                                     |
| `Max-Downloads`         | 最多下载次数                                         |
| `X-Encrypt-Password`    | 服务端加密（OpenPGP AES-256）                        |
| `X-Decrypt-Password`    | 下载时提供解密密码                                   |

| 响应头                  | 含义                                                 |
| ----------------------- | ---------------------------------------------------- |
| `X-Url-Delete`          | 可直接 `DELETE` 的删除链接                           |
| `X-Remaining-Days`      | 还剩多少天过期                                       |
| `X-Remaining-Downloads` | 还剩多少次下载配额                                   |

### JSON 响应

上传时带上 `Accept: application/json`，就能拿到结构化结果，不用再去抠
`X-Url-Delete` 响应头：

```bash
curl -H "Accept: application/json" -H "Max-Days: 7"      --upload-file notes.md https://send.to/notes.md
```

```json
{
  "url": "https://send.to/aB3cD4eF/notes.md",
  "delete_url": "https://send.to/aB3cD4eF/notes.md/9xK…",
  "filename": "notes.md",
  "size": 4096,
  "content_type": "text/x-markdown",
  "encrypted": false,
  "expires_at": "2026-07-30T08:06:49Z"
}
```

Multipart `POST` 返回 `{"files": [ … ]}`，每个文件一个对象。

### 下载计数规则

`Max-Downloads` 只统计**完整传输完成**的下载：

- **Range 请求不计数。** `curl -C -` 断点续传、视频拖拽进度、字节范围探测都
  不会消耗配额。
- 传输中途失败或被中断的不计数。
- 配额或 `Max-Days` 用尽时，文件会**立即从存储中删除**，不再等 `--purge-days`。

Web UI 的 `/api-docs` 页面有实时 API 参考。

---

## 配置

每个 CLI 参数都有对应的环境变量（`--listener` ↔ `LISTENER`）。运行 `./send.to --help` 查看完整列表。最常用的几个：

| Flag / 环境变量                     | 默认值    | 说明                                           |
| ----------------------------------- | --------- | ---------------------------------------------- |
| `--listener` / `LISTENER`           | `:18080`   | HTTP 监听地址                                  |
| `--tls-listener` / `TLS_LISTENER`   | 空        | 启用原生 HTTPS                                 |
| `--provider` / `PROVIDER`           | —         | `local` \| `s3` \| `gdrive` \| `storj`         |
| `--basedir` / `BASEDIR`             | —         | `local` 存储后端的数据目录                     |
| `--max-upload-size`                 | `0`       | 每次上传大小上限（KB）；`0` = 无限             |
| `--temp-path` / `TEMP_PATH`         | 系统临时目录 | 上传暂存目录，必须是磁盘路径                |
| `--rate-limit`                      | `0`       | 每 IP 每分钟最大请求数，所有路由共用同一份配额 |
| `--purge-days`                      | `0`       | 清理 N 天前的旧文件                            |
| `--shutdown-timeout`                | `30s`     | 退出时等待进行中请求完成的最长时间             |
| `--http-auth-user` / `_pass`        | 空        | HTTP Basic Auth 用户名 / 密码                  |
| `--cors-domains`                    | 空        | 允许的 CORS 源，逗号分隔                       |
| `--clamav-host`                     | 空        | 例如 `tcp://clamav:3310`                       |
| `--virustotal-key`                  | 空        | 启用 `/{file}/virustotal` 端点                 |
| `--lets-encrypt-hosts`              | 空        | Let's Encrypt 自动签发的域名列表               |

容器部署全部配置见 [`docker-compose.yml`](./docker-compose.yml) 和 [`.env.example`](./.env.example)。

---

## 部署

### TLS / HTTPS

生产环境有两种方案：

**1. 反向代理终止 TLS（最常见）** —— `send.to` 本身跑 HTTP，让 Caddy / Nginx / Traefik / Cloudflare 终止 TLS：

```caddy
files.example.com {
    reverse_proxy 127.0.0.1:18080
}
```

记得让代理带上 `X-Forwarded-Proto: https` 头，这样 HSTS / CSP 会自动生效。

**2. 原生 TLS**

```bash
./send.to --tls-listener :8443 \
  --tls-cert-file fullchain.pem --tls-private-key privkey.pem
```

或者 Let's Encrypt 自动签发：

```bash
./send.to --lets-encrypt-hosts files.example.com
```

### 生产 checklist

- [ ] 设置 `MAX_UPLOAD_SIZE`，保护磁盘 / S3 账单。
- [ ] 设置 `RATE_LIMIT`，防单 IP 滥用。
- [ ] 务必走 HTTPS（浏览器在非安全上下文会拒绝 `clipboard.writeText`）。
- [ ] 不允许匿名上传的实例打开 Basic Auth。
- [ ] 存储卷挂在有配额的分区上。
- [ ] 监控 `HEALTHCHECK` 状态（`docker ps`）或定期打 `/health`。
- [ ] 采集 `/metrics`（上传数、下载数、字节数、429 次数、过期清理数）。
- [ ] 把 `TEMP_PATH` 放在磁盘路径上 —— 没有 `Content-Length` 的上传，以及开启
      ClamAV 预扫描时的所有上传，都会先落盘到这里。compose 已把它指向数据卷，
      因为容器里的 `/tmp` 是很小的 tmpfs。

### 升级

```bash
git pull
./scripts/docker.sh up            # 重新构建并滚动容器
# 或：
./scripts/deploy.sh --stop && ./scripts/deploy.sh --daemon
```

关闭过程受 `SHUTDOWN_TIMEOUT` 限制，在超时前所有进行中的上传都能完整收尾。

---

## 架构

```
┌──────────────┐       HTTPS        ┌────────────────┐
│   浏览器 /   │ ─────────────────▶ │  反向代理      │
│   curl /     │                    │  （可选）      │
│   HTTPie     │ ◀───────────────── │                │
└──────────────┘                    └────────┬───────┘
                                             │ HTTP
                                             ▼
                                    ┌────────────────┐
                                    │   send.to      │
                                    │   Go 二进制    │
                                    │                │
                                    │ • mux 路由     │
                                    │ • 限流         │
                                    │ • CSP / HSTS   │
                                    │ • OpenPGP 加密 │
                                    │ • ClamAV /     │
                                    │   VirusTotal   │
                                    └────────┬───────┘
                                             │
                  ┌──────────────┬───────────┴───────────┬──────────────┐
                  ▼              ▼                       ▼              ▼
              本地文件系统    S3 / Minio           Google Drive      Storj
```

### 代码结构

```
.
├── cmd/                CLI 参数解析（urfave/cli）
├── server/             HTTP 处理器、认证、安全、存储
│   └── storage/        local | s3 | gdrive | storj 四种后端
├── internal/clamd/     内嵌的 ClamAV 守护进程客户端
├── web/                Astro 5 + React 19 前端
├── scripts/            deploy.sh · deploy.ps1 · docker.sh 一键脚本
├── test/               端到端 smoke test + Windows 信号辅助工具
├── Dockerfile          三阶段构建：Web → Go → scratch
├── docker-compose.yml  生产就绪的 compose（含 healthcheck、只读 FS）
├── Makefile            build · test · lint · vuln · smoke · docker
└── .github/workflows/  CI：Go test+race+coverage、lint、govulncheck、web build
```

---

## 性能

单核笔记本（Windows 11、本地 FS 后端、冷启动）的参考数据：

| 指标                          | 数值                  |
| ----------------------------- | --------------------- |
| 冷启动耗时                    | < 50 ms               |
| 镜像大小                      | 51.8 MB               |
| 空闲内存                      | ≈ 20 MB               |
| 本地上传吞吐                  | 近似磁盘线速          |
| 并发上传（10 × 并发）         | 全部成功、无错、token 各不相同 |
| 优雅关闭最大耗时              | 受 `SHUTDOWN_TIMEOUT` 约束（默认 30 秒）|

仓库自带一个 22 条断言的 smoke test（健康、安全头、完整上传/下载/删除、加密、限制、并发、优雅关闭），位于 [`test/smoke/`](./test/smoke/)，执行 `make smoke` 即可。

---

## 常见问题

<details>
<summary><b>有文件大小限制吗？</b></summary>

没有硬编码限制。通过 `--max-upload-size`（单位 KB）配置。不设置就只受存储后端磁盘容量限制。
</details>

<details>
<summary><b>文件保留多久？</b></summary>

上传端用 `Max-Days` / `Max-Downloads` 头自己设。运维端用 `--purge-days` 设全局下限。任一条件到达 → 文件在下次访问或下次定时清理时被删除。
</details>

<details>
<summary><b>文件是否静态加密？</b></summary>

仅当上传方传了 `X-Encrypt-Password` 时才加密（OpenPGP AES-256），否则按原样存储。S3 / Storj 后端可以叠加使用它们自身的服务端加密。
</details>

<details>
<summary><b>能放在反向代理的子路径下吗？</b></summary>

可以。设置 `--proxy-path /send`（或 `PROXY_PATH=/send`）后，代理将请求转到容器，响应中所有 URL 会自动按前缀重写。
</details>

<details>
<summary><b>如何备份上传的文件？</b></summary>

`local` 后端：备份 `--basedir` 目录即可（`rsync`、ZFS snapshot 等皆可）。S3 / Storj：使用后端自身的复制 / 快照能力。
</details>

<details>
<summary><b>能在离线 / 气隙网络中使用吗？</b></summary>

可以。二进制零运行时依赖，镜像无外部依赖，Web UI 是完全静态的 Astro 产物。唯一的出网行为是访问你配置的存储后端（以及可选的 ClamAV / VirusTotal）。
</details>

---

## 贡献指南

开发环境搭建、构建 / 测试命令、PR 规范见 [CONTRIBUTING.md](./CONTRIBUTING.md)。

**安全漏洞**请按 [SECURITY.md](./SECURITY.md) 流程私下报告，不要发公开 issue。

第三方代码归属统一在 [THIRD_PARTY_LICENSES.md](./THIRD_PARTY_LICENSES.md)。

---

## 许可证

[MIT](./LICENSE)
