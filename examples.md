# Examples

Recipes for day-to-day use. Replace `https://send.to` with your own instance.

[简体中文](./examples.zh-CN.md)

- [Shell aliases](#shell-aliases)
- [Uploading](#uploading)
- [Downloading](#downloading)
- [Expiry and download limits](#expiry-and-download-limits)
- [Encryption](#encryption)
- [Deleting](#deleting)
- [Archives and backups](#archives-and-backups)
- [Scripting against the JSON API](#scripting-against-the-json-api)
- [Operations](#operations)

---

## Shell aliases

### bash / zsh

Add to `~/.bashrc` or `~/.zshrc`:

```bash
send() {
    if [ $# -eq 0 ]; then
        echo "usage: send <file> [...]" >&2
        return 1
    fi
    for file in "$@"; do
        curl --progress-bar --upload-file "$file" \
            "https://send.to/$(basename "$file")"
        echo
    done
}
```

Pipe support as well — `cat report.txt | send report.txt`:

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

Add to `~/.config/fish/functions/send.fish`:

```fish
function send --description "Share a file with send.to"
    if test (count $argv) -eq 0
        echo "usage: send <file> [...]" >&2
        return 1
    end
    for file in $argv
        curl --progress-bar --upload-file "$file" "https://send.to/"(basename "$file")
        echo
    end
end
```

### PowerShell

Add to `$PROFILE`:

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

## Uploading

```bash
# Single file
curl --upload-file ./notes.md https://send.to/notes.md

# From a pipe
tar czf - ./project | curl --upload-file - https://send.to/project.tar.gz

# Several files in one request (multipart)
curl -F file1=@a.txt -F file2=@b.txt https://send.to/

# With wget
wget --method PUT --body-file=./notes.md https://send.to/notes.md -O - -q

# With HTTPie
http --print=b PUT https://send.to/notes.md < notes.md

# Only the lines you care about
grep -i error /var/log/app.log | curl --upload-file - https://send.to/errors.log
```

Behind HTTP basic auth:

```bash
curl -u user:pass --upload-file ./notes.md https://send.to/notes.md
```

---

## Downloading

```bash
curl https://send.to/<token>/notes.md -o notes.md

# Resume an interrupted transfer — Range requests never consume the
# Max-Downloads budget
curl -C - -o big.iso https://send.to/<token>/big.iso

# With wget
wget https://send.to/<token>/notes.md

# Straight into a pipeline
curl -s https://send.to/<token>/project.tar.gz | tar xz
```

Check a file without spending a download — `HEAD` reports the limits and does
not count:

```bash
curl -I https://send.to/<token>/notes.md
# X-Remaining-Downloads: 3
# X-Remaining-Days: 6
```

---

## Expiry and download limits

```bash
# Expire in 7 days
curl -H "Max-Days: 7" --upload-file ./notes.md https://send.to/notes.md

# Allow 3 completed downloads, then delete the file
curl -H "Max-Downloads: 3" --upload-file ./notes.md https://send.to/notes.md

# Both
curl -H "Max-Days: 1" -H "Max-Downloads: 1" \
     --upload-file ./secret.pdf https://send.to/secret.pdf
```

Only completed transfers count. An aborted download, a link preview that never
reads the body, and any Range request all leave the budget untouched. When the
budget or the deadline runs out, the file is deleted from storage immediately.

An unparseable value is rejected with `400` rather than silently ignored:

```bash
curl -H "Max-Downloads: three" --upload-file ./a.txt https://send.to/a.txt
# Max-Downloads must be a positive integer
```

---

## Encryption

### Server-side (built in)

The server encrypts with OpenPGP AES-256 and stores only ciphertext. The
password is never stored.

```bash
curl -H "X-Encrypt-Password: correct-horse-battery-staple" \
     --upload-file ./contract.pdf https://send.to/contract.pdf

curl -H "X-Decrypt-Password: correct-horse-battery-staple" \
     https://send.to/<token>/contract.pdf -o contract.pdf
```

Without the password you get the PGP-armored ciphertext, which is also what
anyone else who obtains the link gets.

> The server sees the plaintext while encrypting. For a threat model where the
> server itself is untrusted, encrypt before uploading — see below.

### Client-side with gpg

```bash
# Upload
gpg --symmetric --cipher-algo AES256 -o - ./contract.pdf | \
    curl --upload-file - https://send.to/contract.pdf.gpg

# Download
curl -s https://send.to/<token>/contract.pdf.gpg | \
    gpg --decrypt -o contract.pdf
```

### Client-side with age

```bash
age -p -o - ./contract.pdf | curl --upload-file - https://send.to/contract.pdf.age
curl -s https://send.to/<token>/contract.pdf.age | age -d -o contract.pdf
```

---

## Deleting

Every upload returns a deletion URL in the `X-Url-Delete` response header:

```bash
curl -D headers.txt --upload-file ./notes.md https://send.to/notes.md
grep -i x-url-delete headers.txt

curl -X DELETE "https://send.to/<token>/notes.md/<deletion-token>"
```

Capture both the share and delete links in one go:

```bash
read -r url delete < <(
    curl -sS -H "Accept: application/json" \
        --upload-file ./notes.md https://send.to/notes.md |
    jq -r '"\(.url) \(.delete_url)"'
)
echo "share:  $url"
echo "delete: $delete"
```

---

## Archives and backups

### Upload a directory

```bash
tar czf - ./project | curl --upload-file - https://send.to/project.tar.gz
```

### Encrypted database dump

```bash
mysqldump --all-databases |
    gzip |
    gpg --symmetric --cipher-algo AES256 -o - |
    curl -H "Max-Days: 7" --upload-file - https://send.to/backup.sql.gz.gpg
```

Restore:

```bash
curl -s https://send.to/<token>/backup.sql.gz.gpg |
    gpg --decrypt | gunzip | mysql
```

### Combine several stored files into one download

Files already on the server can be streamed back as a single archive without
re-uploading. Note the parentheses:

```bash
curl "https://send.to/(tokenA/a.txt,tokenB/b.txt).zip" -o bundle.zip
curl "https://send.to/(tokenA/a.txt,tokenB/b.txt).tar.gz" -o bundle.tgz
```

### Email the link

```bash
send_and_mail() {
    url=$(curl -sS --upload-file "$1" "https://send.to/$(basename "$1")")
    printf 'Here is the file:\n\n%s\n' "$url" |
        mail -s "File: $(basename "$1")" "$2"
}
send_and_mail ./report.pdf colleague@example.com
```

---

## Scripting against the JSON API

Send `Accept: application/json` and the response carries everything you need,
so there is no need to scrape response headers:

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

A multipart `POST` returns `{"files": [ ... ]}` with one object per file.

### In CI

```yaml
- name: Publish build artifact
  run: |
    url=$(curl -sS -H "Accept: application/json" -H "Max-Days: 14" \
          --upload-file dist/app.tar.gz "$SENDTO_URL/app-${GITHUB_SHA:0:7}.tar.gz" |
          jq -r .url)
    echo "Artifact: $url" >> "$GITHUB_STEP_SUMMARY"
```

### Fail the script when the upload fails

```bash
set -euo pipefail
response=$(curl -sS --fail-with-body -H "Accept: application/json" \
                --upload-file ./a.bin https://send.to/a.bin)
url=$(printf '%s' "$response" | jq -er .url)
```

---

## Operations

```bash
# Liveness, version, backend, uptime
curl -s -H "Accept: application/json" https://send.to/health

# Prometheus metrics
curl -s https://send.to/metrics | grep sendto_

# QR code for a share link (PNG)
curl -s "https://send.to/qr?url=https://send.to/<token>/notes.md&size=256" -o qr.png
```

Virus scanning, when the server is configured for it:

```bash
curl -u user:pass -X PUT --upload-file ./suspect.bin https://send.to/suspect.bin/scan
curl -u user:pass -X PUT --upload-file ./suspect.bin https://send.to/suspect.bin/virustotal
```
