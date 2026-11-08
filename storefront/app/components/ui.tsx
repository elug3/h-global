import { useState, type ReactNode } from "react";
import { t } from "../i18n/ko";

// Line icons (24px grid, stroke 1.8), the same set as the Figma draft.
const paths = {
  search: "M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14ZM20 20l-4-4",
  user: "M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8ZM4 21c0-4.4 3.6-8 8-8s8 3.6 8 8",
  back: "m15 18-6-6 6-6",
  close: "M18 6 6 18M6 6l12 12",
  chev: "m9 18 6-6-6-6",
  clock: "M12 3.5a8.5 8.5 0 1 0 0 17 8.5 8.5 0 0 0 0-17ZM12 7.5V12l3 2",
  bell: "M6 16V11a6 6 0 1 1 12 0v5l1.5 2h-15ZM10 20a2 2 0 0 0 4 0",
  play: "M8 5v14l11-7z",
  check: "m5 12.5 4.5 4.5L19 7.5",
  plus: "M12 5v14M5 12h14",
  minus: "M5 12h14",
  star: "m12 3.5 2.6 5.3 5.9.9-4.3 4.1 1 5.8L12 16.9l-5.2 2.7 1-5.8-4.3-4.1 5.9-.9z",
  link: "M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1",
  receipt: "M6 3h12v18l-3-2-3 2-3-2-3 2V3ZM9 8h6M9 12h6",
} as const;
export type IconName = keyof typeof paths;

export function Icon({ name, size = 22, filled = false, stroke = 1.8 }: { name: IconName; size?: number; filled?: boolean; stroke?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill={filled ? "currentColor" : "none"} stroke="currentColor"
      strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={paths[name]} />
    </svg>
  );
}

/** The red LIVE label. Red is used for the live state and nothing else. */
export function LivePill({ size = "m", label = "LIVE" }: { size?: "xs" | "s" | "m"; label?: string }) {
  return (
    <span className={`live live-${size}`}>
      <i />
      {label}
    </span>
  );
}

/** "No.4" badge. Numbers never change once listed. */
export function NumberBadge({ n, size = "m" }: { n: number; size?: "xs" | "s" | "m" }) {
  return (
    <span className={`num num-${size}`}>
      <small>No.</small>
      {n}
    </span>
  );
}

/** Round color swatch; two hex values make it two-tone. */
export function Swatch({ hex1, hex2, size = 14 }: { hex1?: string | null; hex2?: string | null; size?: number }) {
  if (!hex1) return null;
  const bg = hex2 ? `linear-gradient(135deg, ${hex1} 50%, ${hex2} 50%)` : hex1;
  return <span className="sw" style={{ width: size, height: size, background: bg }} />;
}

export function Stepper({ value, onChange, min = 1, max = 99, label }: { value: number; onChange: (v: number) => void; min?: number; max?: number; label: string }) {
  return (
    <div className="stepper" role="group" aria-label={label}>
      <button type="button" aria-label="-" disabled={value <= min} onClick={() => onChange(Math.max(min, value - 1))}>
        <Icon name="minus" size={16} />
      </button>
      <b aria-live="polite">{value}</b>
      <button type="button" aria-label="+" disabled={value >= max} onClick={() => onChange(Math.min(max, value + 1))}>
        <Icon name="plus" size={16} />
      </button>
    </div>
  );
}

export function ErrorBanner({ text }: { text?: string }) {
  if (!text) return null;
  return (
    <div className="err" role="alert">
      {text}
    </div>
  );
}

const statusClass: Record<string, string> = {
  pending_payment: "t-wait",
  paid: "t-paid",
  inspecting: "t-insp",
  customs_check: "t-insp",
  shipped: "t-ship",
  delivered: "t-done",
  completed: "t-done",
  canceled: "t-cancel",
};

export function OrderStatus({ status }: { status: string }) {
  return <span className={`stag ${statusClass[status] ?? ""}`}>{t.orders.status[status] ?? status}</span>;
}

export function CopyButton({ text, children }: { text: string; children?: ReactNode }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className="btn btn-sec btn-xs"
      onClick={() => {
        void navigator.clipboard?.writeText(text).then(() => {
          setDone(true);
          setTimeout(() => setDone(false), 1500);
        });
      }}
    >
      {done ? t.app.copied : (children ?? t.app.copy)}
    </button>
  );
}

/** A product photo, or a tinted empty square. */
export function Photo({ src, size, radius = 12, alt = "" }: { src?: string; size?: number | string; radius?: number; alt?: string }) {
  const style = { width: size, height: size, borderRadius: radius };
  return src ? <img className="photo" src={src} alt={alt} style={style} loading="lazy" /> : <span className="photo photo-empty" style={style} />;
}
