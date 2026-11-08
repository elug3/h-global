import createClient from "openapi-fetch";
import type { components, paths } from "./schema";
import { t } from "../i18n/ko";

// One client for hg-server's /studio-api. Requests go to the same origin;
// Vite (development) or Caddy (production) forwards them to hg-server, so the
// HttpOnly staff session cookie is sent automatically.
export const api = createClient<paths>({ baseUrl: "", credentials: "same-origin" });

type S = components["schemas"];
export type Staff = S["Staff"];
export type Show = S["Show"];
export type StudioShow = S["StudioShow"];
export type StudioItem = S["StudioItem"];
export type Product = S["Product"];
export type Variant = S["Variant"];
export type Capture = S["Capture"];
export type Ingest = S["Ingest"];
export type RelayStatus = S["RelayStatus"];
export type FrameInfo = S["FrameInfo"];
export type Suggestion = S["Suggestion"];
export type NewOption = S["NewOption"];
export type NewShow = S["NewShow"];
export type ShowPatch = S["ShowPatch"];
export type QuickListInput = S["QuickListInput"];
export type Media = S["Media"];
export type ProductPatch = S["ProductPatch"];
export type VariantPatch = S["VariantPatch"];
export type NewProduct = S["NewProduct"];
export type Order = S["Order"];
export type OrderLine = S["Line"];
export type OrderHistory = S["History"];
export type OrderMedia = S["OrderMedia"];
export type OrderCounts = S["OrderCounts"];
export type OrderStatus = Order["status"];
export type ShowSales = S["ShowSales"];
export type ItemSales = S["ItemSales"];
export type NewStaff = S["NewStaff"];
export type StaffPatch = S["StaffPatch"];

/** An error from hg-server, with its stable code and a Korean message. */
export class ApiError extends Error {
  constructor(
    public code: string,
    public status: number,
    serverMessage?: string,
  ) {
    super(t.errors[code] ?? (code.endsWith("_not_found") ? t.errors.not_found : undefined) ?? serverMessage ?? t.errors.generic);
  }
}

/** Unwraps an openapi-fetch result: returns data or throws ApiError. */
export async function call<T>(
  p: Promise<{ data?: T; error?: unknown; response: Response }>,
): Promise<T> {
  let res;
  try {
    res = await p;
  } catch {
    throw new ApiError("network", 0, t.errors.network);
  }
  if (res.error !== undefined || !res.response.ok) {
    const e = (res.error ?? {}) as { error?: string; message?: string };
    throw new ApiError(e.error ?? "generic", res.response.status, e.message);
  }
  return res.data as T;
}

/** Sends a multipart upload (openapi-fetch handles JSON only). */
export async function upload<T>(url: string, file: Blob, fields: Record<string, string> = {}): Promise<T> {
  const form = new FormData();
  for (const [k, v] of Object.entries(fields)) form.append(k, v);
  form.append("file", file, file instanceof File ? file.name : "image");
  let res: Response;
  try {
    res = await fetch(url, { method: "POST", body: form, credentials: "same-origin" });
  } catch {
    throw new ApiError("network", 0, t.errors.network);
  }
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(body.error ?? "generic", res.status, body.message);
  return body as T;
}

/** Message to show for any thrown value. */
export function errorText(e: unknown): string {
  if (e instanceof Error) return e.message;
  return t.errors.generic;
}
