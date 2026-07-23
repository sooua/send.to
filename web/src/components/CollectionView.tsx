// CollectionView — the landing page one link opens when it stands for several
// files. The server routes GET /c/{token} here for HTML clients and answers the
// same path with JSON for everything else, which is where this gets its data.
import { useState, useCallback, useEffect } from "react";
import {
  File as FileIcon,
  Download,
  Copy,
  Check,
  Package,
  AlertCircle,
  Loader2,
  Lock,
} from "lucide-react";
import { translations, defaultLang, type Lang } from "../i18n/translations";

interface CollectionFile {
  url: string;
  filename: string;
  size: number;
  content_type?: string;
  encrypted: boolean;
}

interface Collection {
  url: string;
  archive_url: string;
  name?: string;
  files: CollectionFile[];
  total_size: number;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "—";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through to the textarea fallback
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
    const w = window as unknown as { __LANG__?: string };
    const l = w.__LANG__ || localStorage.getItem("lang") || defaultLang;
    if (l in translations) setLang(l as Lang);
  }, []);
  return useCallback(
    (key: string): string => translations[lang]?.[key] ?? translations.en[key] ?? key,
    [lang]
  );
}

export default function CollectionView() {
  const t = useI18n();
  const [collection, setCollection] = useState<Collection | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    const controller = new AbortController();

    // The page is served at its own URL, so asking for the same path with
    // Accept: application/json is all it takes.
    fetch(window.location.pathname, {
      headers: { Accept: "application/json" },
      signal: controller.signal,
    })
      .then((res) => {
        if (!res.ok) throw new Error(String(res.status));
        return res.json();
      })
      .then((data: Collection) => setCollection(data))
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(t("collection.unavailable"));
      });

    return () => controller.abort();
  }, [t]);

  const copy = useCallback(async (value: string) => {
    if (await copyText(value)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }, []);

  if (error) {
    return (
      <div className="rounded-2xl border border-border bg-surface p-8 text-center">
        <AlertCircle className="mx-auto mb-4 h-8 w-8 text-muted" strokeWidth={1.5} />
        <p className="mb-1 text-[17px] text-primary">{error}</p>
        <p className="text-[13px] text-muted">{t("collection.unavailableHint")}</p>
      </div>
    );
  }

  if (!collection) {
    return (
      <div className="flex items-center justify-center gap-3 rounded-2xl border border-border bg-surface p-12 text-muted">
        <Loader2 className="h-4 w-4 animate-spin" strokeWidth={1.5} />
        <span className="text-[14px]">{t("collection.loading")}</span>
      </div>
    );
  }

  return (
    <div className="rounded-2xl border border-border bg-surface p-6 sm:p-8">
      <div className="mb-6 flex items-start gap-4">
        <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl border border-border bg-surface-light">
          <Package className="h-5 w-5 text-muted" strokeWidth={1.5} />
        </div>
        <div className="min-w-0">
          <h1 className="truncate text-[19px] font-medium text-primary">
            {collection.name || t("collection.title")}
          </h1>
          <p className="text-[13px] text-muted">
            {collection.files.length} {t("collection.files")} · {formatBytes(collection.total_size)}
          </p>
        </div>
      </div>

      <a href={collection.archive_url} className="btn btn-primary mb-6 w-full justify-center">
        <Download className="h-4 w-4" strokeWidth={1.75} />
        <span>{t("collection.downloadAll")}</span>
      </a>

      <ul className="mb-6 divide-y divide-border overflow-hidden rounded-xl border border-border">
        {collection.files.map((file) => (
          <li key={file.url}>
            <a
              href={file.url}
              download={file.filename}
              className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-surface-light"
            >
              {file.encrypted ? (
                <Lock className="h-4 w-4 shrink-0 text-muted" strokeWidth={1.5} />
              ) : (
                <FileIcon className="h-4 w-4 shrink-0 text-muted" strokeWidth={1.5} />
              )}
              <span className="min-w-0 flex-1 truncate text-[14px] text-primary">
                {file.filename}
              </span>
              <span className="shrink-0 font-mono text-[12px] text-muted">
                {formatBytes(file.size)}
              </span>
            </a>
          </li>
        ))}
      </ul>

      <label className="mb-2 block text-[13px] font-medium text-primary">
        {t("upload.shareLink")}
      </label>
      <div className="flex gap-2">
        <code
          className="flex-1 truncate rounded-xl border border-border bg-background px-4 py-3 font-mono text-[13px] text-primary"
          title={collection.url}
        >
          {collection.url}
        </code>
        <button
          onClick={() => copy(collection.url)}
          className="flex w-12 shrink-0 items-center justify-center rounded-xl border border-border bg-surface-light text-muted transition-colors hover:text-primary"
          aria-label={t("upload.copyLink")}
        >
          {copied ? (
            <Check className="h-4 w-4 text-[#5a8a4a]" strokeWidth={2} />
          ) : (
            <Copy className="h-4 w-4" strokeWidth={1.5} />
          )}
        </button>
      </div>
    </div>
  );
}
