const wonFmt = new Intl.NumberFormat("ko-KR");

/** 149000 → "₩149,000" */
export function won(n: number): string {
  return "₩" + wonFmt.format(n);
}

const dateTimeFmt = new Intl.DateTimeFormat("ko-KR", {
  month: "long",
  day: "numeric",
  weekday: "short",
  hour: "2-digit",
  minute: "2-digit",
});

/** "11월 8일 (토) 오후 08:00" */
export function dateTime(iso: string | null | undefined): string {
  return iso ? dateTimeFmt.format(new Date(iso)) : "";
}

/** Seconds → "00:42:10" */
export function clock(totalSec: number): string {
  const s = Math.max(0, Math.floor(totalSec));
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(Math.floor(s / 3600))}:${pad(Math.floor((s % 3600) / 60))}:${pad(s % 60)}`;
}

/** ISO time → value for <input type="datetime-local"> in local time. */
export function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  const off = d.getTimezoneOffset() * 60_000;
  return new Date(d.getTime() - off).toISOString().slice(0, 16);
}

/** <input type="datetime-local"> value → ISO time, or undefined. */
export function fromLocalInput(v: string): string | undefined {
  return v ? new Date(v).toISOString() : undefined;
}

/** A random key for Idempotency-Key headers. */
export function newKey(): string {
  return crypto.randomUUID();
}
