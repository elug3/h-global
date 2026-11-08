import { useState } from "react";
import { useLoaderData, useNavigate, useRevalidator } from "react-router";
import { api, call, errorText, load, serverApi, type Order } from "../api/client";
import { ErrorBanner, Icon, OrderStatus, Photo } from "../components/ui";
import { t } from "../i18n/ko";
import { date, dateTime, validPCCC, won } from "../lib/format";
import type { Route } from "./+types/order";

export async function loader({ request, params }: Route.LoaderArgs) {
  const order = await load(request, serverApi(request).GET("/api/orders/{id}", { params: { path: { id: params.id } } }));
  return { order };
}

export function meta() {
  return [{ title: `${t.orders.detail} — ${t.app.name}` }, { name: "robots", content: "noindex" }];
}

/** Order detail (board 14). */
export default function OrderPage() {
  const { order: o } = useLoaderData<typeof loader>();
  const revalidator = useRevalidator();
  const navigate = useNavigate();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  async function run(p: () => Promise<unknown>, confirmText?: string) {
    if (confirmText && !confirm(confirmText)) return;
    setBusy(true);
    setError(undefined);
    try {
      await p();
      await revalidator.revalidate();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  async function payAgain() {
    setBusy(true);
    try {
      const res = await call(api.POST("/api/orders/{id}/pay", { params: { path: { id: o.id } } }));
      if (o.payment_method === "virtual_account") navigate(`/orders/${o.id}/done`);
      else if (res.payment.provider === "fake") navigate(`/pay/${res.payment.id}?order=${o.id}`);
      else window.location.assign(res.payment.redirect_url);
    } catch (e) {
      setError(errorText(e));
      setBusy(false);
    }
  }

  const steps = [o.paid_at, o.inspection_done_at, o.customs_checked_at, o.shipped_at, o.delivered_at];
  const current = o.status === "canceled" || o.status === "pending_payment" ? -1 : steps.findIndex((x) => !x);
  const canceled = o.status === "canceled";

  return (
    <main className="page narrow">
      <div className="odh">
        <span className="mono">{o.number}</span>
        <OrderStatus status={o.status} />
      </div>
      <div className="odd">{t.orders.orderedAt(dateTime(o.created_at), o.payment_method === "card" ? t.checkout.card : t.checkout.va)}</div>
      {!canceled && (
        <ol className="psteps" aria-label={t.orders.detail}>
          {t.orders.steps.map((s, i) => (
            <li key={s} className={"ps" + (steps[i] ? " done" : i === current ? " cur" : "")}>
              <span>{steps[i] ? <Icon name="check" size={12} stroke={3} /> : i + 1}</span>
              <b>{s}</b>
              <em>{steps[i] ? date(steps[i]) : i === current ? t.orders.inProgress : ""}</em>
            </li>
          ))}
        </ol>
      )}
      {canceled && <p className="err">{t.orders.canceled(dateTime(o.canceled_at))}</p>}

      {o.inspection_media.length > 0 && (
        <section>
          <h2 className="secttl sm">
            {t.orders.inspection} {!o.shipped_at && <em className="newdot">{t.orders.inspectionNew}</em>}
          </h2>
          <div className="insp" data-testid="inspection">
            {o.inspection_media.map((m) =>
              m.kind === "video" ? (
                <video key={m.id} src={m.url} controls preload="metadata" playsInline />
              ) : (
                <a key={m.id} href={m.url} target="_blank" rel="noreferrer">
                  <img src={m.url} alt="" />
                </a>
              ),
            )}
          </div>
          {o.inspection_done_at && !o.shipped_at && <p className="hint">{t.orders.inspectionDone(date(o.inspection_done_at))}</p>}
        </section>
      )}

      {o.lines.map((l) => (
        <div className="box oi" key={l.id}>
          <Photo src={l.photo_url} size={64} radius={8} />
          <div className="oit">
            <div className="oin">{l.title}</div>
            <div className="oio">{[l.variant_label, t.orders.qty(l.qty)].filter(Boolean).join(" · ")}</div>
            <b>{won(l.unit_price_won * l.qty)}</b>
          </div>
        </div>
      ))}

      {o.shipped_at && (
        <p className="box">
          <b>{t.orders.tracking(o.carrier, o.tracking_number)}</b>
        </p>
      )}

      <h2 className="secttl sm">{t.orders.pccc}</h2>
      <Pccc o={o} onSave={(v) => run(() => call(api.PUT("/api/orders/{id}/customs-code", { params: { path: { id: o.id } }, body: { pccc: v } })))} />

      <h2 className="secttl sm">{t.orders.shipping}</h2>
      <div className="box addr">
        <b>
          {o.shipping.recipient} · {o.shipping.phone}
        </b>
        <span>
          ({o.shipping.postcode}) {o.shipping.address1} {o.shipping.address2}
        </span>
      </div>

      <h2 className="secttl sm">{t.orders.amounts}</h2>
      <div className="sums2">
        <div>
          <span>{t.orders.subtotal}</span>
          <b>{won(o.subtotal_won)}</b>
        </div>
        <div>
          <span>{t.orders.shippingFee}</span>
          <b>{won(o.shipping_fee_won)}</b>
        </div>
        <div className="tot2">
          <span>{t.orders.total}</span>
          <b>{won(o.total_won)}</b>
        </div>
        {o.refunded_won > 0 && (
          <div className="bad">
            <span>{t.orders.refunded}</span>
            <b>−{won(o.refunded_won)}</b>
          </div>
        )}
      </div>

      <ErrorBanner text={error} />
      <div className="form">
        {o.status === "pending_payment" && (
          <button type="button" className="btn btn-pri full" disabled={busy} onClick={() => void payAgain()}>
            {o.payment_method === "virtual_account" ? t.orders.showAccount : t.orders.pay}
          </button>
        )}
        {o.status === "delivered" && (
          <button
            type="button"
            className="btn btn-pri full"
            disabled={busy}
            onClick={() => void run(() => call(api.POST("/api/orders/{id}/receipt", { params: { path: { id: o.id } } })), t.orders.receiptConfirm)}
          >
            {t.orders.receipt}
          </button>
        )}
        {(o.status === "pending_payment" || o.status === "paid") && (
          <button
            type="button"
            className="btn btn-sec full"
            disabled={busy}
            onClick={() => void run(() => call(api.POST("/api/orders/{id}/cancel", { params: { path: { id: o.id } }, body: {} })), t.orders.cancelConfirm)}
          >
            {t.orders.cancel}
          </button>
        )}
        {["inspecting", "customs_check", "shipped"].includes(o.status) && <p className="cancel-note">{t.orders.noCancelHint}</p>}
      </div>
    </main>
  );
}

/** The customs code, editable until staff have checked it. */
function Pccc({ o, onSave }: { o: Order; onSave: (v: string) => Promise<void> }) {
  const locked = !!o.customs_checked_at || ["shipped", "delivered", "completed", "canceled"].includes(o.status);
  const [editing, setEditing] = useState(false);
  const [v, setV] = useState(o.shipping.pccc ?? "");
  const value = v.toUpperCase().replace(/\s/g, "");
  if (editing) {
    return (
      <form
        className="frow"
        onSubmit={(e) => {
          e.preventDefault();
          void onSave(value).then(() => setEditing(false));
        }}
      >
        <input className={"field grow" + (validPCCC(value) ? " ok" : " bad")} aria-label={t.orders.pccc} maxLength={13} value={v} onChange={(e) => setV(e.target.value)} />
        <button className="btn btn-pri btn-sm h48" disabled={!validPCCC(value)}>
          {t.orders.pcccSave}
        </button>
      </form>
    );
  }
  return (
    <>
      <div className={"field" + (validPCCC(o.shipping.pccc) ? " ok" : " bad")}>
        <span>{o.shipping.pccc || "—"}</span>
        {validPCCC(o.shipping.pccc) && (
          <em>
            <Icon name="check" size={16} stroke={2.4} /> {t.checkout.pcccOk}
          </em>
        )}
      </div>
      {!locked && (
        <p className="hint">
          {t.orders.pcccEditHint}{" "}
          <button type="button" className="linkbtn ul" onClick={() => setEditing(true)}>
            {t.orders.pcccEdit}
          </button>
        </p>
      )}
    </>
  );
}
