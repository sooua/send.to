// FilePreview — the landing page a share link opens in a browser.
//
// The server routes GET /{token}/{filename} here for HTML clients, then serves
// this page unchanged; everything below is derived client-side from the URL
// plus a HEAD request, which the server answers without spending a download
// from the file's Max-Downloads budget.
import { useState, useCallback, useEffect } from "react";
import {
  File as FileIcon,
  Download,
  Copy,
  Check,
  QrCode,
  Lock,
  Eye,
  AlertCircle,
  Loader2,
} from "lucide-react";
import { translations, defaultLang, type Lang } from "../i18n/translations";

interface FileInfo {
  token: string;
  filename: string;
  url: string;
  contentType: string;
  size: number;
  remainingDownloads: string;
  remainingDays: string;
  encrypted: boolean;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "—";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

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

/** Split /{token}/{filename} — or /get/{token}/{filename} — out of the path. */
function parseLocation(pathname: string): { token: string; filename: string } | null {
  const segments = pathname.replace(/^\/+/, "").split("/").filter(Boolean);
  const offset = ["download", "get", "inline"].includes(segments[0]) ? 1 : 0;

  if (segments.length < offset + 2) return null;

  return {
    token: segments[offset],
    filename: decodeURIComponent(segments.slice(offset + 1).join("/")),
  };
}

export default function FilePreview() {
  const t = useI18n();
  const [info, setInfo] = useState<FileInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [showQr, setShowQr] = useState(false);
  const [revealed, setRevealed] = useState(false);

  useEffect(() => {
    const parsed = parseLocation(window.location.pathname);
    if (!parsed) {
      setError(t("preview.badLink"));
      return;
    }

    const url = `${window.location.origin}/${parsed.token}/${encodeURIComponent(parsed.filename)}`;

    // HEAD does not consume a download, so opening the preview cannot burn the
    // recipient's only allowed fetch.
    fetch(url, { method: "HEAD" })
      .then((res) => {
        if (!res.ok) throw new Error(String(res.status));
        const contentType = res.headers.get("Content-Type") || "application/octet-stream";
        setInfo({
          token: parsed.token,
          filename: parsed.filename,
          url,
          contentType,
          size: Number(res.headers.get("Content-Length") || 0),
          remainingDownloads: res.headers.get("X-Remaining-Downloads") || "n/a",
          remainingDays: res.headers.get("X-Remaining-Days") || "n/a",
          // An encrypted upload is stored — and therefore reported — as text.
          encrypted: contentType.startsWith("text/plain") && parsed.filename.includes("."),
        });
      })
      .catch(() => setError(t("preview.unavailable")));
  }, [t]);

  const copy = useCallback(async (text: string, key: string) => {
    if (await copyText(text)) {
      setCopied(key);
      setTimeout(() => setCopied(null), 2000);
    }
  }, []);

  if (error) {
    return (
      <div className="rounded-3xl border border-border bg-surface p-12 text-center">
        <div className="mx-auto mb-6 flex h-14 w-14 items-center justify-center rounded-2xl bg-[#b533330a] text-[#b53333]">
          <AlertCircle className="h-6 w-6" strokeWidth={1.5} />
        </div>
        <p className="font-serif mb-2 text-[24px] font-medium text-primary">{error}</p>
        <p className="mb-8 text-[15px] text-muted">{t("preview.unavailableHint")}</p>
        <a href="/" className="btn btn-primary">
          {t("error.backHome")}
        </a>
      </div>
    );
  }

  if (!info) {
    return (
      <div
        className="flex items-center justify-center rounded-3xl border border-border bg-surface p-16"
        role="status"
        aria-live="polite"
      >
        <Loader2 className="h-6 w-6 animate-spin text-[#c96442]" strokeWidth={1.5} />
      </div>
    );
  }

  const isImage = info.contentType.startsWith("image/");
  const isVideo = info.contentType.startsWith("video/");
  const isAudio = info.contentType.startsWith("audio/");
  const isMedia = isImage || isVideo || isAudio;

  // Rendering media issues a real GET, which does spend a download. When the
  // upload has a limited budget, make that the recipient's explicit choice
  // rather than a side effect of opening the link.
  const limited = info.remainingDownloads !== "n/a";
  const showMedia = isMedia && (!limited || revealed);

  return (
    <div className="rounded-3xl border border-border bg-surface p-8 shadow-[0_4px_24px_rgba(0,0,0,0.04)]">
      <div className="mb-6 flex min-w-0 flex-wrap items-center gap-3">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-[#c9644214] text-[#c96442]">
          <FileIcon className="h-5 w-5" strokeWidth={1.5} />
        </div>
        <div className="min-w-0 flex-1">
          <h1
            className="font-serif truncate text-[24px] font-medium text-primary"
            title={info.filename}
          >
            {info.filename}
          </h1>
          <p className="text-[13px] text-muted">
            {formatBytes(info.size)} · <span className="font-mono">{info.contentType}</span>
          </p>
        </div>
      </div>

      <div className="mb-6 flex flex-wrap gap-x-6 gap-y-2 rounded-xl border border-border bg-background px-4 py-3 text-[13px] text-muted">
        <span>
          {t("preview.downloadsLeft")}:{" "}
          <span className="text-primary">
            {limited ? info.remainingDownloads : t("upload.maxDownloadsUnlimited")}
          </span>
        </span>
        <span>
          {t("preview.daysLeft")}:{" "}
          <span className="text-primary">
            {info.remainingDays === "n/a" ? t("upload.expiryNever") : info.remainingDays}
          </span>
        </span>
      </div>

      {isMedia && (
        <div className="mb-6 overflow-hidden rounded-xl border border-border bg-background">
          {showMedia ? (
            <>
              {isImage && (
                <img
                  src={`/inline/${info.token}/${encodeURIComponent(info.filename)}`}
                  alt={info.filename}
                  className="mx-auto max-h-[520px] w-auto max-w-full"
                />
              )}
              {isVideo && (
                <video
                  src={`/inline/${info.token}/${encodeURIComponent(info.filename)}`}
                  controls
                  className="mx-auto max-h-[520px] w-full"
                >
                  {/* Uploaded media carries no caption data; an empty track
                      declares that explicitly rather than omitting it. */}
                  <track kind="captions" />
                </video>
              )}
              {isAudio && (
                <audio
                  src={`/inline/${info.token}/${encodeURIComponent(info.filename)}`}
                  controls
                  className="w-full p-4"
                >
                  <track kind="captions" />
                </audio>
              )}
            </>
          ) : (
            <button
              onClick={() => setRevealed(true)}
              className="flex w-full flex-col items-center gap-2 px-6 py-12 text-muted transition-colors hover:text-primary"
            >
              <Eye className="h-6 w-6" strokeWidth={1.5} />
              <span className="text-[15px]">{t("preview.reveal")}</span>
              <span className="text-[12px] text-muted-dark">{t("preview.revealNote")}</span>
            </button>
          )}
        </div>
      )}

      <a
        href={`/${info.token}/${encodeURIComponent(info.filename)}`}
        className="btn btn-primary mb-6 w-full justify-center"
        download={info.filename}
      >
        <Download className="h-4 w-4" strokeWidth={1.75} />
        <span>{t("preview.download")}</span>
      </a>

      <div className="space-y-4">
        <div>
          <label className="mb-2 block text-[13px] font-medium text-primary">
            {t("upload.shareLink")}
          </label>
          <div className="flex gap-2">
            <code
              className="flex-1 truncate rounded-xl border border-border bg-background px-4 py-3 font-mono text-[13px] text-primary"
              title={info.url}
            >
              {info.url}
            </code>
            <button
              onClick={() => copy(info.url, "url")}
              className="flex w-12 shrink-0 items-center justify-center rounded-xl border border-border bg-surface-light text-muted transition-colors hover:text-primary"
              aria-label={t("upload.copyLink")}
            >
              {copied === "url" ? (
                <Check className="h-4 w-4 text-[#5a8a4a]" strokeWidth={2} />
              ) : (
                <Copy className="h-4 w-4" strokeWidth={1.5} />
              )}
            </button>
          </div>
        </div>

        <div>
          <label className="mb-2 block text-[13px] font-medium text-primary">
            {t("upload.downloadCmd")}
          </label>
          <div className="flex gap-2">
            <code className="flex-1 overflow-x-auto whitespace-nowrap rounded-xl border border-border bg-background px-4 py-3 font-mono text-[13px] text-primary">
              curl {info.url} -o {shellQuote(info.filename)}
            </code>
            <button
              onClick={() => copy(`curl ${info.url} -o ${shellQuote(info.filename)}`, "curl")}
              className="flex w-12 shrink-0 items-center justify-center rounded-xl border border-border bg-surface-light text-muted transition-colors hover:text-primary"
              aria-label={t("upload.copyCurl")}
            >
              {copied === "curl" ? (
                <Check className="h-4 w-4 text-[#5a8a4a]" strokeWidth={2} />
              ) : (
                <Copy className="h-4 w-4" strokeWidth={1.5} />
              )}
            </button>
          </div>
        </div>

        {info.encrypted && (
          <p className="flex items-start gap-2 rounded-xl border border-border bg-background px-4 py-3 text-[13px] text-muted">
            <Lock className="mt-0.5 h-3.5 w-3.5 shrink-0" strokeWidth={1.5} />
            <span>{t("preview.maybeEncrypted")}</span>
          </p>
        )}

        <div>
          <button
            onClick={() => setShowQr((v) => !v)}
            className="flex items-center gap-2 rounded-lg px-2 py-1 text-[13px] text-muted transition-colors hover:text-primary"
            aria-expanded={showQr}
          >
            <QrCode className="h-4 w-4" strokeWidth={1.5} />
            {showQr ? t("upload.qrHide") : t("upload.qrShow")}
          </button>

          {showQr && (
            <div className="mt-3 flex justify-center rounded-xl border border-border bg-white p-4">
              <img
                src={`/qr?url=${encodeURIComponent(info.url)}&size=220`}
                alt={t("upload.qrAlt")}
                width={220}
                height={220}
                loading="lazy"
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
