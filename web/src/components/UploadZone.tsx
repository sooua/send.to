// UploadZone — Claude-themed drag-and-drop React island
import {
  Component,
  useState,
  useCallback,
  useRef,
  useEffect,
  useMemo,
  type ReactNode,
} from "react";
import {
  UploadCloud,
  File,
  Check,
  Copy,
  X,
  Loader2,
  Settings2,
  QrCode,
  RotateCcw,
  Trash2,
  Lock,
  Eye,
  EyeOff,
} from "lucide-react";
import { translations, defaultLang, type Lang } from "../i18n/translations";

const API_BASE = (import.meta as any).env?.PUBLIC_API_URL || "";
const MAX_UPLOAD_BYTES = Number((import.meta as any).env?.PUBLIC_MAX_UPLOAD_BYTES) || 0;

const HISTORY_KEY = "sendto.history";
const OPTIONS_KEY = "sendto.options";
const HISTORY_LIMIT = 50;

interface UploadResult {
  url: string;
  deleteUrl: string;
  filename: string;
  size: number;
  encrypted: boolean;
  expiresAt?: string;
  maxDownloads?: number;
}

interface HistoryEntry extends UploadResult {
  at: number;
}

type ItemStatus = "queued" | "uploading" | "done" | "error";

interface QueueItem {
  id: string;
  file: globalThis.File;
  status: ItemStatus;
  progress: number;
  speed: number;
  eta: number;
  result?: UploadResult;
  error?: string;
}

interface UploadOptions {
  expiryDays: string;
  maxDownloads: string;
  password: string;
}

const defaultOptions: UploadOptions = { expiryDays: "", maxDownloads: "", password: "" };

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

function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString();
}

function truncateMiddle(s: string, max = 40): string {
  if (s.length <= max) return s;
  const head = Math.ceil((max - 1) / 2);
  const tail = Math.floor((max - 1) / 2);
  return `${s.slice(0, head)}…${s.slice(-tail)}`;
}

// Shell-quote a value that goes into a copyable curl command, so a filename
// containing spaces or quotes cannot break (or extend) the command.
function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
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

function loadHistory(): HistoryEntry[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((e) => e && typeof e.url === "string") : [];
  } catch {
    return [];
  }
}

function saveHistory(entries: HistoryEntry[]) {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(entries.slice(0, HISTORY_LIMIT)));
  } catch {
    // Quota or private-mode failures are not worth interrupting an upload for.
  }
}

function loadOptions(): UploadOptions {
  try {
    const raw = localStorage.getItem(OPTIONS_KEY);
    if (!raw) return defaultOptions;
    const parsed = JSON.parse(raw);
    return {
      ...defaultOptions,
      expiryDays: typeof parsed.expiryDays === "string" ? parsed.expiryDays : "",
      maxDownloads: typeof parsed.maxDownloads === "string" ? parsed.maxDownloads : "",
    };
  } catch {
    return defaultOptions;
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

/** Upload one file, reporting progress. Resolves with the parsed result. */
function uploadFile(
  file: globalThis.File,
  options: UploadOptions,
  onProgress: (progress: number, speed: number, eta: number) => void,
  registerXhr: (xhr: XMLHttpRequest | null) => void
): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    registerXhr(xhr);

    const startedAt = performance.now();

    xhr.upload.addEventListener("progress", (e) => {
      if (!e.lengthComputable) return;
      const elapsedSec = (performance.now() - startedAt) / 1000;
      const bytesPerSec = elapsedSec > 0 ? e.loaded / elapsedSec : 0;
      const remaining = bytesPerSec > 0 ? (e.total - e.loaded) / bytesPerSec : 0;
      onProgress(Math.round((e.loaded / e.total) * 100), bytesPerSec, remaining);
    });

    xhr.onload = () => {
      registerXhr(null);
      if (xhr.status < 200 || xhr.status >= 300) {
        const detail = (xhr.responseText || "").trim();
        reject(new Error(detail || `Upload failed: ${xhr.status} ${xhr.statusText}`));
        return;
      }

      // The server answers JSON when asked; fall back to the historical
      // plain-text URL + X-Url-Delete header so an older backend still works.
      try {
        const parsed = JSON.parse(xhr.responseText);
        resolve({
          url: parsed.url,
          deleteUrl: parsed.delete_url || "",
          filename: parsed.filename || file.name,
          size: typeof parsed.size === "number" ? parsed.size : file.size,
          encrypted: Boolean(parsed.encrypted),
          expiresAt: parsed.expires_at,
          maxDownloads: parsed.max_downloads,
        });
      } catch {
        resolve({
          url: (xhr.responseText || "").trim(),
          deleteUrl: xhr.getResponseHeader("X-Url-Delete") || "",
          filename: file.name,
          size: file.size,
          encrypted: Boolean(options.password),
        });
      }
    };
    xhr.onerror = () => {
      registerXhr(null);
      reject(new Error("Network error"));
    };
    xhr.onabort = () => {
      registerXhr(null);
      reject(new Error("aborted"));
    };

    xhr.open("PUT", `${API_BASE}/${encodeURIComponent(file.name)}`);
    xhr.setRequestHeader("Content-Type", file.type || "application/octet-stream");
    xhr.setRequestHeader("Accept", "application/json");

    if (options.expiryDays) xhr.setRequestHeader("Max-Days", options.expiryDays);
    if (options.maxDownloads) xhr.setRequestHeader("Max-Downloads", options.maxDownloads);
    if (options.password) xhr.setRequestHeader("X-Encrypt-Password", options.password);

    xhr.send(file);
  });
}

function CopyRow({
  label,
  value,
  copyKey,
  copied,
  onCopy,
  ariaLabel,
}: {
  label: string;
  value: string;
  copyKey: string;
  copied: string | null;
  onCopy: (text: string, key: string) => void;
  ariaLabel: string;
}) {
  return (
    <div>
      <label className="mb-2 block text-[13px] font-medium text-primary">{label}</label>
      <div className="flex gap-2">
        <code
          className="flex-1 overflow-x-auto whitespace-nowrap rounded-xl border border-border bg-background px-4 py-3 font-mono text-[13px] text-primary"
          title={value}
        >
          {value}
        </code>
        <button
          onClick={() => onCopy(value, copyKey)}
          className="flex w-12 shrink-0 items-center justify-center rounded-xl border border-border bg-surface-light text-muted transition-colors hover:text-primary"
          aria-label={ariaLabel}
        >
          {copied === copyKey ? (
            <Check className="h-4 w-4 text-[#5a8a4a]" strokeWidth={2} />
          ) : (
            <Copy className="h-4 w-4" strokeWidth={1.5} />
          )}
        </button>
      </div>
    </div>
  );
}

function UploadZoneInner() {
  const t = useI18n();
  const [isDragging, setIsDragging] = useState(false);
  const [queue, setQueue] = useState<QueueItem[]>([]);
  const [results, setResults] = useState<UploadResult[]>([]);
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [showOptions, setShowOptions] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [showQr, setShowQr] = useState<string | null>(null);
  const [options, setOptions] = useState<UploadOptions>(defaultOptions);
  const [deleting, setDeleting] = useState<string | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const xhrRef = useRef<XMLHttpRequest | null>(null);
  const runningRef = useRef(false);
  // The drain loop needs the live queue, so every mutation site updates this
  // ref alongside the state. It is never written during render.
  const queueRef = useRef<QueueItem[]>([]);
  // The upload loop reads options at send time; a ref keeps it from capturing
  // a stale copy when the user edits a field mid-queue.
  const optionsRef = useRef(options);

  useEffect(() => {
    optionsRef.current = options;
  }, [options]);

  // localStorage is unavailable during Astro's server render, so both are
  // hydrated after mount rather than in a lazy state initialiser.
  useEffect(() => {
    setHistory(loadHistory());
    setOptions(loadOptions());
  }, []);

  useEffect(() => () => xhrRef.current?.abort(), []);

  // Persist the limits but never the password.
  useEffect(() => {
    try {
      localStorage.setItem(
        OPTIONS_KEY,
        JSON.stringify({ expiryDays: options.expiryDays, maxDownloads: options.maxDownloads })
      );
    } catch {
      // ignore
    }
  }, [options.expiryDays, options.maxDownloads]);

  const pushHistory = useCallback((result: UploadResult) => {
    setHistory((prev) => {
      const next = [{ ...result, at: Date.now() }, ...prev].slice(0, HISTORY_LIMIT);
      saveHistory(next);
      return next;
    });
  }, []);

  const updateItem = useCallback((id: string, patch: Partial<QueueItem>) => {
    setQueue((prev) => prev.map((item) => (item.id === id ? { ...item, ...patch } : item)));
  }, []);

  // Drains the queue one file at a time. Sequential rather than parallel so
  // the per-file speed and ETA readouts mean something on a shared uplink.
  const drainQueue = useCallback(async () => {
    if (runningRef.current) return;
    runningRef.current = true;

    try {
      for (;;) {
        const item = queueRef.current.find((q) => q.status === "queued");
        if (!item) break;

        // Mark it in the ref too: React state updates are async, and the next
        // loop iteration must not pick the same item again.
        queueRef.current = queueRef.current.map((q) =>
          q.id === item.id ? { ...q, status: "uploading" as ItemStatus } : q
        );
        updateItem(item.id, {
          status: "uploading",
          progress: 0,
          speed: 0,
          eta: 0,
          error: undefined,
        });

        try {
          const result = await uploadFile(
            item.file,
            optionsRef.current,
            (progress, speed, eta) => updateItem(item.id, { progress, speed, eta }),
            (xhr) => {
              xhrRef.current = xhr;
            }
          );

          queueRef.current = queueRef.current.map((q) =>
            q.id === item.id ? { ...q, status: "done" as ItemStatus } : q
          );
          updateItem(item.id, { status: "done", progress: 100, result });
          setResults((prev) => [...prev, result]);
          pushHistory(result);
        } catch (err) {
          const msg = err instanceof Error ? err.message : "Upload failed";

          if (msg === "aborted") {
            queueRef.current = queueRef.current.filter((q) => q.id !== item.id);
            setQueue((prev) => prev.filter((q) => q.id !== item.id));
            continue;
          }

          const friendly =
            msg.includes("502") || msg === "Network error" ? t("upload.backendError") : msg;

          queueRef.current = queueRef.current.map((q) =>
            q.id === item.id ? { ...q, status: "error" as ItemStatus } : q
          );
          updateItem(item.id, { status: "error", error: friendly });
        }
      }
    } finally {
      runningRef.current = false;
    }
  }, [pushHistory, t, updateItem]);

  const enqueue = useCallback(
    (files: globalThis.File[]) => {
      if (files.length === 0) return;

      const accepted: QueueItem[] = [];
      let rejected: string | null = null;

      for (const file of files) {
        if (MAX_UPLOAD_BYTES > 0 && file.size > MAX_UPLOAD_BYTES) {
          rejected = `${t("upload.tooLarge")}: ${file.name} (${formatBytes(file.size)} > ${formatBytes(MAX_UPLOAD_BYTES)})`;
          continue;
        }
        accepted.push({
          id: `${file.name}-${file.size}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
          file,
          status: "queued",
          progress: 0,
          speed: 0,
          eta: 0,
        });
      }

      setError(rejected);
      if (accepted.length === 0) return;

      queueRef.current = [...queueRef.current, ...accepted];
      setQueue((prev) => [...prev, ...accepted]);
      void drainQueue();
    },
    [drainQueue, t]
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setIsDragging(false);

      // A dropped directory arrives as an entry with an empty File, which
      // used to upload as a 0-byte file or silently do nothing.
      const items = Array.from(e.dataTransfer.items || []);
      const hasDirectory = items.some((item) => {
        const entry = (item as any).webkitGetAsEntry?.();
        return entry?.isDirectory;
      });

      if (hasDirectory) {
        setError(t("upload.foldersUnsupported"));
        return;
      }

      enqueue(Array.from(e.dataTransfer.files));
    },
    [enqueue, t]
  );

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      enqueue(Array.from(e.target.files || []));
      e.target.value = "";
    },
    [enqueue]
  );

  // Ctrl+V / Cmd+V uploads whatever is on the clipboard — the fastest path
  // for sharing a screenshot.
  useEffect(() => {
    const onPaste = (e: ClipboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (target && ["INPUT", "TEXTAREA"].includes(target.tagName)) return;

      const files = Array.from(e.clipboardData?.files || []);
      if (files.length === 0) return;

      e.preventDefault();
      enqueue(files);
    };

    window.addEventListener("paste", onPaste);
    return () => window.removeEventListener("paste", onPaste);
  }, [enqueue]);

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

  const cancelAll = useCallback(() => {
    queueRef.current = queueRef.current.filter(
      (item) => item.status === "done" || item.status === "error"
    );
    setQueue((prev) => prev.filter((item) => item.status === "done" || item.status === "error"));
    xhrRef.current?.abort();
  }, []);

  const retryItem = useCallback(
    (id: string) => {
      queueRef.current = queueRef.current.map((q) =>
        q.id === id ? { ...q, status: "queued" as ItemStatus, error: undefined, progress: 0 } : q
      );
      updateItem(id, { status: "queued", error: undefined, progress: 0 });
      void drainQueue();
    },
    [drainQueue, updateItem]
  );

  const removeItem = useCallback((id: string) => {
    queueRef.current = queueRef.current.filter((item) => item.id !== id);
    setQueue((prev) => prev.filter((item) => item.id !== id));
  }, []);

  const deleteUpload = useCallback(
    async (entry: HistoryEntry) => {
      if (!entry.deleteUrl) return;
      setDeleting(entry.url);
      try {
        const res = await fetch(entry.deleteUrl, { method: "DELETE" });
        // A 404 means it is already gone, which is the desired end state.
        if (!res.ok && res.status !== 404) throw new Error(String(res.status));

        setHistory((prev) => {
          const next = prev.filter((e) => e.url !== entry.url);
          saveHistory(next);
          return next;
        });
        setResults((prev) => prev.filter((r) => r.url !== entry.url));
      } catch {
        setError(t("upload.deleteFailed"));
      } finally {
        setDeleting(null);
      }
    },
    [t]
  );

  const clearHistory = useCallback(() => {
    setHistory([]);
    saveHistory([]);
  }, []);

  const reset = useCallback(() => {
    queueRef.current = [];
    setResults([]);
    setQueue([]);
    setError(null);
    setShowQr(null);
  }, []);

  const pending = queue.filter((item) => item.status === "uploading" || item.status === "queued");
  const failed = queue.filter((item) => item.status === "error");

  const optionsSummary = useMemo(() => {
    const parts: string[] = [];
    if (options.expiryDays) parts.push(`${options.expiryDays}d`);
    if (options.maxDownloads) parts.push(`≤${options.maxDownloads}`);
    if (options.password) parts.push("🔒");
    return parts.join(" · ");
  }, [options]);

  return (
    <div className="space-y-6">
      {error && (
        <div
          className="flex items-center justify-between rounded-xl border border-[#b5333333] bg-[#b533330a] px-4 py-3 text-sm text-[#b53333]"
          role="alert"
        >
          <span>{error}</span>
          <button onClick={() => setError(null)} aria-label={t("upload.close")}>
            <X className="h-4 w-4" strokeWidth={1.5} />
          </button>
        </div>
      )}

      {/* ============ DROPZONE ============ */}
      <div>
        <div
          role="button"
          tabIndex={0}
          aria-label={t("upload.dropzoneMulti")}
          onDragOver={(e) => {
            e.preventDefault();
            setIsDragging(true);
          }}
          onDragLeave={() => setIsDragging(false)}
          onDrop={handleDrop}
          onClick={() => fileInputRef.current?.click()}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              fileInputRef.current?.click();
            }
          }}
          className={`cursor-pointer rounded-3xl border bg-surface p-16 text-center transition-all duration-200 focus:outline-none ${
            isDragging
              ? "border-[#c96442] bg-[#c9644208] shadow-[0_0_0_4px_rgba(201,100,66,0.08)]"
              : "border-border hover:border-border-light hover:bg-surface-light"
          }`}
          style={{ borderStyle: isDragging ? "solid" : "dashed" }}
        >
          <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-[#c9644214]">
            <UploadCloud className="h-7 w-7 text-[#c96442]" strokeWidth={1.5} />
          </div>
          <p className="font-serif mb-2 text-[28px] font-medium text-primary">
            {t("upload.dropzoneMulti")}
          </p>
          <p className="text-[15px] text-muted">{t("upload.hintMulti")}</p>
          <input
            ref={fileInputRef}
            type="file"
            multiple
            onChange={handleFileSelect}
            className="hidden"
          />
        </div>

        {/* ============ OPTIONS ============ */}
        <div className="mt-3">
          <button
            onClick={() => setShowOptions((v) => !v)}
            className="flex items-center gap-2 rounded-lg px-2 py-1 text-[13px] text-muted transition-colors hover:text-primary"
            aria-expanded={showOptions}
          >
            <Settings2 className="h-4 w-4" strokeWidth={1.5} />
            <span>{showOptions ? t("upload.optionsHide") : t("upload.options")}</span>
            {!showOptions && optionsSummary && (
              <span className="font-mono text-[12px] text-muted-dark">{optionsSummary}</span>
            )}
          </button>

          {showOptions && (
            <div className="mt-3 grid gap-4 rounded-2xl border border-border bg-surface p-5 sm:grid-cols-2">
              <div>
                <label
                  htmlFor="opt-expiry"
                  className="mb-2 block text-[13px] font-medium text-primary"
                >
                  {t("upload.expiryDays")}
                </label>
                <input
                  id="opt-expiry"
                  type="number"
                  min={1}
                  max={36500}
                  value={options.expiryDays}
                  placeholder={t("upload.expiryNever")}
                  onChange={(e) => setOptions((o) => ({ ...o, expiryDays: e.target.value }))}
                  className="w-full rounded-xl border border-border bg-background px-4 py-2.5 text-[14px] text-primary outline-none focus:border-[#c96442]"
                />
              </div>

              <div>
                <label
                  htmlFor="opt-downloads"
                  className="mb-2 block text-[13px] font-medium text-primary"
                >
                  {t("upload.maxDownloads")}
                </label>
                <input
                  id="opt-downloads"
                  type="number"
                  min={1}
                  value={options.maxDownloads}
                  placeholder={t("upload.maxDownloadsUnlimited")}
                  onChange={(e) => setOptions((o) => ({ ...o, maxDownloads: e.target.value }))}
                  className="w-full rounded-xl border border-border bg-background px-4 py-2.5 text-[14px] text-primary outline-none focus:border-[#c96442]"
                />
              </div>

              <div className="sm:col-span-2">
                <label
                  htmlFor="opt-password"
                  className="mb-2 flex items-center gap-2 text-[13px] font-medium text-primary"
                >
                  <Lock className="h-3.5 w-3.5" strokeWidth={1.5} />
                  {t("upload.password")}
                </label>
                <div className="flex gap-2">
                  <input
                    id="opt-password"
                    type={showPassword ? "text" : "password"}
                    autoComplete="new-password"
                    value={options.password}
                    placeholder={t("upload.passwordPlaceholder")}
                    onChange={(e) => setOptions((o) => ({ ...o, password: e.target.value }))}
                    className="w-full rounded-xl border border-border bg-background px-4 py-2.5 font-mono text-[14px] text-primary outline-none focus:border-[#c96442]"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword((v) => !v)}
                    className="flex w-12 shrink-0 items-center justify-center rounded-xl border border-border bg-surface-light text-muted transition-colors hover:text-primary"
                    aria-label={showPassword ? t("upload.passwordHide") : t("upload.passwordShow")}
                  >
                    {showPassword ? (
                      <EyeOff className="h-4 w-4" strokeWidth={1.5} />
                    ) : (
                      <Eye className="h-4 w-4" strokeWidth={1.5} />
                    )}
                  </button>
                </div>
                <p className="mt-2 text-[12px] text-muted-dark">{t("upload.passwordHint")}</p>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* ============ QUEUE ============ */}
      {(pending.length > 0 || failed.length > 0) && (
        <div
          className="rounded-3xl border border-border bg-surface p-6"
          role="status"
          aria-live="polite"
        >
          <div className="mb-4 flex items-center justify-between">
            <span className="font-serif text-[18px] font-medium text-primary">
              {t("upload.uploading")}
            </span>
            {pending.length > 0 && (
              <button onClick={cancelAll} className="btn btn-sand btn-sm">
                {t("upload.cancelAll")}
              </button>
            )}
          </div>

          <ul className="space-y-4">
            {[...pending, ...failed].map((item) => (
              <li key={item.id} className="min-w-0">
                <div className="mb-2 flex items-center gap-3">
                  {item.status === "uploading" ? (
                    <Loader2
                      className="h-4 w-4 shrink-0 animate-spin text-[#c96442]"
                      strokeWidth={1.5}
                    />
                  ) : (
                    <File className="h-4 w-4 shrink-0 text-muted" strokeWidth={1.5} />
                  )}
                  <span className="truncate text-[14px] text-primary" title={item.file.name}>
                    {truncateMiddle(item.file.name, 48)}
                  </span>
                  <span className="ml-auto shrink-0 text-xs text-muted-dark">
                    {item.status === "uploading"
                      ? `${item.progress}%`
                      : item.status === "error"
                        ? t("upload.failed")
                        : t("upload.queued")}
                  </span>
                </div>

                {item.status === "uploading" && (
                  <>
                    <div
                      className="relative h-1.5 overflow-hidden rounded-full bg-border"
                      role="progressbar"
                      aria-valuenow={item.progress}
                      aria-valuemin={0}
                      aria-valuemax={100}
                    >
                      <div
                        className="h-full rounded-full bg-[#c96442] transition-all duration-300"
                        style={{ width: `${item.progress}%` }}
                      />
                    </div>
                    <div className="mt-2 flex justify-between text-xs text-muted-dark">
                      <span>{formatSpeed(item.speed)}</span>
                      <span>
                        {t("upload.eta")} {formatEta(item.eta)}
                      </span>
                    </div>
                  </>
                )}

                {item.status === "error" && (
                  <div className="flex items-center gap-2">
                    <span className="flex-1 truncate text-xs text-[#b53333]" title={item.error}>
                      {item.error}
                    </span>
                    <button
                      onClick={() => retryItem(item.id)}
                      className="flex items-center gap-1 rounded-lg border border-border px-2 py-1 text-xs text-muted transition-colors hover:text-primary"
                    >
                      <RotateCcw className="h-3 w-3" strokeWidth={1.5} />
                      {t("upload.retry")}
                    </button>
                    <button
                      onClick={() => removeItem(item.id)}
                      className="rounded-lg border border-border px-2 py-1 text-xs text-muted transition-colors hover:text-primary"
                      aria-label={t("upload.remove")}
                    >
                      <X className="h-3 w-3" strokeWidth={1.5} />
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* ============ RESULTS ============ */}
      {results.length > 0 && (
        <div
          className="rounded-3xl border border-border bg-surface p-8 shadow-[0_4px_24px_rgba(0,0,0,0.04)]"
          style={{ animation: "scale-in 0.25s ease-out" }}
        >
          <div className="mb-6 flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-full bg-[#5a8a4a1a] text-[#5a8a4a]">
                <Check className="h-4 w-4" strokeWidth={2} />
              </div>
              <span className="font-serif text-[22px] font-medium text-primary">
                {t("upload.complete")}
              </span>
            </div>
            <button
              onClick={reset}
              className="flex h-9 w-9 items-center justify-center rounded-lg border border-border bg-surface-light text-muted transition-colors hover:text-primary"
              aria-label={t("upload.close")}
            >
              <X className="h-4 w-4" strokeWidth={1.5} />
            </button>
          </div>

          <div className="space-y-8">
            {results.map((result) => (
              <div key={result.url} className="space-y-4">
                <div className="flex min-w-0 flex-wrap items-center gap-3 rounded-xl border border-border bg-background px-4 py-3">
                  <File className="h-4 w-4 shrink-0 text-muted" strokeWidth={1.5} />
                  <span className="truncate text-[15px] text-primary" title={result.filename}>
                    {truncateMiddle(result.filename, 48)}
                  </span>
                  {result.encrypted && (
                    <span className="flex items-center gap-1 rounded-full bg-[#c9644214] px-2 py-0.5 text-[11px] text-[#c96442]">
                      <Lock className="h-3 w-3" strokeWidth={1.5} />
                      {t("upload.encryptedBadge")}
                    </span>
                  )}
                  <span className="ml-auto shrink-0 text-xs text-muted-dark">
                    {formatBytes(result.size)}
                  </span>
                </div>

                {(result.expiresAt || typeof result.maxDownloads === "number") && (
                  <div className="flex flex-wrap gap-x-4 gap-y-1 px-1 text-xs text-muted-dark">
                    {result.expiresAt && (
                      <span>
                        {t("upload.expires")}: {formatDate(result.expiresAt)}
                      </span>
                    )}
                    {typeof result.maxDownloads === "number" && (
                      <span>
                        {result.maxDownloads} {t("upload.downloadsLeft")}
                      </span>
                    )}
                  </div>
                )}

                <CopyRow
                  label={t("upload.shareLink")}
                  value={result.url}
                  copyKey={`url:${result.url}`}
                  copied={copied}
                  onCopy={copyToClipboard}
                  ariaLabel={t("upload.copyLink")}
                />

                <CopyRow
                  label={result.encrypted ? t("upload.decryptCmd") : t("upload.downloadCmd")}
                  value={
                    result.encrypted
                      ? `curl -H "X-Decrypt-Password: <password>" ${result.url} -o ${shellQuote(result.filename)}`
                      : `curl ${result.url} -o ${shellQuote(result.filename)}`
                  }
                  copyKey={`curl:${result.url}`}
                  copied={copied}
                  onCopy={copyToClipboard}
                  ariaLabel={result.encrypted ? t("upload.copyDecrypt") : t("upload.copyCurl")}
                />

                {result.deleteUrl && (
                  <CopyRow
                    label={t("upload.deleteCmd")}
                    value={`curl -X DELETE ${result.deleteUrl}`}
                    copyKey={`delete:${result.url}`}
                    copied={copied}
                    onCopy={copyToClipboard}
                    ariaLabel={t("upload.copyDelete")}
                  />
                )}

                <div>
                  <button
                    onClick={() => setShowQr((v) => (v === result.url ? null : result.url))}
                    className="flex items-center gap-2 rounded-lg px-2 py-1 text-[13px] text-muted transition-colors hover:text-primary"
                    aria-expanded={showQr === result.url}
                  >
                    <QrCode className="h-4 w-4" strokeWidth={1.5} />
                    {showQr === result.url ? t("upload.qrHide") : t("upload.qrShow")}
                  </button>

                  {showQr === result.url && (
                    <div className="mt-3 flex justify-center rounded-xl border border-border bg-white p-4">
                      <img
                        src={`${API_BASE}/qr?url=${encodeURIComponent(result.url)}&size=220`}
                        alt={t("upload.qrAlt")}
                        width={220}
                        height={220}
                        loading="lazy"
                      />
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>

          <button onClick={reset} className="btn btn-sand mt-8 w-full">
            {t("upload.another")}
          </button>
        </div>
      )}

      {/* ============ HISTORY ============ */}
      {history.length > 0 && (
        <div className="rounded-3xl border border-border bg-surface p-6">
          <div className="mb-4 flex items-center justify-between">
            <span className="font-serif text-[18px] font-medium text-primary">
              {t("upload.history")}
            </span>
            <button
              onClick={clearHistory}
              className="rounded-lg px-2 py-1 text-[13px] text-muted transition-colors hover:text-primary"
            >
              {t("upload.historyClear")}
            </button>
          </div>

          <ul className="divide-y divide-border">
            {history.map((entry) => (
              <li key={`${entry.url}-${entry.at}`} className="flex min-w-0 items-center gap-3 py-3">
                <File className="h-4 w-4 shrink-0 text-muted" strokeWidth={1.5} />
                <div className="min-w-0 flex-1">
                  <a
                    href={entry.url}
                    className="block truncate text-[14px] text-primary hover:underline"
                    title={entry.filename}
                  >
                    {truncateMiddle(entry.filename, 40)}
                  </a>
                  <span className="text-xs text-muted-dark">
                    {formatBytes(entry.size)}
                    {entry.expiresAt
                      ? ` · ${t("upload.expires")} ${formatDate(entry.expiresAt)}`
                      : ""}
                  </span>
                </div>

                <button
                  onClick={() => copyToClipboard(entry.url, `hist:${entry.url}`)}
                  className="shrink-0 rounded-lg border border-border p-2 text-muted transition-colors hover:text-primary"
                  aria-label={t("upload.copyLink")}
                >
                  {copied === `hist:${entry.url}` ? (
                    <Check className="h-3.5 w-3.5 text-[#5a8a4a]" strokeWidth={2} />
                  ) : (
                    <Copy className="h-3.5 w-3.5" strokeWidth={1.5} />
                  )}
                </button>

                {entry.deleteUrl && (
                  <button
                    onClick={() => deleteUpload(entry)}
                    disabled={deleting === entry.url}
                    className="shrink-0 rounded-lg border border-border p-2 text-muted transition-colors hover:text-[#b53333] disabled:opacity-50"
                    aria-label={t("upload.deleteNow")}
                  >
                    {deleting === entry.url ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.5} />
                    ) : (
                      <Trash2 className="h-3.5 w-3.5" strokeWidth={1.5} />
                    )}
                  </button>
                )}
              </li>
            ))}
          </ul>

          <p className="mt-4 text-[12px] text-muted-dark">{t("upload.historyNote")}</p>
        </div>
      )}
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
