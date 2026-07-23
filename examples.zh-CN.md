# 使用示例

日常用得上的配方。把 `https://send.to` 换成你自己的实例地址。

[English](./examples.md)

- [Shell 别名](#shell-别名)
- [上传](#上传)
- [下载](#下载)
- [过期与下载次数](#过期与下载次数)
- [加密](#加密)
- [删除](#删除)
- [打包与备份](#打包与备份)
- [用 JSON API 写脚本](#用-json-api-写脚本)
- [运维](#运维)

---

## Shell 别名

### bash / zsh

加到 `~/.bashrc` 或 `~/.zshrc`：

```bash
send() {
    if [ $# -eq 0 ]; then
        echo "用法: send <文件> [...]" >&2
        return 1
    fi
    for file in "$@"; do
        curl --progress-bar --upload-file "$file" \
            "https://send.to/$(basename "$file")"
        echo
    done
}
```

同时支持管道 —— `cat report.txt | send report.txt`：

```bash
send() {
    if tty -s; then
        curl --progress-bar --upload-file "$1" "https://send.to/$(basename "$1")"
    else
        curl --progress-bar --upload-file - "https://send.to/${1:-stdin.txt}"
    fi
    echo
}
```

### fish

写到 `~/.config/fish/functions/send.fish`：

```fish
function send --description "用 send.to 分享文件"
    if test (count $argv) -eq 0
        echo "用法: send <文件> [...]" >&2
        return 1
    end
    for file in $argv
        curl --progress-bar --upload-file "$file" "https://send.to/"(basename "$file")
        echo
    end
end
```

### PowerShell

加到 `$PROFILE`：

```powershell
function Send-File {
    param([Parameter(Mandatory)][string]$Path)
    $name = Split-Path $Path -Leaf
    Invoke-RestMethod -Uri "https://send.to/$name" -Method Put -InFile $Path
}
Set-Alias send Send-File
```

### Windows cmd

```bat
curl --upload-file "%1" "https://send.to/%~nx1"
```

---

## 上传

```bash
# 单个文件
curl --upload-file ./notes.md https://send.to/notes.md

# 从管道读
tar czf - ./project | curl --upload-file - https://send.to/project.tar.gz

# 一次传多个（multipart）
curl -F file1=@a.txt -F file2=@b.txt https://send.to/

# 用 wget
wget --method PUT --body-file=./notes.md https://send.to/notes.md -O - -q

# 用 HTTPie
http --print=b PUT https://send.to/notes.md < notes.md

# 只传你关心的那几行
grep -i error /var/log/app.log | curl --upload-file - https://send.to/errors.log
```

实例开了 HTTP Basic Auth 时：

```bash
curl -u user:pass --upload-file ./notes.md https://send.to/notes.md
```

---

## 断点续传大文件

16 MiB 以上的文件会自动分片上传，`send put` 会记住这次会话：再跑一遍同样的命令，
就从服务端停下的那个字节接着传。

```bash
send put ./build.tar.gz --days 7
# ……网络断了……
send put ./build.tar.gz --days 7
# resuming at 1.4 GB of 5.0 GB
```

客户端加密同样支持 —— 密文会从服务端已有的偏移量重新生成：

```bash
send put ./db-dump.sql.gz --e2e
```

只用 curl 的话，自己走这三个调用：

```bash
file=build.tar.gz
size=$(stat -c%s "$file")

session=$(curl -sS -D- -o /dev/null -X POST -H "Upload-Length: $size"     https://send.to/upload/"$file" | awk '/^[Ll]ocation:/{print $2}' | tr -d '')

offset=0
chunk=$((8 * 1024 * 1024))

while [ "$offset" -lt "$size" ]; do
    end=$((offset + chunk - 1))
    [ "$end" -ge "$size" ] && end=$((size - 1))

    dd if="$file" bs=1 skip="$offset" count=$((end - offset + 1)) 2>/dev/null |
        curl -sS -X PATCH -H "Content-Range: bytes $offset-$end/$size"              --data-binary @- "$session"

    # 不要自己假设进度，问服务端：失败的分片根本没落盘
    offset=$(curl -sS -I "$session" | awk '/^[Uu]pload-[Oo]ffset:/{print $2}' | tr -d '')
    [ -z "$offset" ] && break   # 没有偏移量可报，说明已经传完
done
```

## 下载

```bash
curl https://send.to/<token>/notes.md -o notes.md

# 断点续传 —— Range 请求不消耗 Max-Downloads 配额
curl -C - -o big.iso https://send.to/<token>/big.iso

# 用 wget
wget https://send.to/<token>/notes.md

# 直接进管道
curl -s https://send.to/<token>/project.tar.gz | tar xz
```

想查看文件状态又不想消耗下载次数 —— `HEAD` 会返回限额信息且不计数：

```bash
curl -I https://send.to/<token>/notes.md
# X-Remaining-Downloads: 3
# X-Remaining-Days: 6
```

---

## 过期与下载次数

```bash
# 7 天后过期
curl -H "Max-Days: 7" --upload-file ./notes.md https://send.to/notes.md

# 允许完整下载 3 次，之后文件删除
curl -H "Max-Downloads: 3" --upload-file ./notes.md https://send.to/notes.md

# 两个一起用
curl -H "Max-Days: 1" -H "Max-Downloads: 1" \
     --upload-file ./secret.pdf https://send.to/secret.pdf
```

只有**完整传完**的下载才计数。中断的传输、只读取头部的链接预览、以及任何 Range
请求都不消耗配额。配额或期限用尽时，文件会立即从存储中删除。

参数写错会返回 `400`，而不是被静默忽略：

```bash
curl -H "Max-Downloads: three" --upload-file ./a.txt https://send.to/a.txt
# Max-Downloads must be a positive integer
```

---

## 加密

### 服务端加密（内置）

服务端用 OpenPGP AES-256 加密，只存密文，密码不落盘。

```bash
curl -H "X-Encrypt-Password: correct-horse-battery-staple" \
     --upload-file ./contract.pdf https://send.to/contract.pdf

curl -H "X-Decrypt-Password: correct-horse-battery-staple" \
     https://send.to/<token>/contract.pdf -o contract.pdf
```

不带密码取到的是 PGP armor 密文 —— 拿到链接的其他人看到的也是这个。

> 加密过程中服务端能看到明文。如果你的威胁模型里服务端本身不可信，请在上传前
> 自己加密，见下。

### 用 gpg 客户端加密

```bash
# 上传
gpg --symmetric --cipher-algo AES256 -o - ./contract.pdf | \
    curl --upload-file - https://send.to/contract.pdf.gpg

# 下载
curl -s https://send.to/<token>/contract.pdf.gpg | \
    gpg --decrypt -o contract.pdf
```

### 用 age 客户端加密

```bash
age -p -o - ./contract.pdf | curl --upload-file - https://send.to/contract.pdf.age
curl -s https://send.to/<token>/contract.pdf.age | age -d -o contract.pdf
```

---

## 比机器活得久的上传历史

删除链接只出现一次。如果上传的那台机器是一个已经销毁的 CI runner，这个文件在
过期之前就删不掉了。

给上传带上 owner token，列表交给服务端保管：

```bash
send put ./nightly.log --days 30
send ls --remote                  # 任何持有同一个 token 的机器都能看
send rm https://send.to/aB3cD4eF/nightly.log
```

多台机器可以共用一个身份：

```bash
export SENDTO_OWNER_TOKEN="$(openssl rand -base64 32)"   # 放进 CI 的 secret 里
send put ./nightly.log
```

用纯 curl 也一样：

```bash
curl -H "X-Owner-Token: $SENDTO_OWNER_TOKEN" --upload-file nightly.log https://send.to/nightly.log

# 清掉这个 token 上传过的所有文件
curl -sH "X-Owner-Token: $SENDTO_OWNER_TOKEN" -H "Accept: application/json" https://send.to/owner/files |
    jq -r '.files[].delete_url' |
    xargs -rn1 curl -sS -X DELETE
```

服务端只保存 token 的哈希，无法还原出 token；不带这个头的上传依然是匿名的。

## 删除

每次上传的响应头 `X-Url-Delete` 里都带着删除链接：

```bash
curl -D headers.txt --upload-file ./notes.md https://send.to/notes.md
grep -i x-url-delete headers.txt

curl -X DELETE "https://send.to/<token>/notes.md/<deletion-token>"
```

一次拿到分享链接和删除链接：

```bash
read -r url delete < <(
    curl -sS -H "Accept: application/json" \
        --upload-file ./notes.md https://send.to/notes.md |
    jq -r '"\(.url) \(.delete_url)"'
)
echo "分享: $url"
echo "删除: $delete"
```

---

## 打包与备份

### 上传整个目录

```bash
tar czf - ./project | curl --upload-file - https://send.to/project.tar.gz
```

### 加密的数据库备份

```bash
mysqldump --all-databases |
    gzip |
    gpg --symmetric --cipher-algo AES256 -o - |
    curl -H "Max-Days: 7" --upload-file - https://send.to/backup.sql.gz.gpg
```

恢复：

```bash
curl -s https://send.to/<token>/backup.sql.gz.gpg |
    gpg --decrypt | gunzip | mysql
```

### 把服务器上已有的多个文件打成一个包下载

不用重新上传，服务端直接流式打包。注意那对括号：

```bash
curl "https://send.to/(tokenA/a.txt,tokenB/b.txt).zip" -o bundle.zip
curl "https://send.to/(tokenA/a.txt,tokenB/b.txt).tar.gz" -o bundle.tgz
```

### 上传后把链接发邮件

```bash
send_and_mail() {
    url=$(curl -sS --upload-file "$1" "https://send.to/$(basename "$1")")
    printf '文件在这里：\n\n%s\n' "$url" |
        mail -s "文件: $(basename "$1")" "$2"
}
send_and_mail ./report.pdf colleague@example.com
```

---

## 用 JSON API 写脚本

带上 `Accept: application/json`，需要的信息全在响应体里，不用再去抠响应头：

```bash
curl -sS -H "Accept: application/json" \
     -H "Max-Days: 7" -H "Max-Downloads: 5" \
     --upload-file ./build.tar.gz https://send.to/build.tar.gz
```

```json
{
  "url": "https://send.to/aB3cD4eF/build.tar.gz",
  "delete_url": "https://send.to/aB3cD4eF/build.tar.gz/9xK...",
  "filename": "build.tar.gz",
  "size": 10485760,
  "content_type": "application/gzip",
  "encrypted": false,
  "max_downloads": 5,
  "expires_at": "2026-08-01T12:00:00Z"
}
```

Multipart `POST` 返回 `{"files": [ ... ]}`，每个文件一个对象。

### 在 CI 里用

```yaml
- name: Publish build artifact
  run: |
    url=$(curl -sS -H "Accept: application/json" -H "Max-Days: 14" \
          --upload-file dist/app.tar.gz "$SENDTO_URL/app-${GITHUB_SHA:0:7}.tar.gz" |
          jq -r .url)
    echo "构建产物: $url" >> "$GITHUB_STEP_SUMMARY"
```

### 上传失败时让脚本退出

```bash
set -euo pipefail
response=$(curl -sS --fail-with-body -H "Accept: application/json" \
                --upload-file ./a.bin https://send.to/a.bin)
url=$(printf '%s' "$response" | jq -er .url)
```

---

## 运维

```bash
# 存活状态、版本、存储后端、运行时长
curl -s -H "Accept: application/json" https://send.to/health

# Prometheus 指标
curl -s https://send.to/metrics | grep sendto_

# 生成分享链接的二维码（PNG）
curl -s "https://send.to/qr?url=https://send.to/<token>/notes.md&size=256" -o qr.png
```

服务端配置了病毒扫描时：

```bash
curl -u user:pass -X PUT --upload-file ./suspect.bin https://send.to/suspect.bin/scan
curl -u user:pass -X PUT --upload-file ./suspect.bin https://send.to/suspect.bin/virustotal
```
