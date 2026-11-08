import createClient from "openapi-fetch";
import { redirect } from "react-router";
import { t } from "../i18n/ko";
import type { components, paths } from "./schema";

type S = components["schemas"];
export type Show = S["Show"];
export type ShowView = S["ShowView"];
export type ItemView = S["ItemView"];
export type Product = S["PublicProduct"];
export type Variant = S["PublicVariant"];
export type Option = S["Option"];
export type Broadcast = S["Broadcast"];
export type Customer = S["Customer"];
export type Checkout = S["Checkout"];
export type Shipping = S["Shipping"];
export type Order = S["Order"];
export type Payment = S["Payment"];
export type PayResult = S["PayResult"];

/**
 * In the browser: same-origin requests to /api (the dev server or the TLS
 * proxy forwards them to hg-server), so the HttpOnly session cookie is set
 * and sent by the browser itself.
 */
export const api = createClient<paths>({ baseUrl: "", credentials: "same-origin" });

/**
 * On the server (loaders): requests go straight to hg-server, carrying the
 * visitor's cookie so signed-in pages render on the server too.
 */
export function serverApi(request: Request) {
  const base = process.env.HG_API_URL ?? "http://localhost:8080";
  const cookie = request.headers.get("Cookie");
  return createClient<paths>({ baseUrl: base, headers: cookie ? { Cookie: cookie } : {} });
}

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
export async function call<T>(p: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
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

/** Like call, for loaders: 401 sends the visitor to sign in and back; 404 is a 404 page. */
export async function load<T>(request: Request, p: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
  try {
    return await call(p);
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) {
      const u = new URL(request.url);
      throw redirect(`/login?next=${encodeURIComponent(u.pathname + u.search)}`);
    }
    if (e instanceof ApiError && e.status === 404) throw new Response(t.errors.not_found, { status: 404 });
    throw e;
  }
}

/** Message to show for any thrown value. */
export function errorText(e: unknown): string {
  return e instanceof Error ? e.message : t.errors.generic;
}

/** Absolute site origin for share tags, honoring the TLS proxy's headers. */
export function originOf(request: Request): string {
  const u = new URL(request.url);
  const proto = request.headers.get("X-Forwarded-Proto") ?? u.protocol.replace(":", "");
  const host = request.headers.get("X-Forwarded-Host") ?? request.headers.get("Host") ?? u.host;
  return `${proto}://${host}`;
}
