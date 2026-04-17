// UploadZone — Claude-themed drag-and-drop React island
import { Component, useState, useCallback, useRef, useEffect, type ReactNode } from "react";
import { UploadCloud, File, Check, Copy, X, Loader2 } from "lucide-react";
import { translations, defaultLang, type Lang } from "../i18n/translations";

const API_BASE = (import.meta as any).env?.PUBLIC_API_URL || "";
const MAX_UPLOAD_BYTES = Number((import.meta as any).env?.PUBLIC_MAX_UPLOAD_BYTES) || 0;

interface UploadResult {
  url: string;
  deleteUrl: string;
  filename: string;
  size: number;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

function formatSpeed(bytesPerSec: number): string {
  if (!Number.isFinite(bytesPerSec) || bytesPerSec <= 0) return "—";
  return `${formatBytes(bytesPerSec)}/s`;
}

function formatEta(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "—";
  if (seconds < 60) return `${Math.ceil(seconds)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.ceil(seconds - m * 60);
  if (m < 60) return `${m}m ${s}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m - h * 60}m`;
}

function truncateMiddle(s: string, max = 40): string {
  if (s.length <= max) return s;
  const head = Math.ceil((max - 1) / 2);
  const tail = Math.floor((max - 1) / 2);
  return `${s.slice(0, head)}…${s.slice(-tail)}`;
}

async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

function useI18n() {
  const [lang, setLang] = useState<Lang>(defaultLang);
  useEffect(() => {
    const w = window as any;
    const l = w.__LANG__ || localStorage.getItem("lang") || defaultLang;
    if (l in translations) setLang(l as Lang);
  }, []);
  return useCallback(
    (key: string): string => translations[lang]?.[key] ?? translations.en[key] ?? key,
    [lang]
  );
}

class UploadErrorBoundary extends Component<
  { children: ReactNode; fallback: ReactNode },
  { hasError: boolean }
> {
  state = { hasError: false };
  static getDerivedStateFromError() {
    return { hasError: true };
  }
  componentDidCatch(err: unknown) {
    console.error("UploadZone error", err);
  }
  render() {
    return this.state.hasError ? this.props.fallback : this.props.children;
  }
}

function UploadZoneInner() {
  const t = useI18n();
  const [isDragging, setIsDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [speed, setSpeed] = useState(0);
  const [eta, setEta] = useState(0);
  const [result, setResult] = useState<UploadResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const xhrRef = useRef<XMLHttpRequest | null>(null);

  useEffect(() => () => xhrRef.current?.abort(), []);

  const handleUpload = useCallback(
    async (file: globalThis.File) => {
      if (MAX_UPLOAD_BYTES > 0 && file.size > MAX_UPLOAD_BYTES) {
        setError(
          `${t("upload.tooLarge")} (${formatBytes(file.size)} > ${formatBytes(MAX_UPLOAD_BYTES)})`
        );
        return;
      }

      setUploading(true);
      setProgress(0);
      setSpeed(0);
      setEta(0);
      setError(null);
      setResult(null);

      const xhr = new XMLHttpRequest();
      xhrRef.current = xhr;
      const startedAt = performance.now();

      xhr.upload.addEventListener("progress", (e) => {
        if (!e.lengthComputable) return;
        const pct = Math.round((e.loaded / e.total) * 100);
        const elapsedSec = (performance.now() - startedAt) / 1000;
        const bytesPerSec = elapsedSec > 0 ? e.loaded / elapsedSec : 0;
        const remaining = bytesPerSec > 0 ? (e.total - e.loaded) / bytesPerSec : 0;
        setProgress(pct);
        setSpeed(bytesPerSec);
        setEta(remaining);
      });

      const responsePromise = new Promise<string>((resolve, reject) => {
        xhr.onload = () => {
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve(xhr.responseText);
          } else {
            reject(new Error(`Upload failed: ${xhr.status} ${xhr.statusText}`));
          }
        };
        xhr.onerror = () => reject(new Error("Network error"));
        xhr.onabort = () => reject(new Error("aborted"));
      });

      try {
        xhr.open("PUT", `${API_BASE}/${encodeURIComponent(file.name)}`);
        xhr.setRequestHeader("Content-Type", file.type || "application/octet-stream");
        xhr.send(file);

        const responseText = await responsePromise;
        const url = responseText.trim();
        const deleteUrl = xhr.getResponseHeader("X-Url-Delete") || "";

        setResult({ url, deleteUrl, filename: file.name, size: file.size });
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Upload failed";
        if (msg === "aborted") {
          // user-initiated; no toast
        } else if (msg.includes("502") || msg.includes("Network error")) {
          setError(t("upload.backendError"));
        } else {
          setError(msg);
        }
      } finally {
        xhrRef.current = null;
        setUploading(false);
      }
    },
    [t]
  );

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
      e.target.value = "";
    },
    [handleUpload]
  );

  const copyToClipboard = useCallback(
    async (text: string, key: string) => {
      const ok = await copyText(text);
      if (ok) {
        setCopied(key);
        setTimeout(() => setCopied(null), 2000);
      } else {
        setError(t("upload.copyFailed"));
      }
    },
    [t]
  );

  const cancelUpload = useCallback(() => xhrRef.current?.abort(), []);

  const reset = useCallback(() => {
    setResult(null);
    setError(null);
    setProgress(0);
    setSpeed(0);
    setEta(0);
  }, []);

  // ============ RESULT VIEW ============
  if (result) {
    return (
      <div
        className="rounded-3xl border border-border bg-surface p-8 shadow-[0_4px_24px_rgba(0,0,0,0.04)]"
        style={{ animation: "scale-in 0.25s ease-out" }}
      >
        <div className="mb-6 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-full bg-[#5a8a4a1a] text-[#5a8a4a]">
              <Check className="h-4 w-4" strokeWidth={2} />
            </div>
            <span className="font-serif text-[22px] font-medium text-primary">{t("upload.complete")}</span>
          </div>
          <button
            onClick={reset}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-border bg-surface-light text-muted transition-colors hover:text-primary"
            aria-label={t("upload.close")}
          >
            <X className="h-4 w-4" strokeWidth={1.5} />
          </button>
        </div>

        <div className="mb-6 flex min-w-0 items-center gap-3 rounded-xl border border-border bg-background px-4 py-3">
          <File className="h-4 w-4 shrink-0 text-muted" strokeWidth={1.5} />
          <span className="truncate text-[15px] text-primary" title={result.filename}>
            {truncateMiddle(result.filename, 56)}
          </span>
          <span className="ml-auto shrink-0 text-xs text-muted-dark">{formatBytes(result.size)}</span>
        </div>

        <div className="space-y-4">
          <div>
            <label className="mb-2 block text-[13px] font-medium text-primary">
              {t("upload.shareLink")}
            </label>
            <div className="flex gap-2">
              <code
                className="flex-1 truncate rounded-xl border border-border bg-background px-4 py-3 font-mono text-[13px] text-primary"
                title={result.url}
              >
                {result.url}
              </code>
              <button
                onClick={() => copyToClipboard(result.url, "url")}
                className="flex w-12 shrink-0 items-center justify-center rounded-xl border border-border bg-surface-light text-muted transition-colors hover:text-primary"
                aria-label={t("upload.copyLink")}
              >
                {copied === "url" ? <Check className="h-4 w-4 text-[#5a8a4a]" strokeWidth={2} /> : <Copy className="h-4 w-4" strokeWidth={1.5} />}
              </button>
            </div>
          </div>

          <div>
            <label className="mb-2 block text-[13px] font-medium text-primary">
              {t("upload.downloadCmd")}
            </label>
            <div className="flex gap-2">
              <code className="flex-1 overflow-x-auto rounded-xl border border-border bg-background px-4 py-3 font-mono text-[13px] text-primary">
                curl {result.url} -o {result.filename}
              </code>
              <button
                onClick={() => copyToClipboard(`curl ${result.url} -o ${result.filename}`, "curl")}
                className="flex w-12 shrink-0 items-center justify-center rounded-xl border border-border bg-surface-light text-muted transition-colors hover:text-primary"
                aria-label={t("upload.copyCurl")}
              >
                {copied === "curl" ? <Check className="h-4 w-4 text-[#5a8a4a]" strokeWidth={2} /> : <Copy className="h-4 w-4" strokeWidth={1.5} />}
              </button>
            </div>
          </div>

          {result.deleteUrl && (
            <div>
              <label className="mb-2 block text-[13px] font-medium text-primary">
                {t("upload.deleteCmd")}
              </label>
              <div className="flex gap-2">
                <code className="flex-1 overflow-x-auto rounded-xl border border-border bg-background px-4 py-3 font-mono text-[13px] text-primary">
                  curl -X DELETE {result.deleteUrl}
                </code>
                <button
                  onClick={() => copyToClipboard(`curl -X DELETE ${result.deleteUrl}`, "delete")}
                  className="flex w-12 shrink-0 items-center justify-center rounded-xl border border-border bg-surface-light text-muted transition-colors hover:text-primary"
                  aria-label={t("upload.copyDelete")}
                >
                  {copied === "delete" ? <Check className="h-4 w-4 text-[#5a8a4a]" strokeWidth={2} /> : <Copy className="h-4 w-4" strokeWidth={1.5} />}
                </button>
              </div>
            </div>
          )}
        </div>

        <button onClick={reset} className="btn btn-sand mt-6 w-full">
          {t("upload.another")}
        </button>
      </div>
    );
  }

  // ============ UPLOADING VIEW ============
  if (uploading) {
    return (
      <div
        className="rounded-3xl border border-border bg-surface p-12 shadow-[0_4px_24px_rgba(0,0,0,0.04)]"
        role="status"
        aria-live="polite"
        aria-label={t("upload.uploading")}
      >
        <div className="flex flex-col items-center gap-6">
          <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-[#c9644214]">
            <Loader2 className="h-7 w-7 animate-spin text-[#c96442]" strokeWidth={1.5} />
          </div>
          <div className="w-full max-w-md">
            <div className="mb-3 flex justify-between text-sm">
              <span className="text-muted">{t("upload.uploading")}</span>
              <span className="font-medium text-primary">{progress}%</span>
            </div>
            <div
              className="relative h-1.5 overflow-hidden rounded-full bg-border"
              role="progressbar"
              aria-valuenow={progress}
              aria-valuemin={0}
              aria-valuemax={100}
            >
              <div
                className="h-full rounded-full bg-[#c96442] transition-all duration-300"
                style={{ width: `${progress}%` }}
              />
            </div>
            <div className="mt-3 flex justify-between text-xs text-muted-dark">
              <span>{formatSpeed(speed)}</span>
              <span>{t("upload.eta")} {formatEta(eta)}</span>
            </div>
          </div>
          <button onClick={cancelUpload} className="btn btn-sand btn-sm">
            {t("upload.cancel")}
          </button>
        </div>
      </div>
    );
  }

  // ============ DROPZONE VIEW ============
  const onKeyActivate = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      fileInputRef.current?.click();
    }
  };

  return (
    <div>
      {error && (
        <div
          className="mb-4 flex items-center justify-between rounded-xl border border-[#b5333333] bg-[#b533330a] px-4 py-3 text-sm text-[#b53333]"
          role="alert"
        >
          <span>{error}</span>
          <button onClick={() => setError(null)} aria-label="Dismiss">
            <X className="h-4 w-4" strokeWidth={1.5} />
          </button>
        </div>
      )}
      <div
        role="button"
        tabIndex={0}
        aria-label={t("upload.dropzone")}
        onDragOver={(e) => {
          e.preventDefault();
          setIsDragging(true);
        }}
        onDragLeave={() => setIsDragging(false)}
        onDrop={handleDrop}
        onClick={() => fileInputRef.current?.click()}
        onKeyDown={onKeyActivate}
        className={`cursor-pointer rounded-3xl border bg-surface p-16 text-center transition-all duration-200 focus:outline-none ${
          isDragging
            ? "border-[#c96442] bg-[#c9644208] shadow-[0_0_0_4px_rgba(201,100,66,0.08)]"
            : "border-border hover:border-border-light hover:bg-surface-light"
        }`}
        style={{
          borderStyle: isDragging ? "solid" : "dashed",
        }}
      >
        <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-[#c9644214]">
          <UploadCloud
            className="h-7 w-7 text-[#c96442]"
            strokeWidth={1.5}
          />
        </div>
        <p className="font-serif mb-2 text-[28px] font-medium text-primary">
          {t("upload.dropzone")}
        </p>
        <p className="text-[15px] text-muted">{t("upload.hint")}</p>
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

export default function UploadZone() {
  const fallback = (
    <div
      className="rounded-3xl border border-[#b5333333] bg-[#b533330a] p-8 text-sm text-[#b53333]"
      role="alert"
    >
      Upload widget crashed — please reload the page.
    </div>
  );
  return (
    <UploadErrorBoundary fallback={fallback}>
      <UploadZoneInner />
    </UploadErrorBoundary>
  );
}
