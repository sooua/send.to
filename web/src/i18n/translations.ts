// i18n 翻译系统
export const languages = {
  en: "English",
  zh: "中文",
  ja: "日本語",
} as const;

export type Lang = keyof typeof languages;
export const defaultLang: Lang = "en";

export const translations: Record<Lang, Record<string, string>> = {
  en: {
    // Navbar
    "nav.home": "Home",
    "nav.apiDocs": "API Docs",
    "nav.useCases": "Use Cases",
    "nav.about": "About",
    // Hero
    "hero.title": "send.to",
    "hero.subtitle":
      "Easy and fast file sharing from the command line. Upload files and get a shareable link instantly.",
    // Upload
    "upload.dropzone": "Drop a file here or",
    "upload.hint": "Files are encrypted in transit and auto-expire",
    "upload.uploading": "Uploading...",
    "upload.complete": "Upload complete",
    "upload.another": "Upload another file",
    "upload.shareLink": "Share link",
    "upload.downloadCmd": "Download with curl",
    "upload.deleteCmd": "Delete file",
    "upload.backendError":
      "Backend not running. Start it with: go run main.go --provider local --basedir /tmp/send.to --listener :18080",
    "upload.eta": "ETA",
    "upload.cancel": "Cancel",
    "upload.close": "Close",
    "upload.copyLink": "Copy link",
    "upload.copyCurl": "Copy curl command",
    "upload.copyDelete": "Copy delete command",
    "upload.copyFailed": "Copy failed — please copy manually",
    "upload.tooLarge": "File is too large",
    "upload.dropzoneMulti": "Drop files here or",
    "upload.hintMulti": "Multiple files, paste from clipboard, or click to browse",
    "upload.foldersUnsupported": "Folders are not supported — zip it first",
    "upload.options": "Options",
    "upload.optionsHide": "Hide options",
    "upload.expiryDays": "Expire after (days)",
    "upload.expiryNever": "Never",
    "upload.maxDownloads": "Download limit",
    "upload.maxDownloadsUnlimited": "Unlimited",
    "upload.password": "Encrypt with password",
    "upload.passwordPlaceholder": "Leave empty for no encryption",
    "upload.passwordHint": "Recipients must pass this password to download.",
    "upload.passwordShow": "Show password",
    "upload.passwordHide": "Hide password",
    "upload.queued": "Queued",
    "upload.failed": "Failed",
    "upload.retry": "Retry",
    "upload.remove": "Remove",
    "upload.cancelAll": "Cancel all",
    "upload.results": "Uploaded",
    "upload.expires": "Expires",
    "upload.downloadsLeft": "downloads left",
    "upload.qrShow": "Show QR code",
    "upload.qrHide": "Hide QR code",
    "upload.qrAlt": "QR code for the share link",
    "upload.history": "Recent uploads",
    "upload.historyEmpty": "Nothing uploaded from this browser yet.",
    "upload.historyClear": "Clear history",
    "upload.historyNote":
      "Stored in this browser only — clearing site data loses the delete links.",
    "upload.deleteNow": "Delete",
    "upload.deleted": "Deleted",
    "upload.deleteFailed": "Delete failed",
    "upload.copied": "Copied",
    "upload.encryptedBadge": "Encrypted",
    "upload.decryptCmd": "Download and decrypt",
    "upload.copyDecrypt": "Copy decrypt command",
    "preview.download": "Download file",
    "preview.downloadsLeft": "Downloads left",
    "preview.daysLeft": "Days left",
    "preview.reveal": "Show preview",
    "preview.revealNote": "This file has a download limit — previewing it uses one.",
    "preview.badLink": "That link doesn't look right",
    "preview.unavailable": "This file is no longer available",
    "preview.unavailableHint": "It may have expired, hit its download limit, or been deleted.",
    "preview.maybeEncrypted":
      'This file may be encrypted. If so, download it with: curl -H "X-Decrypt-Password: <password>" <url>',
    "upload.e2e": "End-to-end encrypt",
    "upload.e2eHint":
      "Encrypted in this browser. The key goes in the link after #, which is never sent to the server — so the server stores a file it cannot read.",
    "upload.e2eBadge": "End-to-end encrypted",
    "upload.e2eKeyWarning":
      "Keep the whole link. Without the part after # the file is unrecoverable — nobody can reset it.",
    "upload.e2eEncrypting": "Encrypting…",
    "preview.e2eTitle": "End-to-end encrypted",
    "preview.e2eDecrypting": "Decrypting in your browser…",
    "preview.e2eReady": "Decrypted in your browser — the server never saw the contents.",
    "preview.e2eNoKey": "This link is missing its key",
    "preview.e2eNoKeyHint": "The part after # was lost. Ask the sender for the complete link.",
    "preview.e2eFailed": "Could not decrypt",
    "preview.e2eFailedHint": "Wrong key, or the file was modified or truncated.",
    "preview.e2eDownload": "Decrypt and download",
    // Collection landing page
    "collection.title": "Shared files",
    "collection.files": "files",
    "collection.downloadAll": "Download all as a zip",
    "collection.loading": "Loading the file list…",
    "collection.unavailable": "This collection is no longer available",
    "collection.unavailableHint":
      "Every file in it may have expired, hit its download limit, or been deleted.",
    // Hero CTA
    "hero.cta.primary": "Upload a file",
    "hero.cta.code": "$ curl --upload-file file.txt",
    "hero.cta.note": "No signup. No tracking. 52 MB Docker image.",
    // Features section
    "features.label": "Built with care",
    "features.title": "Every detail, considered.",
    "features.subtitle":
      "Self-hosted infrastructure shouldn't feel industrial. send.to is built to be pleasant to operate, pleasant to use, and pleasant to look at.",
    "feature.encryption.label": "Encryption",
    "feature.encryption.title": "End-to-end encryption that just works.",
    "feature.encryption.desc":
      "Add one header and your file is encrypted with OpenPGP AES-256 before touching disk. The server can't read it, and neither can your hosting provider.",
    "feature.expiry.label": "Auto-expiry",
    "feature.expiry.title": "Links that expire.",
    "feature.expiry.desc":
      "Per-file Max-Days and Max-Downloads. The file — and the link — quietly vanish.",
    "feature.expiry.caption": "default ttl",
    "feature.storage.label": "Storage",
    "feature.storage.title": "One flag, four backends.",
    "feature.storage.desc":
      "Local filesystem for dev, S3 for scale, Google Drive for personal, Storj for decentralized. Swap with a single CLI flag.",
    "feature.hardening.label": "Hardening",
    "feature.hardening.title": "Hardened by default.",
    "feature.hardening.desc":
      "Runs non-root on a scratch container. No shell, no package manager, no attack surface.",
    "feature.binary.label": "Developer experience",
    "feature.binary.title": "One binary, zero dependencies.",
    "feature.binary.desc":
      "CGO_ENABLED=0 build runs on scratch. Cold starts under 50 ms. The whole container is smaller than a Slack message.",
    // CLI section
    "cli.label": "For the command line",
    "cli.title": "Your terminal is the UI.",
    "cli.subtitle":
      "Works with curl, wget, HTTPie, PowerShell — anything that speaks HTTP. Headers do all the work.",
    // How it works (flow)
    "flow.label": "Three steps",
    "flow.title": "That's the whole API.",
    "flow.step1.title": "Upload",
    "flow.step1.desc":
      "PUT any file to any URL. The body is the file; headers configure expiry, download caps, and encryption.",
    "flow.step2.title": "Share",
    "flow.step2.desc":
      "Paste the token URL into chat, a CI log, an email — anywhere. No login required to download.",
    "flow.step3.title": "Forget",
    "flow.step3.desc":
      "Files auto-expire per your limits. Or use the one-shot DELETE URL returned on upload to remove it now.",
    // Final CTA
    "finalcta.label": "Ready to ship",
    "finalcta.title": "Stop emailing files.\nStart sending them.",
    "finalcta.subtitle":
      "One docker compose up and you have a production-hardened file sharing service on your own domain. It takes about thirty seconds.",
    "finalcta.cta.primary": "Get started",
    "finalcta.meta": "Open source · MIT · no tracking · 52 MB image",
    // Features (index page cards)
    "feature.encryption": "End-to-end encryption",
    "feature.expiration": "Auto-expiration",
    "feature.selfHosted": "Self-hosted",
    // About page category headings
    "about.featureTransfer": "File Transfer",
    "about.featureSecurity": "Security",
    "about.featurePreview": "Preview & Clients",
    "about.featureInfra": "Infrastructure",
    // Features (about page list)
    "feature.shareUrl": "Share files with a URL",
    "feature.maxDownloads": "Set maximum download count per file",
    "feature.multiUpload": "Upload multiple files at once",
    "feature.resumable": "Resumable downloads via HTTP Range requests",
    "feature.deleteToken": "Delete files with a unique deletion token",
    "feature.storage": "Multiple storage backends: local, S3, Google Drive, Storj",
    "feature.virusScan": "Virus scanning with ClamAV and VirusTotal",
    "feature.auth": "HTTP Basic Auth, htpasswd, and IP whitelist/blacklist",
    "feature.tls": "Automatic TLS via Let's Encrypt",
    "feature.rateLimit": "Per-IP rate limiting",
    "feature.preview": "File preview for images, video, audio, and Markdown",
    "feature.altClients": "Works with curl, wget, HTTPie, and PowerShell",
    "feature.gpgOpenssl": "Client-side encryption with GPG or OpenSSL",
    // Footer
    "footer.license": "MIT License",
    // 404
    "error.notFound": "File not found",
    "error.notFoundDesc":
      "The file may have expired, reached its download limit, or never existed.",
    "error.backHome": "Back to home",
    // About
    "about.title": "About",
    "about.subtitle":
      "send.to is an open-source file sharing service that makes it easy to share files from the command line.",
    "about.whatIs": "What is send.to?",
    "about.whatIsDesc":
      "send.to is a lightweight, self-hosted file sharing service built with Go. It compiles to a single static binary with zero external dependencies, making it trivial to deploy. Upload files via curl or the web interface, get a shareable link, and files auto-expire based on your configuration.",
    "about.selfHosting": "Self-hosting",
    "about.selfHostingDesc": "Deploy your own instance with Docker in seconds:",
    "about.techStack": "Tech Stack",
    "about.github": "GitHub Repository",
    "about.issues": "Report an Issue",
    // Tech Stack labels
    "tech.language": "Language",
    "tech.router": "Router",
    "tech.encryption": "Encryption",
    "tech.deployment": "Deployment",
    "tech.storage": "Storage",
    "tech.frontend": "Frontend",
    // API Docs
    "api.title": "API Documentation",
    "api.subtitle":
      "send.to provides a simple HTTP API for uploading, downloading, and managing files. All endpoints accept standard HTTP methods and can be used with curl or any HTTP client.",
    "api.upload": "Upload",
    "api.uploadDesc":
      "Upload a file using PUT or POST. The response contains the download URL. The X-Url-Delete response header contains the deletion URL.",
    "api.download": "Download",
    "api.downloadDesc":
      "Download or preview a file. Supports range requests for resumable downloads. The default GET endpoint auto-detects preview vs download based on Accept and Referer headers.",
    "api.delete": "Delete",
    "api.deleteDesc":
      "Delete a file using the deletion token returned in the X-Url-Delete response header during upload.",
    "api.archive": "Archive Download",
    "api.resumable": "Resumable Upload",
    "api.resumableDesc":
      "Upload a large file in chunks so an interrupted transfer continues instead of starting over. Open a session with the total size, send chunks with Content-Range, and ask with HEAD where to continue. A chunk is all or nothing: a body that arrives short is discarded and the offset stays where it was, so the offset you resume from is always one you chose. A wrong offset answers 409 with the correct one. The chunk that completes the file answers exactly like a plain PUT. Sessions expire after 24 hours, and X-Encrypt-Password is refused here — encrypt on the client instead.",
    "api.collections": "Collections",
    "api.collectionsDesc":
      "Group uploads that already exist behind one link. The collection link renders a page for browsers, returns JSON with Accept: application/json, and one share URL per line for everything else; adding an archive extension downloads all of it at once. A collection owns no bytes: deleting it leaves the files alone, a member that expires or runs out of downloads drops off the list, and a collection with nothing left answers 404. At most 100 files.",
    "api.owner": "Upload History",
    "api.ownerDesc":
      "Send X-Owner-Token with an upload and the server records it in that owner's list, which the same token can read back from any machine — the delete links included. There is still no account: the server stores only sha256 of the token, never the token itself, and an upload without the header stays anonymous. Holding the token is the whole of the authorisation, so treat it as a password. Entries drop out when their upload is deleted, expires or runs out of downloads; at most 200 are kept per token.",
    "code.resumableUpload": "resume a big upload",
    "code.createCollection": "one link for several files",
    "code.ownerList": "list what this token uploaded",
    "api.archiveDesc":
      "Download multiple files as a single archive (ZIP, TAR, or TAR.GZ). Separate file paths with commas.",
    "api.scan": "Virus Scan",
    "api.scanDesc":
      "Scan uploaded files with ClamAV or VirusTotal (requires server configuration).",
    "api.scanNote":
      "When --perform-clamav-prescan is enabled, all uploads are automatically scanned before being accepted. Infected files are rejected with HTTP 412.",
    "api.headers": "Request Headers",
    "api.headersDesc": "Optional headers to control upload and download behavior.",
    "api.uploadHeaders": "Upload Headers",
    "api.downloadHeaders": "Download Headers",
    "api.responseHeaders": "Response Headers",
    "api.responseHeadersDesc": "Headers returned by the server in upload and download responses.",
    "api.uploadResponseHeaders": "Upload Response",
    "api.downloadResponseHeaders": "Download Response",
    "api.caching": "Caching",
    "api.cachingDesc":
      "A stored file never changes, so every download carries an ETag and answers 304 Not Modified when you send it back in If-None-Match. Whether a browser or CDN may keep the response is up to the server operator, via CACHE_MAX_AGE, and it is off by default.",
    "api.cachingLimited":
      "Files with Max-Downloads are never cacheable — a cached copy is served without the origin counting it.",
    "api.cachingEncrypted":
      "Server-side encrypted files are never cacheable — they are decrypted per request against a password header.",
    "api.cachingDelete":
      "When caching is on, a deleted file stays reachable through the cache until its max-age runs out.",
    "api.languages": "Languages",
    "api.languagesDesc":
      "Error messages follow Accept-Language, in English, Chinese and Japanese. A request without the header gets English, and the response says which language it used in Content-Language.",
    "api.errors": "Error Codes",
    "api.errorsDesc": "Common HTTP status codes returned by the API.",
    // Code block titles
    "code.copy": "Copy",
    "code.uploadFile": "upload a file",
    "code.encryptUpload": "encrypt & upload",
    "code.setLimits": "set limits",
    "code.downloadFile": "download a file",
    "code.deleteFile": "delete a file",
    "code.dockerRun": "docker run",
    "code.dockerRunS3": "docker run with S3",
    "code.multiUpload": "upload multiple files",
    "code.combineArchive": "combine as archive",
    "code.gpgEncrypt": "gpg encrypt & upload",
    "code.gpgDecrypt": "download & decrypt",
    "code.clamavScan": "clamav scan",
    "code.virustotalScan": "virustotal scan",
    "code.mysqlBackup": "mysql backup",
    "code.transferEmail": "transfer & email",
    "code.keybaseEncrypt": "keybase encrypt",
    "code.keybaseDecrypt": "keybase decrypt",
    "code.wget": "wget",
    "code.httpie": "httpie",
    "code.powershell": "powershell",
    "code.opensslEncrypt": "openssl encrypt",
    "code.opensslDecrypt": "openssl decrypt",
    "code.dockerAdvanced": "with encryption & limits",
    "code.pipeLogs": "pipe logs",
    // Use cases
    "usecase.title": "Use Cases",
    "usecase.subtitle":
      "Real-world examples of how to use send.to with curl, GPG, OpenSSL, and other tools.",
    "usecase.multiUpload": "Upload Multiple Files",
    "usecase.multiUploadDesc":
      "Upload multiple files at once using multipart form data. Combine downloads as ZIP or TAR archives.",
    "usecase.gpgEncrypt": "Encrypt with GPG",
    "usecase.gpgEncryptDesc":
      "Encrypt files with GPG before transfer. Pipe through gpg for maximum security with your own keys.",
    "usecase.malwareScan": "Scan for Malware",
    "usecase.malwareScanDesc":
      "Scan uploaded files for viruses using ClamAV or upload to VirusTotal for comprehensive analysis.",
    "usecase.dbBackup": "Database Backup",
    "usecase.dbBackupDesc":
      "Backup MySQL databases, compress with gzip, encrypt with GPG, and transfer — all in one pipeline.",
    "usecase.emailLink": "Email with Transfer Link",
    "usecase.emailLinkDesc":
      "Upload a file and pipe the download link directly to an email using the shell transfer function.",
    "usecase.keybase": "Use with Keybase",
    "usecase.keybaseDesc":
      "Encrypt files for specific recipients using Keybase keys. Import, encrypt, upload, and decrypt seamlessly.",
    "usecase.altClients": "Alternative Clients",
    "usecase.altClientsDesc":
      "Works with wget, HTTPie, PowerShell, and third-party CLI tools. Use your preferred HTTP client.",
    "usecase.openssl": "Encrypt with OpenSSL",
    "usecase.opensslDesc":
      "Use OpenSSL AES-256-CBC encryption for files before transfer. No GPG required.",
    "usecase.pipeLogs": "Pipe Logs & Output",
    "usecase.pipeLogsDesc":
      "Pipe any command output directly to send.to. Share syslog entries, grep results, or any stream.",
    // Code block comments
    "code.comment.setLimits": "# max 5 downloads, expires in 7 days",
    "code.comment.multiUpload": "# upload multiple files at once",
    "code.comment.combineArchive": "# download as zip or tar.gz",
    "code.comment.gpgEncrypt": "# encrypt with gpg and upload",
    "code.comment.gpgDecrypt": "# download and decrypt",
    "code.comment.clamavScan": "# scan with ClamAV",
    "code.comment.virustotalScan": "# upload to VirusTotal",
    "code.comment.mysqlBackup": "# backup, encrypt and transfer",
    "code.comment.transferEmail": "# transfer and send email with link",
    "code.comment.keybaseEncrypt": "# encrypt for recipient and upload",
    "code.comment.keybaseDecrypt": "# download and decrypt",
    "code.comment.wget": "# upload with wget",
    "code.comment.httpie": "# upload with HTTPie",
    "code.comment.powershell": "# upload with PowerShell",
    "code.comment.opensslEncrypt": "# encrypt with openssl and upload",
    "code.comment.opensslDecrypt": "# download and decrypt",
    "code.comment.pipeLogs": "# grep syslog and transfer",
    // Q&A
  },
  zh: {
    // Navbar
    "nav.home": "首页",
    "nav.apiDocs": "API 文档",
    "nav.useCases": "使用场景",
    "nav.about": "关于",
    // Hero
    "hero.title": "send.to",
    "hero.subtitle": "从命令行轻松快速地分享文件。上传文件，即刻获取分享链接。",
    // Upload
    "upload.dropzone": "拖拽文件到此处或",
    "upload.hint": "文件在传输中加密，并自动过期",
    "upload.uploading": "上传中...",
    "upload.complete": "上传完成",
    "upload.another": "上传更多文件",
    "upload.shareLink": "分享链接",
    "upload.downloadCmd": "curl 下载命令",
    "upload.deleteCmd": "删除文件",
    "upload.backendError":
      "后端未运行。请启动：go run main.go --provider local --basedir /tmp/send.to --listener :18080",
    "upload.eta": "预计剩余",
    "upload.cancel": "取消",
    "upload.close": "关闭",
    "upload.copyLink": "复制链接",
    "upload.copyCurl": "复制 curl 命令",
    "upload.copyDelete": "复制删除命令",
    "upload.copyFailed": "复制失败 — 请手动复制",
    "upload.tooLarge": "文件过大",
    "upload.dropzoneMulti": "拖拽文件到此处或",
    "upload.hintMulti": "支持多文件、剪贴板粘贴，或点击选择",
    "upload.foldersUnsupported": "不支持文件夹 — 请先打包成压缩文件",
    "upload.options": "上传选项",
    "upload.optionsHide": "收起选项",
    "upload.expiryDays": "有效期（天）",
    "upload.expiryNever": "永不过期",
    "upload.maxDownloads": "下载次数限制",
    "upload.maxDownloadsUnlimited": "不限制",
    "upload.password": "使用密码加密",
    "upload.passwordPlaceholder": "留空则不加密",
    "upload.passwordHint": "下载方需要提供该密码才能解密。",
    "upload.passwordShow": "显示密码",
    "upload.passwordHide": "隐藏密码",
    "upload.queued": "等待中",
    "upload.failed": "失败",
    "upload.retry": "重试",
    "upload.remove": "移除",
    "upload.cancelAll": "全部取消",
    "upload.results": "已上传",
    "upload.expires": "过期时间",
    "upload.downloadsLeft": "次下载剩余",
    "upload.qrShow": "显示二维码",
    "upload.qrHide": "隐藏二维码",
    "upload.qrAlt": "分享链接二维码",
    "upload.history": "最近上传",
    "upload.historyEmpty": "此浏览器还没有上传记录。",
    "upload.historyClear": "清空记录",
    "upload.historyNote": "仅保存在本浏览器 — 清除站点数据会丢失删除链接。",
    "upload.deleteNow": "删除",
    "upload.deleted": "已删除",
    "upload.deleteFailed": "删除失败",
    "upload.copied": "已复制",
    "upload.encryptedBadge": "已加密",
    "upload.decryptCmd": "下载并解密",
    "upload.copyDecrypt": "复制解密命令",
    "preview.download": "下载文件",
    "preview.downloadsLeft": "剩余下载次数",
    "preview.daysLeft": "剩余天数",
    "preview.reveal": "显示预览",
    "preview.revealNote": "该文件有下载次数限制 —— 预览会消耗一次。",
    "preview.badLink": "这个链接格式不对",
    "preview.unavailable": "文件已不可用",
    "preview.unavailableHint": "可能已过期、下载次数用尽，或已被删除。",
    "preview.maybeEncrypted":
      '该文件可能已加密。若是，请用：curl -H "X-Decrypt-Password: <密码>" <链接> 下载。',
    "upload.e2e": "端到端加密",
    "upload.e2eHint":
      "在你的浏览器里加密。密钥放在链接 # 之后，永远不会发给服务器 —— 服务器存的是它自己解不开的文件。",
    "upload.e2eBadge": "端到端加密",
    "upload.e2eKeyWarning": "请保留完整链接。缺了 # 之后那段，文件将无法恢复，任何人都无法重置。",
    "upload.e2eEncrypting": "加密中…",
    "preview.e2eTitle": "端到端加密",
    "preview.e2eDecrypting": "正在你的浏览器中解密…",
    "preview.e2eReady": "已在你的浏览器中解密 —— 服务器从未接触过内容。",
    "preview.e2eNoKey": "链接缺少密钥",
    "preview.e2eNoKeyHint": "# 之后那段丢失了。请向发送方索取完整链接。",
    "preview.e2eFailed": "无法解密",
    "preview.e2eFailedHint": "密钥错误，或文件已被修改 / 截断。",
    "preview.e2eDownload": "解密并下载",
    // 集合落地页
    "collection.title": "共享文件",
    "collection.files": "个文件",
    "collection.downloadAll": "打包下载全部",
    "collection.loading": "正在读取文件列表…",
    "collection.unavailable": "该集合已不可用",
    "collection.unavailableHint": "其中的文件可能都已过期、下载次数用尽，或已被删除。",
    // Hero CTA
    "hero.cta.primary": "上传文件",
    "hero.cta.code": "$ curl --upload-file 文件.txt",
    "hero.cta.note": "无需注册，无任何追踪，Docker 镜像仅 52 MB。",
    // Features section
    "features.label": "用心打造",
    "features.title": "每个细节都经过深思。",
    "features.subtitle":
      "自托管的基础设施不该让人觉得冰冷。send.to 被做成运维起来、使用起来、看起来都让人舒服的样子。",
    "feature.encryption.label": "加密",
    "feature.encryption.title": "端到端加密，开箱即用。",
    "feature.encryption.desc":
      "加一个请求头，文件就会在落盘前用 OpenPGP AES-256 加密。服务端读不到，托管商也读不到。",
    "feature.expiry.label": "自动过期",
    "feature.expiry.title": "会过期的分享链接。",
    "feature.expiry.desc":
      "每个文件独立设置 Max-Days 和 Max-Downloads。到期后文件和链接一起悄悄消失。",
    "feature.expiry.caption": "默认有效期",
    "feature.storage.label": "存储",
    "feature.storage.title": "一个参数,四种后端。",
    "feature.storage.desc":
      "开发用本地文件系统,生产用 S3,个人用 Google Drive,去中心化用 Storj。切换只需一个 CLI 参数。",
    "feature.hardening.label": "安全加固",
    "feature.hardening.title": "默认就是生产级安全。",
    "feature.hardening.desc":
      "以非 root 身份运行在 scratch 容器里。没有 shell,没有包管理器,没有攻击面。",
    "feature.binary.label": "开发体验",
    "feature.binary.title": "单一二进制,零依赖。",
    "feature.binary.desc":
      "CGO_ENABLED=0 编译,跑在 scratch 上。冷启动 50ms 以内。整个容器比一条 Slack 消息还小。",
    // CLI section
    "cli.label": "面向命令行",
    "cli.title": "终端就是你的 UI。",
    "cli.subtitle":
      "兼容 curl、wget、HTTPie、PowerShell —— 任何支持 HTTP 的客户端。所有配置通过请求头完成。",
    // How it works (flow)
    "flow.label": "三步走",
    "flow.title": "整个 API 就这么简单。",
    "flow.step1.title": "上传",
    "flow.step1.desc":
      "PUT 任何文件到任何 URL。请求体就是文件本身,请求头控制过期、下载次数和加密。",
    "flow.step2.title": "分享",
    "flow.step2.desc": "把带 token 的 URL 粘贴到聊天、CI 日志或邮件里 —— 哪都行。下载不需要登录。",
    "flow.step3.title": "忘掉它",
    "flow.step3.desc":
      "文件按你设置的规则自动过期。也可以用上传时返回的一次性 DELETE URL 立即删除。",
    // Final CTA
    "finalcta.label": "准备好上线",
    "finalcta.title": "别再用邮件发文件了。\n开始发链接吧。",
    "finalcta.subtitle":
      "一条 docker compose up,你就在自己的域名上拥有了一个生产级硬化的文件分享服务。大约需要三十秒。",
    "finalcta.cta.primary": "立即开始",
    "finalcta.meta": "开源 · MIT · 无追踪 · 镜像 52 MB",
    // Features (index page cards)
    "feature.encryption": "端到端加密",
    "feature.expiration": "自动过期",
    "feature.selfHosted": "自托管",
    // About page category headings
    "about.featureTransfer": "文件传输",
    "about.featureSecurity": "安全",
    "about.featurePreview": "预览与客户端",
    "about.featureInfra": "基础设施",
    // Features (about page list)
    "feature.shareUrl": "通过 URL 分享文件",
    "feature.maxDownloads": "可设置每个文件的最大下载次数",
    "feature.multiUpload": "一次上传多个文件",
    "feature.resumable": "通过 HTTP Range 请求实现断点续传",
    "feature.deleteToken": "使用唯一删除令牌删除文件",
    "feature.storage": "多种存储后端：本地、S3、Google Drive、Storj",
    "feature.virusScan": "病毒扫描：支持 ClamAV 和 VirusTotal",
    "feature.auth": "HTTP Basic Auth、htpasswd 和 IP 黑白名单",
    "feature.tls": "通过 Let's Encrypt 自动配置 TLS",
    "feature.rateLimit": "按 IP 限流",
    "feature.preview": "文件预览：支持图片、视频、音频和 Markdown",
    "feature.altClients": "支持 curl、wget、HTTPie 和 PowerShell",
    "feature.gpgOpenssl": "客户端使用 GPG 或 OpenSSL 加密",
    // Footer
    "footer.license": "MIT 许可证",
    // 404
    "error.notFound": "文件未找到",
    "error.notFoundDesc": "文件可能已过期、下载次数已耗尽，或从未存在。",
    "error.backHome": "返回首页",
    // About
    "about.title": "关于",
    "about.subtitle": "send.to 是一个开源文件分享服务，让你从命令行轻松分享文件。",
    "about.whatIs": "什么是 send.to？",
    "about.whatIsDesc":
      "send.to 是一个用 Go 构建的轻量级自托管文件分享服务。它编译为单一静态二进制文件，零外部依赖，部署极其简单。通过 curl 或 Web 界面上传文件，获取分享链接，文件根据配置自动过期。",
    "about.selfHosting": "自托管部署",
    "about.selfHostingDesc": "使用 Docker 秒级部署你自己的实例：",
    "about.techStack": "技术栈",
    "about.github": "GitHub 仓库",
    "about.issues": "报告问题",
    // Tech Stack labels
    "tech.language": "编程语言",
    "tech.router": "路由",
    "tech.encryption": "加密",
    "tech.deployment": "部署",
    "tech.storage": "存储",
    "tech.frontend": "前端",
    // API Docs
    "api.title": "API 文档",
    "api.subtitle":
      "send.to 提供简单的 HTTP API 用于上传、下载和管理文件。所有端点接受标准 HTTP 方法，可使用 curl 或任何 HTTP 客户端。",
    "api.upload": "上传",
    "api.uploadDesc":
      "使用 PUT 或 POST 上传文件。响应包含下载 URL。X-Url-Delete 响应头包含删除 URL。",
    "api.download": "下载",
    "api.downloadDesc":
      "下载或预览文件。支持 Range 请求实现断点续传。默认 GET 端点根据 Accept 和 Referer 头自动检测预览或下载。",
    "api.delete": "删除",
    "api.deleteDesc": "使用上传时 X-Url-Delete 响应头中返回的删除 token 删除文件。",
    "api.archive": "归档下载",
    "api.resumable": "分片续传上传",
    "api.resumableDesc":
      "把大文件切成分片上传，断掉之后接着传而不是从头再来。先用总大小开一个会话，然后带 Content-Range 逐片发送，用 HEAD 询问从哪继续。分片是原子的：传到一半的分片会被丢弃、偏移量不变，所以你续传的位置永远是自己选的位置；偏移量对不上会返回 409 并给出正确值。补齐最后一个分片时的响应与普通 PUT 完全一致。会话 24 小时过期。这里不接受 X-Encrypt-Password —— 请改用客户端加密。",
    "api.collections": "集合链接",
    "api.collectionsDesc":
      "把已经存在的上传收进一条链接。浏览器打开是落地页，带 Accept: application/json 返回 JSON，其他客户端拿到每行一个分享链接；加上打包后缀就能一次下载全部。集合本身不持有任何字节：删除集合不会动里面的文件，成员过期或下载次数用尽会自动掉队，一个都不剩时返回 404。每个集合最多 100 个文件。",
    "api.owner": "上传历史",
    "api.ownerDesc":
      "上传时带上 X-Owner-Token，服务端就会把它记进该身份的列表；之后在任何机器上用同一个 token 都能读回来，包括删除链接。它仍然不是账号：服务端只存 token 的 sha256，永远不存 token 本身，不带这个头的上传依然匿名。持有 token 就是全部授权，请当成密码对待。文件被删除、过期或下载次数用尽时，对应条目会自动消失；每个 token 最多保留 200 条。",
    "code.resumableUpload": "续传一个大文件",
    "code.createCollection": "一条链接装多个文件",
    "code.ownerList": "列出这个 token 上传过什么",
    "api.archiveDesc": "将多个文件打包为单个归档下载（ZIP、TAR 或 TAR.GZ）。用逗号分隔文件路径。",
    "api.scan": "病毒扫描",
    "api.scanDesc": "使用 ClamAV 或 VirusTotal 扫描上传的文件（需要服务器配置）。",
    "api.scanNote":
      "启用 --perform-clamav-prescan 后，所有上传文件在接受前自动扫描。感染文件将被拒绝，返回 HTTP 412。",
    "api.headers": "请求头",
    "api.headersDesc": "可选的请求头，用于控制上传和下载行为。",
    "api.uploadHeaders": "上传请求头",
    "api.downloadHeaders": "下载请求头",
    "api.responseHeaders": "响应头",
    "api.responseHeadersDesc": "服务器在上传和下载响应中返回的头信息。",
    "api.uploadResponseHeaders": "上传响应",
    "api.downloadResponseHeaders": "下载响应",
    "api.caching": "缓存",
    "api.cachingDesc":
      "文件存好后不再改变，因此每个下载响应都带 ETag；用 If-None-Match 带回来时返回 304 Not Modified。浏览器或 CDN 是否可以保存响应由服务端通过 CACHE_MAX_AGE 决定，默认关闭。",
    "api.cachingLimited": "带 Max-Downloads 的文件永远不可缓存 —— 缓存命中时源站不会计入这次下载。",
    "api.cachingEncrypted": "服务端加密的文件永远不可缓存 —— 它们要按请求携带的密码头逐次解密。",
    "api.cachingDelete": "开启缓存后，已删除的文件在 max-age 到期前仍可能从缓存中取到。",
    "api.languages": "语言",
    "api.languagesDesc":
      "错误信息按 Accept-Language 返回中文、英文或日文。不带该请求头时返回英文，响应用 Content-Language 说明实际使用的语言。",
    "api.errors": "错误码",
    "api.errorsDesc": "API 返回的常见 HTTP 状态码。",
    // Code block titles
    "code.copy": "复制",
    "code.uploadFile": "上传文件",
    "code.encryptUpload": "加密上传",
    "code.setLimits": "设置限制",
    "code.downloadFile": "下载文件",
    "code.deleteFile": "删除文件",
    "code.dockerRun": "Docker 运行",
    "code.dockerRunS3": "Docker 运行（S3）",
    "code.multiUpload": "上传多个文件",
    "code.combineArchive": "合并为归档",
    "code.gpgEncrypt": "GPG 加密上传",
    "code.gpgDecrypt": "下载并解密",
    "code.clamavScan": "ClamAV 扫描",
    "code.virustotalScan": "VirusTotal 扫描",
    "code.mysqlBackup": "MySQL 备份",
    "code.transferEmail": "传输并发送邮件",
    "code.keybaseEncrypt": "Keybase 加密",
    "code.keybaseDecrypt": "Keybase 解密",
    "code.wget": "wget",
    "code.httpie": "httpie",
    "code.powershell": "powershell",
    "code.opensslEncrypt": "OpenSSL 加密",
    "code.opensslDecrypt": "OpenSSL 解密",
    "code.dockerAdvanced": "加密与限制配置",
    "code.pipeLogs": "管道日志",
    // Use cases
    "usecase.title": "使用场景",
    "usecase.subtitle": "使用 curl、GPG、OpenSSL 和其他工具操作 send.to 的实际示例。",
    "usecase.multiUpload": "一次上传多个文件",
    "usecase.multiUploadDesc":
      "使用 multipart 表单数据一次上传多个文件。合并下载为 ZIP 或 TAR 归档。",
    "usecase.gpgEncrypt": "GPG 加密传输",
    "usecase.gpgEncryptDesc":
      "传输前使用 GPG 加密文件。通过 gpg 管道实现最高安全性，使用你自己的密钥。",
    "usecase.malwareScan": "恶意软件扫描",
    "usecase.malwareScanDesc":
      "使用 ClamAV 扫描上传文件中的病毒，或上传到 VirusTotal 进行全面分析。",
    "usecase.dbBackup": "数据库备份",
    "usecase.dbBackupDesc":
      "备份 MySQL 数据库，用 gzip 压缩，用 GPG 加密，然后传输——一条管道搞定。",
    "usecase.emailLink": "邮件发送传输链接",
    "usecase.emailLinkDesc": "上传文件后，使用 shell transfer 函数将下载链接直接通过邮件发送。",
    "usecase.keybase": "配合 Keybase 使用",
    "usecase.keybaseDesc": "使用 Keybase 密钥为特定接收者加密文件。无缝导入、加密、上传和解密。",
    "usecase.altClients": "多种客户端",
    "usecase.altClientsDesc":
      "支持 wget、HTTPie、PowerShell 和第三方 CLI 工具。使用你喜欢的 HTTP 客户端。",
    "usecase.openssl": "OpenSSL 加密传输",
    "usecase.opensslDesc": "传输前使用 OpenSSL AES-256-CBC 加密文件。无需 GPG。",
    "usecase.pipeLogs": "管道日志与输出",
    "usecase.pipeLogsDesc":
      "将任何命令输出直接管道到 send.to。分享 syslog 条目、grep 结果或任何流。",
    // Code block comments
    "code.comment.setLimits": "# 最多 5 次下载，7 天后过期",
    "code.comment.multiUpload": "# 一次上传多个文件",
    "code.comment.combineArchive": "# 下载为 zip 或 tar.gz",
    "code.comment.gpgEncrypt": "# 使用 gpg 加密并上传",
    "code.comment.gpgDecrypt": "# 下载并解密",
    "code.comment.clamavScan": "# 使用 ClamAV 扫描",
    "code.comment.virustotalScan": "# 上传到 VirusTotal",
    "code.comment.mysqlBackup": "# 备份、加密并传输",
    "code.comment.transferEmail": "# 传输并通过邮件发送链接",
    "code.comment.keybaseEncrypt": "# 为接收者加密并上传",
    "code.comment.keybaseDecrypt": "# 下载并解密",
    "code.comment.wget": "# 使用 wget 上传",
    "code.comment.httpie": "# 使用 HTTPie 上传",
    "code.comment.powershell": "# 使用 PowerShell 上传",
    "code.comment.opensslEncrypt": "# 使用 openssl 加密并上传",
    "code.comment.opensslDecrypt": "# 下载并解密",
    "code.comment.pipeLogs": "# 过滤 syslog 并传输",
    // Q&A
  },
  ja: {
    // Navbar
    "nav.home": "ホーム",
    "nav.apiDocs": "APIドキュメント",
    "nav.useCases": "ユースケース",
    "nav.about": "概要",
    // Hero
    "hero.title": "send.to",
    "hero.subtitle":
      "コマンドラインから簡単・高速にファイルを共有。アップロードして、共有リンクを即座に取得。",
    // Upload
    "upload.dropzone": "ファイルをここにドロップまたは",
    "upload.hint": "ファイルは転送中に暗号化され、自動的に期限切れになります",
    "upload.uploading": "アップロード中...",
    "upload.complete": "アップロード完了",
    "upload.another": "別のファイルをアップロード",
    "upload.shareLink": "共有リンク",
    "upload.downloadCmd": "curlでダウンロード",
    "upload.deleteCmd": "ファイルを削除",
    "upload.backendError":
      "バックエンドが起動していません。起動コマンド: go run main.go --provider local --basedir /tmp/send.to --listener :18080",
    "upload.eta": "残り時間",
    "upload.cancel": "キャンセル",
    "upload.close": "閉じる",
    "upload.copyLink": "リンクをコピー",
    "upload.copyCurl": "curl コマンドをコピー",
    "upload.copyDelete": "削除コマンドをコピー",
    "upload.copyFailed": "コピー失敗 — 手動でコピーしてください",
    "upload.tooLarge": "ファイルサイズが大きすぎます",
    "upload.dropzoneMulti": "ファイルをここにドロップまたは",
    "upload.hintMulti": "複数ファイル、クリップボードから貼り付け、クリックして選択",
    "upload.foldersUnsupported": "フォルダには対応していません — 先に圧縮してください",
    "upload.options": "オプション",
    "upload.optionsHide": "オプションを隠す",
    "upload.expiryDays": "有効期限（日）",
    "upload.expiryNever": "期限なし",
    "upload.maxDownloads": "ダウンロード回数制限",
    "upload.maxDownloadsUnlimited": "無制限",
    "upload.password": "パスワードで暗号化",
    "upload.passwordPlaceholder": "空欄なら暗号化しません",
    "upload.passwordHint": "受信者はこのパスワードを入力する必要があります。",
    "upload.passwordShow": "パスワードを表示",
    "upload.passwordHide": "パスワードを隠す",
    "upload.queued": "待機中",
    "upload.failed": "失敗",
    "upload.retry": "再試行",
    "upload.remove": "削除",
    "upload.cancelAll": "すべてキャンセル",
    "upload.results": "アップロード済み",
    "upload.expires": "有効期限",
    "upload.downloadsLeft": "回ダウンロード可能",
    "upload.qrShow": "QRコードを表示",
    "upload.qrHide": "QRコードを隠す",
    "upload.qrAlt": "共有リンクのQRコード",
    "upload.history": "最近のアップロード",
    "upload.historyEmpty": "このブラウザからのアップロードはまだありません。",
    "upload.historyClear": "履歴を消去",
    "upload.historyNote":
      "このブラウザにのみ保存されます — サイトデータを消すと削除リンクが失われます。",
    "upload.deleteNow": "削除",
    "upload.deleted": "削除しました",
    "upload.deleteFailed": "削除に失敗しました",
    "upload.copied": "コピーしました",
    "upload.encryptedBadge": "暗号化済み",
    "upload.decryptCmd": "ダウンロードして復号",
    "upload.copyDecrypt": "復号コマンドをコピー",
    "preview.download": "ファイルをダウンロード",
    "preview.downloadsLeft": "残りダウンロード回数",
    "preview.daysLeft": "残り日数",
    "preview.reveal": "プレビューを表示",
    "preview.revealNote": "このファイルには回数制限があります — プレビューで1回消費します。",
    "preview.badLink": "リンクの形式が正しくありません",
    "preview.unavailable": "このファイルは利用できません",
    "preview.unavailableHint": "期限切れ、回数上限、または削除された可能性があります。",
    "preview.maybeEncrypted":
      'このファイルは暗号化されている可能性があります。その場合は curl -H "X-Decrypt-Password: <パスワード>" <URL> でダウンロードしてください。',
    "upload.e2e": "エンドツーエンド暗号化",
    "upload.e2eHint":
      "このブラウザ内で暗号化します。鍵はリンクの # 以降に入り、サーバーには送信されません — サーバーは自分では読めないファイルを保存します。",
    "upload.e2eBadge": "エンドツーエンド暗号化",
    "upload.e2eKeyWarning":
      "リンク全体を保管してください。# 以降がないとファイルは復元できず、誰にもリセットできません。",
    "upload.e2eEncrypting": "暗号化中…",
    "preview.e2eTitle": "エンドツーエンド暗号化",
    "preview.e2eDecrypting": "ブラウザで復号しています…",
    "preview.e2eReady": "ブラウザで復号しました — サーバーは内容を一度も見ていません。",
    "preview.e2eNoKey": "リンクに鍵がありません",
    "preview.e2eNoKeyHint": "# 以降が失われています。送信者に完全なリンクを依頼してください。",
    "preview.e2eFailed": "復号できませんでした",
    "preview.e2eFailedHint": "鍵が違うか、ファイルが改変または切り詰められています。",
    "preview.e2eDownload": "復号してダウンロード",
    // コレクションのランディングページ
    "collection.title": "共有ファイル",
    "collection.files": "個のファイル",
    "collection.downloadAll": "まとめて zip でダウンロード",
    "collection.loading": "ファイル一覧を読み込み中…",
    "collection.unavailable": "このコレクションは利用できません",
    "collection.unavailableHint":
      "含まれるファイルがすべて期限切れ、ダウンロード上限、または削除された可能性があります。",
    // Hero CTA
    "hero.cta.primary": "ファイルをアップロード",
    "hero.cta.code": "$ curl --upload-file file.txt",
    "hero.cta.note": "登録不要・トラッキングなし・Dockerイメージ52MB。",
    // Features section
    "features.label": "丁寧に作られた",
    "features.title": "すべてのディテールを大切に。",
    "features.subtitle":
      "セルフホスト基盤は無機質である必要はありません。send.to は運用も、使用も、眺めていて心地よいように作られています。",
    "feature.encryption.label": "暗号化",
    "feature.encryption.title": "実用的な E2E 暗号化。",
    "feature.encryption.desc":
      "ヘッダーを 1 つ追加するだけで、ファイルはディスクに書き込まれる前に OpenPGP AES-256 で暗号化されます。サーバーもホスティング業者も読めません。",
    "feature.expiry.label": "自動期限切れ",
    "feature.expiry.title": "期限のあるリンク。",
    "feature.expiry.desc":
      "ファイルごとに Max-Days と Max-Downloads を設定。ファイルもリンクも静かに消えていきます。",
    "feature.expiry.caption": "デフォルト有効期限",
    "feature.storage.label": "ストレージ",
    "feature.storage.title": "1 つのフラグで 4 種のバックエンド。",
    "feature.storage.desc":
      "開発はローカル FS、スケールは S3、個人用は Google Drive、分散型は Storj。CLI フラグ 1 つで切り替え。",
    "feature.hardening.label": "セキュリティ強化",
    "feature.hardening.title": "デフォルトで堅牢。",
    "feature.hardening.desc":
      "scratch コンテナ上を非 root で実行。シェルもパッケージマネージャもなく、攻撃面はゼロ。",
    "feature.binary.label": "開発者体験",
    "feature.binary.title": "1 つのバイナリ、依存ゼロ。",
    "feature.binary.desc":
      "CGO_ENABLED=0 ビルドで scratch 上で動作。コールドスタートは 50ms 未満。コンテナ全体が Slack メッセージより小さい。",
    // CLI section
    "cli.label": "コマンドライン向け",
    "cli.title": "ターミナルが UI です。",
    "cli.subtitle":
      "curl、wget、HTTPie、PowerShell 何でも OK。HTTP が話せるクライアントなら動きます。設定はすべてヘッダーで。",
    // How it works (flow)
    "flow.label": "3 ステップ",
    "flow.title": "API はこれだけ。",
    "flow.step1.title": "アップロード",
    "flow.step1.desc":
      "任意のファイルを任意の URL に PUT。本文がファイル、ヘッダーで有効期限・ダウンロード上限・暗号化を指定。",
    "flow.step2.title": "共有",
    "flow.step2.desc":
      "トークン付きの URL をチャット、CI ログ、メールに貼るだけ。ダウンロードにログインは不要。",
    "flow.step3.title": "忘れる",
    "flow.step3.desc":
      "ファイルは設定に従って自動で消えます。アップロード時に返される DELETE URL で即削除も可能。",
    // Final CTA
    "finalcta.label": "デプロイ準備完了",
    "finalcta.title": "ファイルをメールで送るのをやめよう。\nリンクを送ろう。",
    "finalcta.subtitle":
      "docker compose up 一発で、自分のドメインに本番運用レベルのファイル共有サービスが立ち上がります。約 30 秒。",
    "finalcta.cta.primary": "はじめる",
    "finalcta.meta": "オープンソース · MIT · トラッキングなし · イメージ 52 MB",
    // Features (index page cards)
    "feature.encryption": "エンドツーエンド暗号化",
    "feature.expiration": "自動期限切れ",
    "feature.selfHosted": "セルフホスト",
    // About page category headings
    "about.featureTransfer": "ファイル転送",
    "about.featureSecurity": "セキュリティ",
    "about.featurePreview": "プレビュー＆クライアント",
    "about.featureInfra": "インフラストラクチャ",
    // Features (about page list)
    "feature.shareUrl": "URLでファイルを共有",
    "feature.maxDownloads": "ファイルごとの最大ダウンロード回数設定",
    "feature.multiUpload": "複数ファイルの一括アップロード",
    "feature.resumable": "HTTP Range リクエストによるレジューム可能なダウンロード",
    "feature.deleteToken": "一意の削除トークンでファイルを削除",
    "feature.storage": "複数のストレージバックエンド：ローカル、S3、Google Drive、Storj",
    "feature.virusScan": "ClamAV と VirusTotal によるウイルススキャン",
    "feature.auth": "HTTP Basic Auth、htpasswd、IP ホワイトリスト/ブラックリスト",
    "feature.tls": "Let's Encrypt による自動 TLS",
    "feature.rateLimit": "IP ごとのレート制限",
    "feature.preview": "画像、動画、音声、Markdown のファイルプレビュー",
    "feature.altClients": "curl、wget、HTTPie、PowerShell に対応",
    "feature.gpgOpenssl": "クライアント側で GPG または OpenSSL による暗号化",
    // Footer
    "footer.license": "MITライセンス",
    // 404
    "error.notFound": "ファイルが見つかりません",
    "error.notFoundDesc":
      "ファイルは期限切れ、ダウンロード上限到達、または存在しない可能性があります。",
    "error.backHome": "ホームに戻る",
    // About
    "about.title": "概要",
    "about.subtitle":
      "send.to はコマンドラインから簡単にファイルを共有できるオープンソースのファイル共有サービスです。",
    "about.whatIs": "send.to とは？",
    "about.whatIsDesc":
      "send.to は Go で構築された軽量なセルフホスト型ファイル共有サービスです。外部依存ゼロの単一静的バイナリにコンパイルされ、デプロイが非常に簡単です。curl または Web インターフェースでファイルをアップロードし、共有リンクを取得。ファイルは設定に基づいて自動的に期限切れになります。",
    "about.selfHosting": "セルフホスティング",
    "about.selfHostingDesc": "Dockerで数秒でデプロイ：",
    "about.techStack": "技術スタック",
    "about.github": "GitHubリポジトリ",
    "about.issues": "問題を報告",
    // Tech Stack labels
    "tech.language": "言語",
    "tech.router": "ルーター",
    "tech.encryption": "暗号化",
    "tech.deployment": "デプロイ",
    "tech.storage": "ストレージ",
    "tech.frontend": "フロントエンド",
    // API Docs
    "api.title": "APIドキュメント",
    "api.subtitle":
      "send.to はファイルのアップロード、ダウンロード、管理のためのシンプルな HTTP API を提供します。すべてのエンドポイントは標準 HTTP メソッドを受け付け、curl や任意の HTTP クライアントで使用できます。",
    "api.upload": "アップロード",
    "api.uploadDesc":
      "PUT または POST でファイルをアップロード。レスポンスにダウンロード URL が含まれます。X-Url-Delete レスポンスヘッダーに削除 URL が含まれます。",
    "api.download": "ダウンロード",
    "api.downloadDesc":
      "ファイルのダウンロードまたはプレビュー。レジューム可能なダウンロードのための Range リクエストをサポート。デフォルトの GET エンドポイントは Accept と Referer ヘッダーに基づいてプレビューとダウンロードを自動検出します。",
    "api.delete": "削除",
    "api.deleteDesc":
      "アップロード時の X-Url-Delete レスポンスヘッダーで返された削除トークンを使用してファイルを削除。",
    "api.archive": "アーカイブダウンロード",
    "api.resumable": "レジューム可能なアップロード",
    "api.resumableDesc":
      "大きなファイルをチャンクに分けてアップロードし、中断しても最初からやり直さずに続きから再開します。合計サイズを指定してセッションを開き、Content-Range を付けてチャンクを送り、HEAD でどこから続けるかを尋ねます。チャンクは全か無かで、途中で切れた本文は破棄されオフセットは動きません。オフセットが合わない場合は 409 と正しい値を返します。最後のチャンクの応答は通常の PUT と同じです。セッションは 24 時間で失効し、X-Encrypt-Password はここでは拒否されます — クライアント側で暗号化してください。",
    "api.collections": "コレクション",
    "api.collectionsDesc":
      "既存のアップロードを 1 つのリンクにまとめます。ブラウザではページを表示し、Accept: application/json では JSON を、それ以外では 1 行 1 URL を返します。拡張子を付ければまとめてダウンロードできます。コレクション自体はバイトを持ちません。削除してもファイルは残り、期限切れやダウンロード上限に達したメンバーは一覧から外れ、何も残っていないコレクションは 404 を返します。最大 100 ファイル。",
    "api.owner": "アップロード履歴",
    "api.ownerDesc":
      "アップロード時に X-Owner-Token を送ると、サーバーがその所有者の一覧に記録し、同じトークンがあればどのマシンからでも削除リンクごと読み戻せます。アカウントは不要のままです。サーバーはトークンの sha256 のみを保存し、トークン自体は保存しません。ヘッダーなしのアップロードは匿名のままです。トークンの所持がそのまま認可なので、パスワードと同じように扱ってください。削除・期限切れ・ダウンロード上限に達したエントリは一覧から消え、1 トークンにつき最大 200 件保持されます。",
    "code.resumableUpload": "大きなアップロードを再開する",
    "code.createCollection": "複数ファイルを 1 つのリンクに",
    "code.ownerList": "このトークンのアップロード一覧",
    "api.archiveDesc":
      "複数ファイルを単一アーカイブとしてダウンロード（ZIP、TAR、TAR.GZ）。ファイルパスはカンマで区切ります。",
    "api.scan": "ウイルススキャン",
    "api.scanDesc":
      "ClamAV または VirusTotal でアップロードファイルをスキャン（サーバー設定が必要）。",
    "api.scanNote":
      "--perform-clamav-prescan を有効にすると、すべてのアップロードが受け入れ前に自動スキャンされます。感染ファイルは HTTP 412 で拒否されます。",
    "api.headers": "リクエストヘッダー",
    "api.headersDesc": "アップロードとダウンロードの動作を制御するオプションヘッダー。",
    "api.uploadHeaders": "アップロードヘッダー",
    "api.downloadHeaders": "ダウンロードヘッダー",
    "api.responseHeaders": "レスポンスヘッダー",
    "api.responseHeadersDesc": "アップロードおよびダウンロードレスポンスでサーバーが返すヘッダー。",
    "api.uploadResponseHeaders": "アップロードレスポンス",
    "api.downloadResponseHeaders": "ダウンロードレスポンス",
    "api.caching": "キャッシュ",
    "api.cachingDesc":
      "保存されたファイルは変更されないため、すべてのダウンロードに ETag が付き、If-None-Match で送り返すと 304 Not Modified を返します。ブラウザや CDN がレスポンスを保持してよいかはサーバー運用者が CACHE_MAX_AGE で決めるもので、既定では無効です。",
    "api.cachingLimited":
      "Max-Downloads 付きのファイルはキャッシュ対象になりません。キャッシュから返された分をオリジンは数えられないためです。",
    "api.cachingEncrypted":
      "サーバー側で暗号化されたファイルはキャッシュ対象になりません。リクエストごとにパスワードヘッダーで復号するためです。",
    "api.cachingDelete":
      "キャッシュを有効にすると、削除したファイルも max-age が切れるまではキャッシュ経由で取得できてしまいます。",
    "api.languages": "言語",
    "api.languagesDesc":
      "エラーメッセージは Accept-Language に従って英語・中国語・日本語で返されます。ヘッダーがなければ英語になり、実際に使われた言語は Content-Language で示されます。",
    "api.errors": "エラーコード",
    "api.errorsDesc": "API が返す一般的な HTTP ステータスコード。",
    // Code block titles
    "code.copy": "コピー",
    "code.uploadFile": "ファイルをアップロード",
    "code.encryptUpload": "暗号化してアップロード",
    "code.setLimits": "制限を設定",
    "code.downloadFile": "ファイルをダウンロード",
    "code.deleteFile": "ファイルを削除",
    "code.dockerRun": "Docker 実行",
    "code.dockerRunS3": "Docker 実行（S3）",
    "code.multiUpload": "複数ファイルをアップロード",
    "code.combineArchive": "アーカイブとして結合",
    "code.gpgEncrypt": "GPG 暗号化してアップロード",
    "code.gpgDecrypt": "ダウンロードして復号",
    "code.clamavScan": "ClamAV スキャン",
    "code.virustotalScan": "VirusTotal スキャン",
    "code.mysqlBackup": "MySQL バックアップ",
    "code.transferEmail": "転送してメール送信",
    "code.keybaseEncrypt": "Keybase 暗号化",
    "code.keybaseDecrypt": "Keybase 復号",
    "code.wget": "wget",
    "code.httpie": "httpie",
    "code.powershell": "powershell",
    "code.opensslEncrypt": "OpenSSL 暗号化",
    "code.opensslDecrypt": "OpenSSL 復号",
    "code.dockerAdvanced": "暗号化と制限設定",
    "code.pipeLogs": "ログをパイプ",
    // Use cases
    "usecase.title": "ユースケース",
    "usecase.subtitle": "curl、GPG、OpenSSL、その他のツールで send.to を使用する実際の例。",
    "usecase.multiUpload": "複数ファイルの一括アップロード",
    "usecase.multiUploadDesc":
      "マルチパートフォームデータで複数ファイルを一度にアップロード。ZIPやTARアーカイブとしてまとめてダウンロード。",
    "usecase.gpgEncrypt": "GPG で暗号化転送",
    "usecase.gpgEncryptDesc":
      "転送前に GPG でファイルを暗号化。自分の鍵を使い、gpg パイプで最高のセキュリティを実現。",
    "usecase.malwareScan": "マルウェアスキャン",
    "usecase.malwareScanDesc":
      "ClamAV でアップロードファイルのウイルスをスキャン、または VirusTotal にアップロードして包括的な分析を実行。",
    "usecase.dbBackup": "データベースバックアップ",
    "usecase.dbBackupDesc":
      "MySQL データベースをバックアップし、gzip で圧縮、GPG で暗号化、そして転送——すべて一つのパイプラインで。",
    "usecase.emailLink": "転送リンクをメールで送信",
    "usecase.emailLinkDesc":
      "ファイルをアップロードし、シェルの transfer 関数を使ってダウンロードリンクを直接メールで送信。",
    "usecase.keybase": "Keybase と連携",
    "usecase.keybaseDesc":
      "Keybase の鍵を使って特定の受信者向けにファイルを暗号化。インポート、暗号化、アップロード、復号をシームレスに。",
    "usecase.altClients": "代替クライアント",
    "usecase.altClientsDesc":
      "wget、HTTPie、PowerShell、サードパーティ CLI ツールに対応。お好みの HTTP クライアントを使用可能。",
    "usecase.openssl": "OpenSSL で暗号化転送",
    "usecase.opensslDesc": "転送前に OpenSSL AES-256-CBC でファイルを暗号化。GPG 不要。",
    "usecase.pipeLogs": "ログ＆出力をパイプ",
    "usecase.pipeLogsDesc":
      "任意のコマンド出力を直接 send.to にパイプ。syslog エントリ、grep 結果、任意のストリームを共有。",
    // Code block comments
    "code.comment.setLimits": "# 最大5回ダウンロード、7日後に期限切れ",
    "code.comment.multiUpload": "# 複数ファイルを一度にアップロード",
    "code.comment.combineArchive": "# zip または tar.gz でダウンロード",
    "code.comment.gpgEncrypt": "# gpg で暗号化してアップロード",
    "code.comment.gpgDecrypt": "# ダウンロードして復号",
    "code.comment.clamavScan": "# ClamAV でスキャン",
    "code.comment.virustotalScan": "# VirusTotal にアップロード",
    "code.comment.mysqlBackup": "# バックアップ、暗号化して転送",
    "code.comment.transferEmail": "# 転送してリンクをメールで送信",
    "code.comment.keybaseEncrypt": "# 受信者向けに暗号化してアップロード",
    "code.comment.keybaseDecrypt": "# ダウンロードして復号",
    "code.comment.wget": "# wget でアップロード",
    "code.comment.httpie": "# HTTPie でアップロード",
    "code.comment.powershell": "# PowerShell でアップロード",
    "code.comment.opensslEncrypt": "# openssl で暗号化してアップロード",
    "code.comment.opensslDecrypt": "# ダウンロードして復号",
    "code.comment.pipeLogs": "# syslog をフィルタして転送",
    // Q&A
  },
};

export function t(lang: Lang, key: string): string {
  return translations[lang]?.[key] ?? translations.en[key] ?? key;
}

export function getLangFromUrl(url: URL): Lang {
  const lang = url.searchParams.get("lang");
  if (lang && lang in languages) return lang as Lang;
  return defaultLang;
}

/** Detect language from browser navigator.language */
export function detectBrowserLang(): Lang {
  if (typeof navigator === "undefined") return defaultLang;
  const browserLang = navigator.language || (navigator as any).userLanguage || "";
  const prefix = browserLang.split("-")[0].toLowerCase();
  if (prefix in languages) return prefix as Lang;
  return defaultLang;
}

/** Get current lang from localStorage, falling back to browser detection */
export function getClientLang(): Lang {
  if (typeof window === "undefined") return defaultLang;
  const saved = localStorage.getItem("lang") as Lang | null;
  if (saved && saved in languages) return saved;
  return detectBrowserLang();
}
