// Dates are shown in Korea time on the server and in the browser alike, so the
// server-rendered HTML matches what the browser renders (buyers are in Korea).
const TZ = "Asia/Seoul";
const wonFmt = new Intl.NumberFormat("ko-KR");

/** 149000 → "₩149,000" */
export function won(n: number): string {
  return "₩" + wonFmt.format(n);
}

const dateTimeFmt = new Intl.DateTimeFormat("ko-KR", {
  timeZone: TZ,
  month: "long",
  day: "numeric",
  weekday: "short",
  hour: "numeric",
  minute: "2-digit",
});
const dateFmt = new Intl.DateTimeFormat("ko-KR", { timeZone: TZ, month: "long", day: "numeric" });
const timeFmt = new Intl.DateTimeFormat("ko-KR", { timeZone: TZ, hour: "2-digit", minute: "2-digit", hourCycle: "h23" });

/** "11월 8일 (일) 오후 8:00" */
export function dateTime(iso: string | null | undefined): string {
  return iso ? dateTimeFmt.format(new Date(iso)) : "";
}

/** "11월 8일" */
export function date(iso: string | null | undefined): string {
  return iso ? dateFmt.format(new Date(iso)) : "";
}

/** "14:02" */
export function time(iso: string | null | undefined): string {
  return iso ? timeFmt.format(new Date(iso)) : "";
}

/** Seconds → "12:34" or "1:02:03". */
export function offset(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s % 60)}` : `${m}:${pad(s % 60)}`;
}

/** Korea's personal customs clearance code: P + 12 digits. */
export function validPCCC(s: string | undefined | null): boolean {
  return !!s && /^P\d{12}$/.test(s);
}

/** A YouTube thumbnail for a video ID (shown before the player loads). */
export function ytThumb(id: string): string {
  return `https://i.ytimg.com/vi/${id}/hqdefault.jpg`;
}
