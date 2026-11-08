import type { Order, OrderHistory } from "../api/client";
import { t } from "../i18n/ko";

/** Every order status, in lifecycle order. */
export const ORDER_STATUSES = [
  "pending_payment",
  "paid",
  "inspecting",
  "customs_check",
  "shipped",
  "delivered",
  "completed",
  "canceled",
] as const;

/** Statuses where staff have work to do (inspect, check customs, ship). */
export const TODO_STATUSES = ["paid", "inspecting", "customs_check"] as const;

export function OrderStatusTag({ status }: { status: string }) {
  return <span className={`tag tag-o-${status}`}>{t.orderStatus[status] ?? status}</span>;
}

/** Korea personal customs clearance code: P + 12 digits. */
export function validPCCC(s: string | undefined): boolean {
  return !!s && /^P\d{12}$/.test(s);
}

/** "퀼팅 체인백 외 2건" */
export function linesSummary(o: Order): string {
  if (o.lines.length === 0) return "";
  const first = o.lines[0].title;
  return o.lines.length > 1 ? first + t.orders.andMore(o.lines.length - 1) : first;
}

// Notes hg-server writes itself, shown in Korean.
const serverNotes: Record<string, string> = {
  "customs code checked": t.orders.historyCustoms,
  "payment deadline passed": "결제 기한이 지나 자동 취소",
};

/** One history row as Korean text: what changed, and the note. */
export function historyText(h: OrderHistory): { what: string; note: string } {
  let what: string;
  if (!h.from) what = t.orders.historyCreated;
  else if (h.from === h.to) what = serverNotes[h.note] ?? t.orderStatus[h.to] ?? h.to;
  else what = `${t.orderStatus[h.from] ?? h.from} → ${t.orderStatus[h.to] ?? h.to}`;
  const note = h.from === h.to && serverNotes[h.note] ? "" : (serverNotes[h.note] ?? h.note);
  return { what, note };
}
