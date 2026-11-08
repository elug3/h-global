import { useEffect } from "react";
import { Link, useLoaderData, useRevalidator } from "react-router";
import { load, serverApi } from "../api/client";
import { FlowHeader } from "../components/FlowHeader";
import { CopyButton, Icon, Photo } from "../components/ui";
import { t } from "../i18n/ko";
import { dateTime, won } from "../lib/format";
import type { Route } from "./+types/done";

export async function loader({ request, params }: Route.LoaderArgs) {
  const api = serverApi(request);
  const id = params.id;
  const [order, payment] = await Promise.all([
    load(request, api.GET("/api/orders/{id}", { params: { path: { id } } })),
    api.GET("/api/orders/{id}/payment", { params: { path: { id } } }).then((r) => r.data ?? null),
  ]);
  return { order, payment };
}

export function meta() {
  return [{ title: `${t.done.paidTitle} — ${t.app.name}` }, { name: "robots", content: "noindex" }];
}

/** After paying (board 12): card done, or the virtual account to pay into. */
export default function Done() {
  const { order, payment } = useLoaderData<typeof loader>();
  const revalidator = useRevalidator();
  const pending = order.status === "pending_payment";
  const va = pending && order.payment_method === "virtual_account" ? payment?.virtual_account : null;

  // A card payment may be confirmed by the provider a moment later.
  useEffect(() => {
    if (!pending || order.payment_method !== "card") return;
    const id = setInterval(() => void revalidator.revalidate(), 3000);
    return () => clearInterval(id);
  }, [pending, order.payment_method, revalidator]);

  const line = order.lines[0];
  return (
    <>
      <FlowHeader title={va ? t.done.vaTitle : t.done.paidTitle} back="/orders" />
      <main className="page narrow">
        {va ? (
          <div className="form" data-testid="va">
            <div className="hold">
              <Icon name="clock" size={20} />
              <div className="holdt">
                <b>{t.done.vaDue(dateTime(va.due_at))}</b>
                <span>{t.done.vaDueHint}</span>
              </div>
            </div>
            <div className="va">
              <div className="var">
                <span>{t.done.amount}</span>
                <b className="amt">{won(order.total_won)}</b>
              </div>
              <div className="var">
                <span>{t.done.bank}</span>
                <b>{va.bank}</b>
              </div>
              <div className="var">
                <span>{t.done.account}</span>
                <b className="mono">{va.account}</b>
                <CopyButton text={va.account} />
              </div>
              <div className="var">
                <span>{t.done.holder}</span>
                <b>{va.holder}</b>
              </div>
            </div>
            <p className="hint">{t.done.vaHint}</p>
            <p className="done-s left">
              {t.done.orderNo} <b className="mono">{order.number}</b>
            </p>
            {payment?.provider === "fake" && (
              <Link className="btn btn-sec full" to={`/pay/${payment.id}?order=${order.id}`}>
                {t.pay.testTitle}
              </Link>
            )}
            <Link className="btn btn-pri full" to={`/orders/${order.id}`}>
              {t.done.toOrder}
            </Link>
          </div>
        ) : pending ? (
          <div className="form center-col">
            <p className="muted">{t.done.waiting}</p>
            <Link className="btn btn-sec full" to={`/orders/${order.id}`}>
              {t.done.toOrder}
            </Link>
          </div>
        ) : (
          <div className="form center-col" data-testid="paid">
            <span className="okc">
              <Icon name="check" size={34} stroke={2.6} />
            </span>
            <h2 className="done-t">{t.done.paidTitle}</h2>
            <p className="done-s">
              {t.done.orderNo} <b className="mono">{order.number}</b>
            </p>
            {line && (
              <div className="box oi wide">
                <Photo src={line.photo_url} size={64} radius={8} />
                <div className="oit">
                  <div className="oin">{line.title}</div>
                  <div className="oio">{[line.variant_label, t.orders.qty(line.qty)].filter(Boolean).join(" · ")}</div>
                  <b>{won(order.total_won)}</b>
                </div>
              </div>
            )}
            <div className="next-box">
              <b>{t.done.nextTitle}</b>
              <ol>
                {t.done.next.map((s, i) => (
                  <li key={i}>
                    <span>{i + 1}</span>
                    <p>{s}</p>
                  </li>
                ))}
              </ol>
            </div>
            <Link className="btn btn-pri full" to={`/orders/${order.id}`}>
              {t.done.toOrder}
            </Link>
            <Link className="btn btn-sec full" to="/">
              {t.done.toHome}
            </Link>
          </div>
        )}
      </main>
    </>
  );
}
