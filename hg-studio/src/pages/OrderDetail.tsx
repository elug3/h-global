import { useRef, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router";
import { api, call, errorText, upload, type Order, type OrderMedia } from "../api/client";
import { useAuth } from "../auth";
import { Icon } from "../components/Icon";
import { historyText, OrderStatusTag, validPCCC } from "../components/order";
import { Gallery } from "../components/Gallery";
import { CopyButton, ErrorBanner } from "../components/ui";
import { t } from "../i18n/ko";
import { dateTime, won } from "../lib/format";
import { ORDER_EVENTS, useDebounced, useEventStream, useLoad } from "../lib/hooks";

/** One order: what was bought, where it goes, and the next step (step 4). */
export function OrderDetail() {
  const { id = "" } = useParams();
  const order = useLoad(() => call(api.GET("/studio-api/orders/{id}", { params: { path: { id } } })), [id]);

  // Another staff member, the customer or the payment provider may change it.
  const reloadSoon = useDebounced(() => void order.reload(), 300);
  useEventStream(
    "/studio-api/orders/events",
    "now",
    ORDER_EVENTS,
    (_type, data) => {
      if ((data as { order?: { id?: string } } | undefined)?.order?.id === id) reloadSoon();
    },
    reloadSoon,
  );

  if (!order.data) {
    return (
      <div className="page">
        <ErrorBanner text={order.error} onRetry={order.reload} />
        {!order.error && <p className="muted">{t.app.loading}</p>}
      </div>
    );
  }
  const o = order.data;
  const changed = (next: Order) => order.setData(next);

  return (
    <div className="page page-wide">
      <header className="page-head">
        <Link to="/orders" className="back" aria-label={t.orders.title}>
          <Icon name="back" />
        </Link>
        <h1>{t.orders.detailTitle(o.number)}</h1>
        <OrderStatusTag status={o.status} />
        <span className="muted small">{dateTime(o.created_at)}</span>
      </header>
      <Progress o={o} />
      <div className="order-grid">
        <div className="col">
          <Lines o={o} />
          <div className="two">
            <ShippingCard o={o} />
            <PaymentCard o={o} />
          </div>
          <History o={o} />
        </div>
        <div className="col">
          <NextStep o={o} onChange={changed} />
          {o.status !== "paid" && o.status !== "inspecting" && o.inspection_media.length > 0 && (
            <section className="card section">
              <h2>{t.orders.inspection}</h2>
              <Gallery media={o.inspection_media} />
            </section>
          )}
          <CancelCard o={o} onChange={changed} />
        </div>
      </div>
    </div>
  );
}

function Progress({ o }: { o: Order }) {
  const at = [o.paid_at, o.inspection_done_at, o.customs_checked_at, o.shipped_at, o.delivered_at, o.completed_at];
  const current = o.status === "canceled" || o.status === "pending_payment" ? -1 : at.findIndex((x) => !x);
  return (
    <ol className={"progress" + (o.status === "canceled" ? " canceled" : "")} aria-label={t.orders.status}>
      {t.orders.steps.map((label, i) => (
        <li key={label} className={at[i] ? "done" : i === current ? "current" : ""}>
          <i aria-hidden="true">{at[i] ? "✓" : i + 1}</i>
          <b>{label}</b>
          <span className="muted small">{dateTime(at[i])}</span>
        </li>
      ))}
    </ol>
  );
}

function Lines({ o }: { o: Order }) {
  return (
    <section className="card section">
      <h2>{t.orders.lines}</h2>
      <ul className="lines">
        {o.lines.map((l) => (
          <li key={l.id}>
            {l.photo_url ? (
              <img className="thumb" src={l.photo_url} width={56} height={56} alt="" />
            ) : (
              <span className="thumb thumb-empty" style={{ width: 56, height: 56 }} />
            )}
            <div className="grow">
              <Link to={`/products/${l.product_id}`} className="strong">
                {l.title}
              </Link>
              {l.variant_label && <div className="muted small">{l.variant_label}</div>}
              <div className="muted small">
                {won(l.unit_price_won)} × {t.orders.qty(l.qty)}
              </div>
            </div>
            <b>{won(l.unit_price_won * l.qty)}</b>
          </li>
        ))}
      </ul>
      <dl className="sums">
        <dt>{t.orders.subtotal}</dt>
        <dd>{won(o.subtotal_won)}</dd>
        <dt>{t.orders.shippingFee}</dt>
        <dd>{won(o.shipping_fee_won)}</dd>
        <dt className="strong">{t.orders.totalPaid}</dt>
        <dd className="strong big">{won(o.total_won)}</dd>
        {o.refunded_won > 0 && (
          <>
            <dt className="error">{t.orders.refunded}</dt>
            <dd className="error">−{won(o.refunded_won)}</dd>
          </>
        )}
      </dl>
    </section>
  );
}

function ShippingCard({ o }: { o: Order }) {
  const s = o.shipping;
  const address = `(${s.postcode}) ${s.address1}${s.address2 ? " " + s.address2 : ""}`;
  const pcccOk = validPCCC(s.pccc);
  return (
    <section className="card section">
      <div className="section-head">
        <h2>{t.orders.shipping}</h2>
        <CopyButton text={`${s.recipient} ${s.phone}\n${address}`} />
      </div>
      <dl className="facts">
        <dt>{t.orders.recipient}</dt>
        <dd className="strong">{s.recipient}</dd>
        <dt>{t.orders.phone}</dt>
        <dd>{s.phone}</dd>
        <dt>{t.orders.address}</dt>
        <dd>{address}</dd>
        <dt>{t.orders.pccc}</dt>
        <dd>
          <span className="mono">{s.pccc || "—"}</span>{" "}
          <span className={"tag " + (pcccOk ? "tag-ok" : "tag-warn")}>
            {pcccOk ? t.orders.pcccOk : s.pccc ? t.orders.pcccInvalid : t.orders.pcccMissing}
          </span>
        </dd>
      </dl>
    </section>
  );
}

function PaymentCard({ o }: { o: Order }) {
  return (
    <section className="card section">
      <h2>{t.orders.payment}</h2>
      <dl className="facts">
        <dt>{t.orders.method}</dt>
        <dd>{t.orders.methods[o.payment_method] ?? o.payment_method}</dd>
        {o.paid_at ? (
          <>
            <dt>{t.orders.paidAt}</dt>
            <dd>{dateTime(o.paid_at)}</dd>
          </>
        ) : (
          o.status === "pending_payment" && (
            <>
              <dt>{t.orders.deadline}</dt>
              <dd>{dateTime(o.payment_deadline)}</dd>
            </>
          )
        )}
        <dt>{t.orders.totalPaid}</dt>
        <dd className="strong">{won(o.total_won)}</dd>
        {o.shipped_at && (
          <>
            <dt>{t.orders.shippedWith}</dt>
            <dd>
              {o.carrier} <span className="mono">{o.tracking_number}</span>
            </dd>
          </>
        )}
      </dl>
    </section>
  );
}

function History({ o }: { o: Order }) {
  return (
    <section className="card section">
      <h2>{t.orders.history}</h2>
      <ol className="timeline" data-testid="history">
        {[...(o.history ?? [])].reverse().map((h, i) => {
          const { what, note } = historyText(h);
          return (
            <li key={i}>
              <span className="muted small">{dateTime(h.at)}</span>
              <span>
                <b>{what}</b> <span className="muted small">· {t.orders.actors[h.actor_kind] ?? h.actor_kind}</span>
                {note && <span className="note">{note}</span>}
              </span>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

/** The one thing to do next for this order. */
function NextStep({ o, onChange }: { o: Order; onChange: (o: Order) => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  async function run(p: () => Promise<Order>) {
    setBusy(true);
    setError(undefined);
    try {
      onChange(await p());
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  const path = { params: { path: { id: o.id } } };

  let body;
  switch (o.status) {
    case "pending_payment":
      body = <p className="muted">{t.orders.waitingPayment(dateTime(o.payment_deadline))}</p>;
      break;
    case "paid":
    case "inspecting":
      body = <Inspection o={o} busy={busy} run={run} />;
      break;
    case "customs_check":
      body = o.customs_checked_at ? (
        <ShipForm o={o} busy={busy} run={run} />
      ) : (
        <CustomsCheck o={o} busy={busy} onCheck={() => void run(() => call(api.POST("/studio-api/orders/{id}/customs-check", path)))} />
      );
      break;
    case "shipped":
      body = (
        <>
          <p>
            {o.carrier} <span className="mono strong">{o.tracking_number}</span>
          </p>
          <p className="muted small">{t.orders.deliverHint}</p>
          <button className="btn btn-primary" disabled={busy} onClick={() => void run(() => call(api.POST("/studio-api/orders/{id}/deliver", path)))}>
            {t.orders.deliver}
          </button>
        </>
      );
      break;
    case "delivered":
      body = (
        <>
          <p className="muted small">{t.orders.completeHint}</p>
          <button className="btn btn-primary" disabled={busy} onClick={() => void run(() => call(api.POST("/studio-api/orders/{id}/complete", path)))}>
            {t.orders.complete}
          </button>
        </>
      );
      break;
    case "completed":
      body = <p className="muted">{t.orders.finished}</p>;
      break;
    default:
      body = (
        <p className="muted">
          {t.orders.canceledInfo(dateTime(o.canceled_at))}
          {o.cancel_reason && (
            <>
              <br />
              {t.orders.cancelReasonLabel}: {o.cancel_reason}
            </>
          )}
        </p>
      );
  }

  const title =
    o.status === "paid" || o.status === "inspecting"
      ? t.orders.inspection
      : o.status === "customs_check"
        ? o.customs_checked_at
          ? t.orders.ship
          : t.orders.customs
        : t.orders.next;
  return (
    <section className="card section next-step" data-testid="next-step">
      <h2>{title}</h2>
      {body}
      <ErrorBanner text={error} />
    </section>
  );
}

type Run = (p: () => Promise<Order>) => Promise<void>;

function Inspection({ o, busy, run }: { o: Order; busy: boolean; run: Run }) {
  const input = useRef<HTMLInputElement>(null);
  const [note, setNote] = useState("");
  const [uploading, setUploading] = useState(false);

  async function add(files: FileList | null) {
    if (!files?.length) return;
    setUploading(true);
    await run(async () => {
      let last = o;
      for (const f of Array.from(files)) {
        last = await upload<Order>(`/studio-api/orders/${o.id}/inspection`, f, { kind: f.type.startsWith("video/") ? "video" : "photo" });
      }
      return last;
    });
    setUploading(false);
    if (input.current) input.current.value = "";
  }

  return (
    <>
      <p className="muted small">{t.orders.inspectionHint}</p>
      <Gallery media={o.inspection_media as OrderMedia[]} />
      <input
        ref={input}
        type="file"
        hidden
        multiple
        accept="image/jpeg,image/png,video/mp4,video/quicktime,video/webm"
        data-testid="inspection-file"
        onChange={(e) => void add(e.target.files)}
      />
      <button className="btn btn-secondary" disabled={busy} onClick={() => input.current?.click()}>
        <Icon name="upload" size={18} />
        {uploading ? t.orders.uploading : t.orders.addInspection}
      </button>
      {o.status === "inspecting" && (
        <>
          <label className="field">
            <span>{t.orders.inspectionNote}</span>
            <input value={note} maxLength={500} placeholder={t.orders.inspectionNotePlaceholder} onChange={(e) => setNote(e.target.value)} />
          </label>
          <button
            className="btn btn-primary"
            disabled={busy || o.inspection_media.length === 0}
            onClick={() =>
              void run(() =>
                call(api.POST("/studio-api/orders/{id}/inspection/complete", { params: { path: { id: o.id } }, body: { note: note.trim() } })),
              )
            }
          >
            {t.orders.completeInspection}
          </button>
        </>
      )}
    </>
  );
}

function CustomsCheck({ o, busy, onCheck }: { o: Order; busy: boolean; onCheck: () => void }) {
  const ok = validPCCC(o.shipping.pccc);
  return (
    <>
      <div className="pccc">
        <span className="muted small">{t.orders.pccc}</span>
        <b className="mono">{o.shipping.pccc || "—"}</b>
        <span className="muted small">
          {t.orders.recipient}: {o.shipping.recipient}
        </span>
      </div>
      <p className="muted small">{ok ? t.orders.customsHint : t.orders.customsMissingHint}</p>
      <button className="btn btn-primary" disabled={busy || !ok} onClick={onCheck}>
        {t.orders.checkCustoms}
      </button>
    </>
  );
}

function ShipForm({ o, busy, run }: { o: Order; busy: boolean; run: Run }) {
  const carriers = t.orders.carriers;
  const [carrier, setCarrier] = useState(carriers[0]);
  const [other, setOther] = useState("");
  const [tracking, setTracking] = useState("");
  const name = carrier === "other" ? other.trim() : carrier;

  function submit(e: FormEvent) {
    e.preventDefault();
    void run(() =>
      call(api.POST("/studio-api/orders/{id}/ship", { params: { path: { id: o.id } }, body: { carrier: name, tracking_number: tracking.trim() } })),
    );
  }

  return (
    <form className="ship-form" onSubmit={submit}>
      <p className="ok small">{t.orders.customsChecked(dateTime(o.customs_checked_at))}</p>
      <label className="field">
        <span>{t.orders.carrier}</span>
        <select value={carrier} onChange={(e) => setCarrier(e.target.value)}>
          {carriers.map((c) => (
            <option key={c}>{c}</option>
          ))}
          <option value="other">{t.orders.carrierOther}</option>
        </select>
      </label>
      {carrier === "other" && (
        <input aria-label={t.orders.carrierOtherPlaceholder} placeholder={t.orders.carrierOtherPlaceholder} maxLength={50} value={other} onChange={(e) => setOther(e.target.value)} />
      )}
      <label className="field">
        <span>{t.orders.tracking}</span>
        <input required maxLength={100} inputMode="numeric" value={tracking} onChange={(e) => setTracking(e.target.value)} />
      </label>
      <button className="btn btn-primary" disabled={busy || !name || !tracking.trim()}>
        {t.orders.ship}
      </button>
    </form>
  );
}

function CancelCard({ o, onChange }: { o: Order; onChange: (o: Order) => void }) {
  const { can } = useAuth();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  if (o.status === "completed" || o.status === "canceled") return null;
  if (!can("manager")) return <p className="muted small">{t.orders.cancelManagerOnly}</p>;

  const amount = won(o.total_won - o.refunded_won);
  const effect =
    o.status === "pending_payment"
      ? t.orders.cancelUnpaid
      : o.status === "shipped" || o.status === "delivered"
        ? t.orders.cancelAfterShip(amount)
        : t.orders.cancelBeforeShip(amount);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      onChange(await call(api.POST("/studio-api/orders/{id}/cancel", { params: { path: { id: o.id } }, body: { reason: reason.trim() } })));
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="card section cancel-card">
      {!open ? (
        <button className="btn btn-secondary btn-danger-text" onClick={() => setOpen(true)}>
          {t.orders.cancel}
        </button>
      ) : (
        <form className="ship-form" onSubmit={submit}>
          <h2>{t.orders.cancel}</h2>
          <p>{effect}</p>
          <label className="field">
            <span>{t.orders.cancelReasonLabel}</span>
            <input required maxLength={500} placeholder={t.orders.cancelReasonPlaceholder} value={reason} onChange={(e) => setReason(e.target.value)} />
          </label>
          <ErrorBanner text={error} />
          <div className="row">
            <button className="btn btn-danger" disabled={busy || !reason.trim()}>
              {o.status === "pending_payment" ? t.orders.cancel : t.orders.cancelSubmit}
            </button>
            <button type="button" className="btn btn-secondary" onClick={() => setOpen(false)}>
              {t.app.close}
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
