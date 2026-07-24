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
- [`send` 命令行客户端](#send-命令行客户端)
- [HTTP API](#http-api)
- [配置](#配置)
- [部署](#部署)
- [架构](#架构)
- [性能](#性能)
- [常见问题](#常见问题)
- [更多配方](./examples.zh-CN.md)
- [贡献指南](#贡献指南)
- [许可证](#许可证)

---

## 功能特性

| | |
|---|---|
| **单一静态二进制** | `CGO_ENABLED=0` 编译，以 `scratch` 镜像 + 非 root UID 10001 运行 |
| **可插拔存储** | 本地文件系统、S3（含 Minio / DO Spaces 及任何 S3 兼容服务） |
| **端到端加密** | 客户端 AES-256-GCM；密钥藏在 URL fragment 里，永不到达服务器 |
| **服务端加密** | 通过 `X-Encrypt-Password` 请求头触发 OpenPGP AES-256 加密 |
| **客户端友好** | 兼容 `curl`、`wget`、HTTPie、PowerShell，或任意 HTTP 客户端 |
| **断点续传上传** | 分片会话：5 GB 传到 90% 断掉，从断点接着传 |
| **集合链接** | 一条链接装多个文件，带落地页和整包下载 |
| **无账号历史** | 一个 owner token，在任何机器上列出并删除自己的上传 |
| **自动过期** | 单文件 `Max-Days` / `Max-Downloads` 头部 + 定时清理 |
| **病毒扫描** | 可选 ClamAV 预扫描和 VirusTotal 上报 |
| **TLS** | 自带证书，或通过 Let's Encrypt 自动签发 |
| **鉴权** | HTTP Basic、htpasswd、IP 白 / 黑名单 |
| **安全加固** | 严格 CSP、COOP/CORP、HSTS、slow-loris 超时、常数时间密码比较、按 IP 限流 |
| **优雅关闭** | SIGINT/SIGTERM → `Shutdown(ctx)` → 正在进行的上传完整收尾 |
| **JSON API** | 带 `Accept: application/json` 即返回下载链接、删除链接、大小、过期时间 |
| **可观测性** | 公开的 `/health`（JSON），以及只绑回环的内部监听器上的 `/metrics` |
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

更多配方 —— fish / PowerShell 别名、加密数据库备份、CI 片段 —— 见 [examples.zh-CN.md](./examples.zh-CN.md)。

---

## `send` 命令行客户端

Shell 别名只能给你一个链接。客户端能让你**再找回**这个链接。

```bash
send config add home https://send.to --default     # 配一次
send put report.pdf                                # 以后每天就这一行
```

```
send put ./build.tar.gz --days 7 --max-downloads 3
send get https://send.to/aB3cD4eF/build.tar.gz     # 中断后自动续传
send info https://send.to/aB3cD4eF/build.tar.gz    # 查限额，且不消耗次数
send ls                                            # 这台机器传过什么
send rm https://send.to/aB3cD4eF/build.tar.gz      # 用本地存的删除链接
```

| 命令 | 作用 |
| ---- | ---- |
| `send put <文件>...` | 上传；`-` 读 stdin（需配合 `--name`） |
| `send get <url>` | 下载，自动续传未完成的文件 |
| `send info <url>` | 大小与剩余限额 —— 走 `HEAD`，不消耗下载次数 |
| `send ls` | 本地上传历史：链接、过期时间、删除链接 |
| `send rm <url>` | 用上传时记录的删除链接删文件 |
| `send config` | 命名服务器配置、`--default`、Basic Auth |

参数放在哪都生效：`send put a.txt --days 7` 就是字面意思，不会静默变成「不过期」。

历史文件在 `send config path`（Linux 上是 `~/.config/sendto`）。链接丢了不再等于
永远删不掉文件 —— 这是 `curl` 别名永远做不到的一件事。

`SENDTO_URL`、`SENDTO_USER`、`SENDTO_PASS` 优先于配置文件，CI 里不需要落盘任何状态。

### 端到端加密

`--e2e` 在你的机器上加密，之后才发送任何字节。密钥本地生成，放在链接的
**fragment** 里 —— 也就是 `#` 之后那段，浏览器、代理和访问日志都不会传输它。
服务器收到的是密文，且没有任何办法读取。

```bash
send put contract.pdf --e2e
# https://send.to/aB3cD4eF/contract.pdf#k=9qbxPpQRFJ0xmdYqZ4qh73BppXc3UzK26wyvyXPQxFs
#                                       └── 密钥。丢了文件就没了。

send get 'https://send.to/aB3cD4eF/contract.pdf#k=9qbx…'   # 本地解密
```

Web 界面上是一个勾选框；收件人用浏览器打开链接时在浏览器内解密 —— 密钥不会离开
那个标签页。

AES-256-GCM，64 KiB 分块，分块计数器和末块标记都折进 nonce，因此重排或截断数据流
会变成认证失败，而不是悄悄变短的文件。Go 与浏览器两套实现由
[`test/e2e-interop/run.sh`](./test/e2e-interop/run.sh) 互相校验。

这和 `--encrypt` / `X-Encrypt-Password` 不是一回事：后者是服务端加密，方便、纯
`curl` 就能用，但服务器在过程中能看到明文。当你不信任服务器本身时，用 `--e2e`。

**没有找回机制。** fragment 丢了，谁都解不开 —— 包括运维方和你自己。

---

## HTTP API

| 方法     | 路径                                | 用途                         |
| -------- | ----------------------------------- | ---------------------------- |
| `PUT`    | `/{filename}`                       | 上传单个文件                 |
| `POST`   | `/`                                 | Multipart 上传（单个或多个） |
| `POST`   | `/upload/{filename}`                | 开始分片续传上传             |
| `PATCH`  | `/upload/{id}/{filename}`           | 上传一个分片                 |
| `HEAD`   | `/upload/{id}/{filename}`           | 查询服务端已收到多少字节     |
| `DELETE` | `/upload/{id}/{filename}`           | 放弃这次续传上传             |
| `GET`    | `/{token}/{filename}`               | 下载或预览                   |
| `HEAD`   | `/{token}/{filename}`               | 只取元数据                   |
| `POST`   | `/collection`                       | 把已有的上传收进一条链接     |
| `GET`    | `/c/{token}`                        | 集合：落地页 / JSON / 每行一个链接 |
| `GET`    | `/c/{token}.{zip,tar,tar.gz}`       | 整个集合打包下载             |
| `GET`    | `/({files}).{zip,tar,tar.gz}`       | 将多个文件打包下载           |
| `DELETE` | `/{token}/{filename}/{delToken}`    | 删除文件                     |
| `PUT`    | `/{filename}/scan`                  | ClamAV 扫描（需鉴权 + 限流） |
| `PUT`    | `/{filename}/virustotal`            | 上传到 VirusTotal（需鉴权 + 限流） |
| `GET`    | `/owner/files`                      | 列出这个 owner token 的全部上传 |
| `GET`    | `/health` · `/health.html`          | 健康检查（`Accept: application/json` 返回 JSON） |
| `GET`    | `/metrics`                          | Prometheus 指标 —— **只在内部监听器上**，见下 |
| `GET`    | `/qr?url=`                          | 生成本站分享链接的二维码 PNG |

### 请求头 / 响应头

| 请求头                  | 用途                                                 |
| ----------------------- | ---------------------------------------------------- |
| `Max-Days`              | 多少天后自动过期                                     |
| `Max-Downloads`         | 最多下载次数                                         |
| `X-Owner-Token`         | 把这次上传记进该身份的服务端列表                     |
| `X-Encrypt-Password`    | 服务端加密（OpenPGP AES-256）                        |
| `X-Decrypt-Password`    | 下载时提供解密密码                                   |
| `If-None-Match`        | 已持有该 `ETag` 时跳过传输                     |
| `Accept-Language`      | 错误信息使用的语言（`en`、`zh`、`ja`）          |

| 响应头                  | 含义                                                 |
| ----------------------- | ---------------------------------------------------- |
| `X-Url-Delete`          | 可直接 `DELETE` 的删除链接                           |
| `X-Remaining-Days`      | 还剩多少天过期                                       |
| `X-Remaining-Downloads` | 还剩多少次下载配额                                   |
| `ETag`                 | 已存储内容的校验标识，在该上传的整个生命周期内不变 |
| `Cache-Control`        | 未开启缓存时为 `no-store`                      |
| `Content-Language`     | 错误信息所使用的语言                            |

### 分片续传上传

`PUT` 是全有或全无：5 GB 的构建产物传到 90% 断线，前面的全部作废。续传上传把
一次上传拆成「会话 + 分片」，一次失败最多只损失一个分片。

```bash
size=$(stat -c%s build.tar.gz)

# 建立会话，URL 在 Location 响应头里返回
session=$(curl -sS -D- -o /dev/null -X POST     -H "Upload-Length: $size" -H "Max-Days: 7"     https://send.to/upload/build.tar.gz | awk '/^[Ll]ocation:/{print $2}' | tr -d '')

# 发送一个分片。204 表示「继续」，Upload-Offset 告诉你从哪继续
curl -sS -X PATCH -H "Content-Range: bytes 0-8388607/$size"     --data-binary @chunk-0 "$session"

# 任何异常之后，用 HEAD 问服务端该从哪续
curl -sS -I "$session" | grep -i upload-offset

# 补齐最后一个分片时的响应与普通 PUT 完全一致：
# 返回分享链接，删除链接在 X-Url-Delete 里
curl -sS -X PATCH -H "Content-Range: bytes 8388608-$((size - 1))/$size"     --data-binary @chunk-1 "$session"
```

几条需要知道的规则：

- 分片是**原子**的。传到一半断掉的分片会被丢弃、偏移量不变，所以你续传的位置
  永远是你自己选的位置。
- 偏移量对不上会返回 `409 Conflict`，并在 `Upload-Offset` 里给出正确值，而不是
  把文件写坏。
- 分片先落在 `TEMP_PATH`，全部传完才写入存储后端 —— 中断的上传不会变成一个
  能被别人下载到的半截文件。
- 会话 24 小时过期，过期时连同临时文件一起清掉。
- 这里不接受 `X-Encrypt-Password`：服务端要在分片之间保存明文密码才能做到，
  代价太大。请改用客户端加密（`send put --e2e`）。

`send put` 对 16 MiB 以上的文件自动走这条路径，并把会话记录在本地，下次运行
直接接着传 —— `--e2e` 同样支持，密文会从服务端停下的偏移量重新生成：

```
$ send put build.tar.gz
^C
$ send put build.tar.gz
resuming at 1.4 GB of 5.0 GB
```

如果服务端没有这些端点，客户端会自动退回普通 `PUT`。

### 集合链接

传五个日志文件就得到五条链接，而群里丢掉的总是第五条。集合就是把它们收进一条
链接：

```bash
send put ./*.log --collection --collection-name "nightly logs"
# https://send.to/c/aB3cD4eF
```

浏览器打开是带「打包下载全部」按钮的文件列表；`curl` 拿到的是每行一个分享链接；
带 `Accept: application/json` 则返回完整记录。整包下载就是同一条链接加个后缀：

```bash
curl -sL https://send.to/c/aB3cD4eF.zip -o logs.zip     # 也支持 .tar 和 .tar.gz
curl -s https://send.to/c/aB3cD4eF | xargs -n1 curl -O  # 或者一个个下
```

集合也可以由已经存在的上传拼出来，客户端内部就是这么做的：

```bash
curl -X POST -H "Content-Type: application/json" \
    -d '{"name":"nightly logs","files":["https://send.to/aB3/app.log","https://send.to/cD4/worker.log"]}' \
    https://send.to/collection
```

集合本身不持有任何字节：

- 删除集合不会动里面的文件 —— 别人手里可能正拿着其中某一条链接。每个文件保留
  自己的删除链接和自己的限制。
- 成员过期、下载次数用尽或被删除时会自动从列表里消失；一个成员都不剩的集合直接
  返回 404。
- 下载整包会给每个成员各记一次下载，和单独下载它们完全一样。
- 每个集合最多 100 个文件。
- `--collection` 与 `--e2e` 不能同时用：每个文件有自己的密钥，一条链接装不下
  多个密钥，除非把解密能力交给服务端。

### 服务端上传历史

`send` 客户端在本地记录上传过什么，但如果上传的那台机器是一个已经销毁的 CI
runner，这份记录连同所有删除链接一起没了。

带上 owner token，改由服务端来记：

```bash
curl -H "X-Owner-Token: $SENDTO_OWNER_TOKEN" --upload-file build.log https://send.to/build.log

curl -H "X-Owner-Token: $SENDTO_OWNER_TOKEN" https://send.to/owner/files
# 每行一个分享链接；带 Accept: application/json 则返回完整记录
```

它仍然不是账号：

- 服务端只存 **token 的 sha256**，永远不存 token 本身。那是一个索引名，不是它
  能交给别人的凭证。
- 持有 token 就是全部授权，所以请当成密码对待：它能列出用它上传的每个文件的
  分享链接和删除链接。
- 文件消失（被删除、过期、下载次数用尽）时，对应条目也会消失；每次读取列表时
  顺手清理。
- 不带这个头的上传依然是匿名的，和以前完全一样。
- 每个 token 最多保留 200 条，超出的按时间淘汰。

`send` 从配置目录里的主密钥（`owner.key`，权限 0600）为每个服务器派生一个独立
token，因此你上传的那台服务器学不到任何能拿去别处重放的东西：

```bash
send put build.log            # 自动带上 token
send ls --remote              # 这个身份在服务端的全部上传，任何机器上都能看
send rm https://send.to/aB3cD4eF/build.log   # 用服务端保存的删除链接
```

想让多台机器共用同一个身份（比如多个 CI runner 发布到同一份列表），设置
`SENDTO_OWNER_TOKEN` 即可。

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

### 缓存与 CDN

文件一旦存好就不再改变，所以每个下载响应都带 `ETag`，并对带 `If-None-Match`
的请求返回 `304 Not Modified`。这一部分始终开启，对运维方没有任何代价——已经
持有该文件的客户端既不用传输正文，也不会产生一次存储读取。

**缓存是否可以保存响应**是另一个问题，默认答案是不可以。用 `--cache-max-age`
打开：

```
CACHE_MAX_AGE=600 ./send.to --provider local --basedir /data
```

即便打开，也只有「缓存不会改变行为」的上传才会变成
`public, max-age=…, immutable`：

- **带 `Max-Downloads` 的上传永远不可缓存。** 该配额统计的是完成的下载，而缓存
  命中时源站根本不知道发生过一次下载。
- **服务端加密的上传永远不可缓存。** 它们要按请求携带的 `X-Decrypt-Password`
  解密，缓存处理不当就会把一个访问者的明文发给另一个人。
- 缓存时长会被文件自身的 `Max-Days` 截断，缓存副本不会比链接活得更久。
- 错误响应（包括 404）始终是 `no-store`，缓存不会把一次临时故障留下来顶替文件。

作为交换你要接受的代价：**已删除的文件在缓存中最多还能被取到
`CACHE_MAX_AGE` 那么久**，并且由缓存服务的下载不会出现在 `/metrics` 里。选值时
请把这一点算进去；10 分钟就能拿到大部分收益，而暴露窗口小得多。

### 语言

错误信息会按 `Accept-Language` 返回中文、英文或日文——和 Web 界面支持的三种语言
一致。不带该请求头时返回英文，所以已有脚本的行为不变。带内部细节的错误（存储
故障、扫描器异常）刻意保持英文：它们是给看日志的人用的。

Web UI 的 `/api-docs` 页面有实时 API 参考。

---

## 配置

每个 CLI 参数都有对应的环境变量（`--listener` ↔ `LISTENER`）。运行 `./send.to --help` 查看完整列表。最常用的几个：

| Flag / 环境变量                     | 默认值    | 说明                                           |
| ----------------------------------- | --------- | ---------------------------------------------- |
| `--listener` / `LISTENER`           | `:18080`   | HTTP 监听地址                                  |
| `--tls-listener` / `TLS_LISTENER`   | 空        | 启用原生 HTTPS                                 |
| `--provider` / `PROVIDER`           | —         | `local` \| `s3`                                |
| `--basedir` / `BASEDIR`             | —         | `local` 存储后端的数据目录                     |
| `--max-upload-size`                 | `0`       | 每次上传大小上限（KB）；`0` = 无限             |
| `--temp-path` / `TEMP_PATH`         | 系统临时目录 | 上传暂存目录，必须是磁盘路径                |
| `--max-storage-size`              | `0`          | 实例最多存多少 KB；`0` = 不限            |
| `--max-temp-size`                 | `0`          | 未完成上传最多占多少 KB 临时空间；`0` = 不限 |
| `--per-ip-upload-quota`           | `0`          | 单个来源 IP 每小时最多上传多少 KB；`0` = 不限 |
| `--cache-max-age`                 | `0`          | 浏览器 / CDN 可缓存下载多少秒；`0` = 不缓存 |
| `--rate-limit`                      | `0`       | 每 IP 每分钟最大请求数，所有路由共用同一份配额 |
| `--purge-days`                      | `0`       | 清理 N 天前的旧文件                            |
| `--shutdown-timeout`                | `30s`     | 退出时等待进行中请求完成的最长时间             |
| `--http-auth-user` / `_pass`        | 空        | HTTP Basic Auth 用户名 / 密码                  |
| `--cors-domains`                    | 空        | 允许的 CORS 源，逗号分隔                       |
| `--clamav-host`                     | 空        | 例如 `tcp://clamav:3310`                       |
| `--virustotal-key`                  | 空        | 启用 `/{file}/virustotal` 端点                 |
| `--lets-encrypt-hosts`              | 空        | Let's Encrypt 自动签发的域名列表               |

### 公网实例值得设的三个上限

`MAX_UPLOAD_SIZE` 只限制单个文件，等于没限制：同一个合规大小的文件传够多次就
能把磁盘填满。两个总量上限补上这个缺口。

| | |
|---|---|
| `MAX_STORAGE_SIZE` | 存储后端最多保存多少字节（KB） |
| `MAX_TEMP_SIZE` | 未完成上传最多占用多少临时空间（KB） |

两者都返回 **507 Insufficient Storage**，而不是传到一半才失败。续传上传会被检查
两次：开会话时按声明的总大小检查一次，让注定放不下的传输在传第一个字节之前就被
拒绝；每个分片再检查一次，因为多个会话可能在都还没占空间时就都通过了第一次检查。

真正防滥用的是 `MAX_TEMP_SIZE`：未完成的会话按设计要保留 24 小时，没有这个上限，
任何人都可以不断开会话、每个传到接近完成然后放着，让字节堆在 `TEMP_PATH` 上 ——
而在自带的 compose 配置里，那就是数据卷本身。

但这两个总量都不区分**是谁**把实例填满的。一个客户端可以自己把
`MAX_STORAGE_SIZE` 用光，让其他所有人只收到 507；`RATE_LIMIT` 也拦不住 ——
每分钟 60 个请求就是每分钟 60 个大文件。

| | |
|---|---|
| `PER_IP_UPLOAD_QUOTA` | 单个来源 IP 每小时可上传多少 KB |

它返回 **429 Too Many Requests**，因为和上面两个不同，它会恢复 —— 是令牌桶，
连续补充，不是整点清零。续传上传在开会话时就按声明的总大小扣掉，会话被放弃也不
退还，所以「不断开会话然后走人」绕不过去。这个值要设得比 `MAX_UPLOAD_SIZE` 大，
否则按上限大小上传的文件永远传不成功 —— 这种配置服务端启动时会告警。

来源 IP 只有在对端是回环地址或内网段时才采信 `X-Forwarded-For` / `X-Real-IP`，
所以直连的客户端没法自己伪造出一份新配额。如果代理不设这两个头，所有上传会共用
同一个桶。

依赖它们之前需要知道两点：

- **`MAX_STORAGE_SIZE` 是计数器，不是实测值。** 启动时从后端取一次基线，之后在
  内存里累加，因为实测意味着列举整个 bucket。如果有人绕过服务端删文件，它会漂移，
  重启会重新取基线。自带的两个后端都能统计；换成统计不了的后端，进程会**拒绝
  启动**，而不是假装限制生效。
- **三个上限都是单进程范围的。** 负载均衡后面的多个副本各算各的，每个副本的
  per-IP 令牌桶也各自存在内存里，所以请求被分到 N 个副本的客户端就有 N 份配额。
  续传上传同样只支持单实例：会话的临时文件在本地磁盘上，在一个副本上开的会话没法
  在另一个副本上续。请用单实例，或者用 sticky session + 共享临时卷。

### 内部监听器

`/metrics` **不在**公开监听器上。它在第二个 server 上，默认绑 `127.0.0.1:6060`；
开了 `--profiler` 时 pprof 也挂在同一个地方：

| | |
|---|---|
| `--profile-listener` / `PROFILE_LISTENER` | 内部监听器的绑定地址 |
| `--profiler` / `PROFILER` | 额外挂上 `/debug/pprof/` |

单看每个计数器都不算机密，但合起来能让陌生人知道实例有多满、限流多久触发一次
——而触发限流的往往正是他自己。`/health` 保持公开，因为负载均衡要从外面探活。

这两个参数互相独立。以前设了 `--profile-listener` 会隐含开启 `--profiler`，那是
监听器上只有 pprof 时的写法；现在为了让采集器能抓而把地址从回环挪开，绝不能顺带
把「能 dump 出上传文件内容的堆」一起公开。

Docker 里默认地址别的容器访问不到。要采集就设 `PROFILE_LISTENER=0.0.0.0:6060`，
把采集器放同一个 network，并且**不要**把这个端口 publish 出去。

`/metrics` 暴露了 `sendto_storage_used_bytes`、`sendto_storage_limit_bytes`、
`sendto_temp_used_bytes`、`sendto_temp_limit_bytes` —— 该对这些做告警，等到 507
出现时已经晚了。

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
- [ ] 从内部监听器采集 `/metrics`（上传数、下载数、字节数、429 次数、过期清理数）。
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
                              ┌──────────────┴──────────────┐
                              ▼                             ▼
                          本地文件系统                   S3 / Minio
```

### 代码结构

```
.
├── cmd/                CLI 参数解析（urfave/cli）
├── server/             HTTP 处理器、认证、安全、存储
│   └── storage/        local | s3 两种后端
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

仅当上传方传了 `X-Encrypt-Password` 时才加密（OpenPGP AES-256），否则按原样存储。用 S3 后端时可以叠加 bucket 自身的服务端加密。
</details>

<details>
<summary><b>能放在反向代理的子路径下吗？</b></summary>

可以。设置 `--proxy-path /send`（或 `PROXY_PATH=/send`）后，代理将请求转到容器，响应中所有 URL 会自动按前缀重写。
</details>

<details>
<summary><b>如何备份上传的文件？</b></summary>

`local` 后端：备份 `--basedir` 目录即可（`rsync`、ZFS snapshot 等皆可）。S3：使用 bucket 自身的复制 / 版本控制能力。
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
