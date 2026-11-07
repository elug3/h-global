import { useState } from "react";
import { t } from "../i18n/ko";
import { Icon } from "./Icon";

/** The red LIVE label. Red is used for the live state and nothing else. */
export function LivePill({ size = "m" }: { size?: "s" | "m" }) {
  return (
    <span className={`live live-${size}`}>
      <i />
      LIVE
    </span>
  );
}

/** "No.4" badge. Numbers never change once a show is live. */
export function NumberBadge({ n, size = "m" }: { n: number; size?: "xs" | "s" | "m" }) {
  return (
    <span className={`num num-${size}`}>
      <small>No.</small>
      {n}
    </span>
  );
}

export function StatusTag({ status }: { status: string }) {
  if (status === "live") return <LivePill size="s" />;
  return <span className={`tag tag-${status}`}>{t.status[status] ?? status}</span>;
}

/** Round color swatch; two hex values make it two-tone. */
export function Swatch({ hex1, hex2, size = 14 }: { hex1?: string | null; hex2?: string | null; size?: number }) {
  if (!hex1) return null;
  const bg = hex2 ? `linear-gradient(135deg, ${hex1} 50%, ${hex2} 50%)` : hex1;
  return <span className="swatch" style={{ width: size, height: size, background: bg }} />;
}

export function Stepper({
  value,
  onChange,
  min = 0,
  max = 100000,
  format = String,
  label,
}: {
  value: number;
  onChange: (v: number) => void;
  min?: number;
  max?: number;
  format?: (v: number) => string;
  label: string;
}) {
  return (
    <div className="stepper" role="group" aria-label={label}>
      <button type="button" aria-label="-" disabled={value <= min} onClick={() => onChange(Math.max(min, value - 1))}>
        <Icon name="minus" size={16} />
      </button>
      <b aria-live="polite">{format(value)}</b>
      <button type="button" aria-label="+" disabled={value >= max} onClick={() => onChange(Math.min(max, value + 1))}>
        <Icon name="plus" size={16} />
      </button>
    </div>
  );
}

export function CopyButton({ text }: { text: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className="btn btn-secondary btn-sm"
      onClick={() => {
        void navigator.clipboard?.writeText(text).then(() => {
          setDone(true);
          setTimeout(() => setDone(false), 1500);
        });
      }}
    >
      <Icon name="copy" size={16} />
      {done ? t.app.copied : t.app.copy}
    </button>
  );
}

export function ErrorBanner({ text, onRetry }: { text?: string; onRetry?: () => void }) {
  if (!text) return null;
  return (
    <div className="banner banner-error" role="alert">
      <span>{text}</span>
      {onRetry && (
        <button className="btn btn-secondary btn-sm" onClick={onRetry}>
          {t.app.retry}
        </button>
      )}
    </div>
  );
}

export function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label className={"toggle" + (checked ? " on" : "")}>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <i aria-hidden="true" />
      <span>{label}</span>
    </label>
  );
}
