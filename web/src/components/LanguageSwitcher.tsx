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
        className="flex cursor-pointer items-center gap-2 text-primary transition-opacity hover:opacity-50"
        aria-label="Switch language"
      >
        <Globe className="h-4 w-4" strokeWidth={1.5} />
        <span className="font-mono-track hidden text-xs sm:inline">{languages[current]}</span>
      </button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-2 min-w-[140px] border border-border bg-background">
          {(Object.entries(languages) as [Lang, string][]).map(([code, name]) => (
            <button
              key={code}
              onClick={() => switchLang(code)}
              className={`font-mono-track block w-full cursor-pointer px-4 py-2.5 text-left text-xs transition-colors ${
                current === code
                  ? "bg-[rgba(255,255,255,0.08)] text-primary"
                  : "text-muted hover:bg-[rgba(255,255,255,0.05)] hover:text-primary"
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
