// LanguageSwitcher — 语言切换下拉菜单
import { useState, useRef, useEffect } from "react";
import { Globe } from "lucide-react";
import { languages, type Lang, detectBrowserLang } from "../i18n/translations";

export default function LanguageSwitcher() {
  const [open, setOpen] = useState(false);
  const [current, setCurrent] = useState<Lang>("en");
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const saved = localStorage.getItem("lang") as Lang | null;
    if (saved && saved in languages) {
      setCurrent(saved);
    } else {
      setCurrent(detectBrowserLang());
    }
  }, []);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const switchLang = (lang: Lang) => {
    setCurrent(lang);
    localStorage.setItem("lang", lang);
    setOpen(false);
    // Reload to re-run the i18n DOM swap from BaseLayout
    window.location.reload();
  };

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex cursor-pointer items-center gap-1 rounded-md px-2 py-1.5 text-sm text-muted transition-colors hover:text-primary"
        aria-label="Switch language"
      >
        <Globe className="h-4 w-4" />
        <span className="hidden sm:inline">{languages[current]}</span>
      </button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 min-w-[120px] overflow-hidden rounded-lg border border-border bg-surface shadow-lg">
          {(Object.entries(languages) as [Lang, string][]).map(([code, name]) => (
            <button
              key={code}
              onClick={() => switchLang(code)}
              className={`flex w-full cursor-pointer items-center px-3 py-2 text-left text-sm transition-colors ${
                current === code
                  ? "bg-surface-light text-primary"
                  : "text-muted hover:bg-surface-light hover:text-primary"
              }`}
            >
              {name}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
