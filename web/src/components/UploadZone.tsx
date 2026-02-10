// UploadZone — 拖拽上传 React 岛屿组件（支持 i18n）
import { useState, useCallback, useRef, useEffect } from "react";
import { Upload, File, Check, Copy, X, Loader2 } from "lucide-react";
import { translations, defaultLang, type Lang } from "../i18n/translations";

// 后端 API 地址：生产环境从环境变量读取，开发环境使用相对路径（走 Astro 代理）
const API_BASE = (import.meta as any).env?.PUBLIC_API_URL || "";

interface UploadResult {
  url: string;
  deleteUrl: string;
  filename: string;
  size: number;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

function useI18n() {
  const [lang, setLang] = useState<Lang>(defaultLang);

  useEffect(() => {
    const w = window as any;
    const l = w.__LANG__ || localStorage.getItem("lang") || defaultLang;
    if (l in translations) setLang(l as Lang);
  }, []);

  const t = useCallback(
    (key: string): string => {
      return translations[lang]?.[key] ?? translations.en[key] ?? key;
    },
    [lang]
  );

  return t;
}

export default function UploadZone() {
  const t = useI18n();
  const [isDragging, setIsDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [result, setResult] = useState<UploadResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleUpload = useCallback(async (file: globalThis.File) => {
    setUploading(true);
    setProgress(0);
    setError(null);
    setResult(null);

    try {
      const xhr = new XMLHttpRequest();
      xhr.upload.addEventListener("progress", (e) => {
        if (e.lengthComputable) {
          setProgress(Math.round((e.loaded / e.total) * 100));
        }
      });

      const responsePromise = new Promise<string>((resolve, reject) => {
        xhr.onload = () => {
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve(xhr.responseText);
          } else {
            reject(new Error(`Upload failed: ${xhr.status}`));
          }
        };
        xhr.onerror = () => reject(new Error("Network error"));
      });

      xhr.open("PUT", `${API_BASE}/${encodeURIComponent(file.name)}`);
      xhr.setRequestHeader("Content-Type", file.type || "application/octet-stream");
      xhr.send(file);

      const responseText = await responsePromise;
      const url = responseText.trim();
      const deleteUrl = xhr.getResponseHeader("X-Url-Delete") || "";

      setResult({
        url,
        deleteUrl,
        filename: file.name,
        size: file.size,
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Upload failed";
      if (msg.includes("502") || msg.includes("Network error")) {
        setError(t("upload.backendError"));
      } else {
        setError(msg);
      }
    } finally {
      setUploading(false);
    }
  }, [t]);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setIsDragging(false);
      const file = e.dataTransfer.files[0];
      if (file) handleUpload(file);
    },
    [handleUpload]
  );

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (file) handleUpload(file);
    },
    [handleUpload]
  );

  const copyToClipboard = useCallback(async (text: string, key: string) => {
    await navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  }, []);

  const reset = useCallback(() => {
    setResult(null);
    setError(null);
    setProgress(0);
  }, []);

  // 上传结果展示
  if (result) {
    return (
      <div className="rounded-xl border border-border bg-surface p-6" style={{ animation: "scale-in 0.2s ease-out" }}>
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Check className="h-4 w-4 text-success" />
            <span className="text-sm font-medium text-primary">{t("upload.complete")}</span>
          </div>
          <button
            onClick={reset}
            className="cursor-pointer rounded-md p-1 text-muted transition-all duration-200 hover:rotate-90 hover:text-primary"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="mb-4 flex items-center gap-3 text-sm text-muted">
          <File className="h-4 w-4" />
          <span>{result.filename}</span>
          <span className="text-border-light">/</span>
          <span>{formatBytes(result.size)}</span>
        </div>

        <div className="space-y-3">
          {/* 分享链接 */}
          <div>
            <label className="mb-1 block font-mono text-xs text-muted-dark">
              {t("upload.shareLink")}
            </label>
            <div className="flex items-center gap-2">
              <code className="flex-1 rounded-md border border-border bg-background px-3 py-2 font-mono text-sm text-primary">
                {result.url}
              </code>
              <button
                onClick={() => copyToClipboard(result.url, "url")}
                className="cursor-pointer rounded-md border border-border bg-background p-2 text-muted transition-colors hover:text-primary"
                aria-label="Copy link"
              >
                {copied === "url" ? (
                  <Check className="h-4 w-4 text-success" />
                ) : (
                  <Copy className="h-4 w-4" />
                )}
              </button>
            </div>
          </div>

          {/* curl 下载命令 */}
          <div>
            <label className="mb-1 block font-mono text-xs text-muted-dark">
              {t("upload.downloadCmd")}
            </label>
            <div className="flex items-center gap-2">
              <code className="flex-1 overflow-x-auto rounded-md border border-border bg-background px-3 py-2 font-mono text-sm text-primary">
                curl {result.url} -o {result.filename}
              </code>
              <button
                onClick={() =>
                  copyToClipboard(
                    `curl ${result.url} -o ${result.filename}`,
                    "curl"
                  )
                }
                className="cursor-pointer rounded-md border border-border bg-background p-2 text-muted transition-colors hover:text-primary"
                aria-label="Copy curl command"
              >
                {copied === "curl" ? (
                  <Check className="h-4 w-4 text-success" />
                ) : (
                  <Copy className="h-4 w-4" />
                )}
              </button>
            </div>
          </div>

          {/* 删除命令 */}
          {result.deleteUrl && (
            <div>
              <label className="mb-1 block font-mono text-xs text-muted-dark">
                {t("upload.deleteCmd")}
              </label>
              <div className="flex items-center gap-2">
                <code className="flex-1 overflow-x-auto rounded-md border border-border bg-background px-3 py-2 font-mono text-sm text-primary">
                  curl -X DELETE {result.deleteUrl}
                </code>
                <button
                  onClick={() =>
                    copyToClipboard(
                      `curl -X DELETE ${result.deleteUrl}`,
                      "delete"
                    )
                  }
                  className="cursor-pointer rounded-md border border-border bg-background p-2 text-muted transition-colors hover:text-primary"
                  aria-label="Copy delete command"
                >
                  {copied === "delete" ? (
                    <Check className="h-4 w-4 text-success" />
                  ) : (
                    <Copy className="h-4 w-4" />
                  )}
                </button>
              </div>
            </div>
          )}
        </div>

        <button
          onClick={reset}
          className="mt-4 w-full cursor-pointer rounded-lg border border-border bg-background py-2.5 text-sm text-muted transition-all duration-200 hover:border-border-light hover:text-primary active:scale-[0.98]"
        >
          {t("upload.another")}
        </button>
      </div>
    );
  }

  // 上传中
  if (uploading) {
    return (
      <div className="rounded-xl border border-border bg-surface p-8">
        <div className="flex flex-col items-center gap-4">
          <Loader2 className="h-8 w-8 animate-spin text-muted" />
          <div className="w-full max-w-xs">
            <div className="mb-2 flex justify-between text-sm">
              <span className="text-muted">{t("upload.uploading")}</span>
              <span className="font-mono text-primary">{progress}%</span>
            </div>
            <div className="relative h-1.5 overflow-hidden rounded-full bg-border">
              <div
                className="h-full rounded-full bg-primary transition-all duration-300"
                style={{ width: `${progress}%` }}
              />
              <div
                className="absolute inset-0 h-full w-full"
                style={{
                  background: "linear-gradient(90deg, transparent, rgba(255,255,255,0.15), transparent)",
                  animation: "progress-shimmer 1.5s ease-in-out infinite",
                }}
              />
            </div>
          </div>
        </div>
      </div>
    );
  }

  // 拖拽上传区
  return (
    <div>
      {error && (
        <div className="mb-4 rounded-lg border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
          {error}
        </div>
      )}
      <div
        onDragOver={(e) => {
          e.preventDefault();
          setIsDragging(true);
        }}
        onDragLeave={() => setIsDragging(false)}
        onDrop={handleDrop}
        onClick={() => fileInputRef.current?.click()}
        className={`cursor-pointer rounded-xl border-2 border-dashed p-12 text-center transition-all duration-300 ${
          isDragging
            ? "scale-[1.01] border-zinc-400 bg-surface-light shadow-lg"
            : "border-border hover:border-border-light hover:bg-surface"
        }`}
      >
        <Upload
          className={`mx-auto mb-4 h-8 w-8 transition-all duration-300 ${
            isDragging ? "-translate-y-1 text-primary" : "text-muted-dark"
          }`}
        />
        <p className="mb-1 text-sm text-primary">
          {t("upload.dropzone")}{" "}
          <span className="underline underline-offset-4">{t("upload.browse")}</span>
        </p>
        <p className="text-xs text-muted-dark">
          {t("upload.hint")}
        </p>
        <input
          ref={fileInputRef}
          type="file"
          onChange={handleFileSelect}
          className="hidden"
        />
      </div>
    </div>
  );
}
