import { useState } from "react";
import { Link, useLoaderData, useNavigate, useRevalidator } from "react-router";
import { api, call, errorText, load, serverApi, type Order } from "../api/client";
import { ErrorBanner, Icon, OrderStatus, Photo } from "../components/ui";
import { t } from "../i18n/ko";
import { date, won } from "../lib/format";
import type { Route } from "./+types/orders";

export async function loader({ request }: Route.LoaderArgs) {
  const api = serverApi(request);
  const [me, orders] = await Promise.all([
    load(request, api.GET("/api/me")),
    load(request, api.GET("/api/orders", { params: { query: { limit: 100 } } })),
  ]);
  return { me, orders: orders.items };
}

export function meta() {
  return [{ title: `${t.orders.title} — ${t.app.name}` }, { name: "robots", content: "noindex" }];
}

const DONE = ["completed", "canceled"];

/** My orders (board 13). */
export default function Orders() {
  const { me, orders } = useLoaderData<typeof loader>();
  const navigate = useNavigate();
  const [tab, setTab] = useState<"all" | "active" | "done">("all");
  const counts = {
    all: orders.length,
    active: orders.filter((o) => !DONE.includes(o.status)).length,
    done: orders.filter((o) => DONE.includes(o.status)).length,
  };
  const shown = orders.filter((o) => tab === "all" || (tab === "done") === DONE.includes(o.status));

  async function logout() {
    await api.POST("/api/auth/logout").catch(() => undefined);
    navigate("/", { replace: true });
  }

  return (
    <main className="page narrow">
      <div className="me">
        <div>
          <b>{me.name || me.email}</b>
          <span>{me.email}</span>
        </div>
        <button type="button" className="linkbtn mutedl" onClick={() => void logout()}>
          {t.nav.logout}
        </button>
      </div>
      <h1 className="page-title">{t.orders.title}</h1>
      <div className="chips tabs2" role="tablist">
        {(["all", "active", "done"] as const).map((k) => (
          <button key={k} type="button" role="tab" aria-selected={tab === k} className={"chip" + (tab === k ? " on" : "")} onClick={() => setTab(k)}>
            {t.orders.tabs[k]} {counts[k]}
          </button>
        ))}
      </div>
      {shown.length === 0 ? (
        <p className="muted empty">{t.orders.empty}</p>
      ) : (
        <div className="olist" data-testid="orders">
          {shown.map((o) => (
            <OrderCard key={o.id} o={o} />
          ))}
        </div>
      )}
    </main>
  );
}

function OrderCard({ o }: { o: Order }) {
  const revalidator = useRevalidator();
  const [error, setError] = useState<string>();
  const line = o.lines[0];
  const more = o.lines.length > 1 ? ` 외 ${o.lines.length - 1}건` : "";

  async function run(confirmText: string, p: () => Promise<unknown>) {
    if (!confirm(confirmText)) return;
    setError(undefined);
    try {
      await p();
      await revalidator.revalidate();
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <div className="ocard">
      <div className="ohead">
        <span>
          {date(o.created_at)} · <span className="mono">{o.number}</span>
        </span>
        <OrderStatus status={o.status} />
      </div>
      <Link className="oi" to={`/orders/${o.id}`}>
        <Photo src={line?.photo_url} size={64} radius={8} />
        <span className="oit">
          <span className="oin">
            {line?.title}
            {more}
          </span>
          <span className="oio">
            {[line?.variant_label, line && t.orders.qty(line.qty), o.shipped_at && t.orders.tracking(o.carrier, o.tracking_number)].filter(Boolean).join(" · ")}
          </span>
          <b>{won(o.total_won)}</b>
        </span>
        <Icon name="chev" size={18} />
      </Link>
      {o.inspection_media.length > 0 && !DONE.includes(o.status) && <span className="note-i">{t.orders.inspectionReady}</span>}
      {o.status === "pending_payment" && (
        <div className="oact">
          {o.payment_method === "virtual_account" ? (
            <Link className="btn btn-sec btn-sm grow" to={`/orders/${o.id}/done`}>
              {t.orders.showAccount}
            </Link>
          ) : (
            <Link className="btn btn-sec btn-sm grow" to={`/orders/${o.id}`}>
              {t.orders.pay}
            </Link>
          )}
          <button
            type="button"
            className="btn btn-sec btn-sm grow"
            onClick={() => void run(t.orders.cancelConfirm, () => call(api.POST("/api/orders/{id}/cancel", { params: { path: { id: o.id } }, body: {} })))}
          >
            {t.orders.cancel}
          </button>
        </div>
      )}
      {o.status === "delivered" && (
        <div className="oact">
          <button
            type="button"
            className="btn btn-pri btn-sm grow"
            onClick={() => void run(t.orders.receiptConfirm, () => call(api.POST("/api/orders/{id}/receipt", { params: { path: { id: o.id } } })))}
          >
            {t.orders.receipt}
          </button>
        </div>
      )}
      <ErrorBanner text={error} />
    </div>
  );
}
