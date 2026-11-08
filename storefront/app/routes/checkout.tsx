import { useEffect, useState } from "react";
import { Link, redirect, useLoaderData, useNavigate } from "react-router";
import { api, call, errorText, load, serverApi, type Checkout, type Shipping } from "../api/client";
import { AddressForm, complete, rememberShipping, savedShipping } from "../components/AddressForm";
import { FlowHeader } from "../components/FlowHeader";
import { ErrorBanner, Icon, Photo } from "../components/ui";
import { t } from "../i18n/ko";
import { won } from "../lib/format";
import type { Route } from "./+types/checkout";

export async function loader({ request, params }: Route.LoaderArgs) {
  const co = await load(request, serverApi(request).GET("/api/checkout/{id}", { params: { path: { id: params.id } } }));
  if (co.status === "ordered" && co.order_id) throw redirect(`/orders/${co.order_id}`);
  return { co, now: Date.now() };
}

export function meta() {
  return [{ title: `${t.checkout.title} — ${t.app.name}` }, { name: "robots", content: "noindex" }];
}

function mmss(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}

/** Checkout (board 5): the held items, address and customs code, payment. */
export default function CheckoutPage() {
  const { co: initial, now: serverNow } = useLoaderData<typeof loader>();
  const navigate = useNavigate();
  const [co, setCo] = useState<Checkout>(initial);
  const [now, setNow] = useState(serverNow);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<Shipping | null>(initial.shipping ?? null);
  const [method, setMethod] = useState<"card" | "virtual_account">("card");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(() => {
    // Use the address saved on this device for a new checkout.
    if (!initial.shipping) {
      const saved = savedShipping();
      if (saved) setDraft(saved);
      setEditing(!complete(saved));
    }
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [initial.shipping]);

  const total = Date.parse(co.expires_at) - Date.parse(co.created_at);
  const left = Date.parse(co.expires_at) - now;
  const expired = co.status !== "open" || left <= 0;
  const productId = co.lines[0]?.product_id;

  async function saveAddress(s: Shipping, remember: boolean) {
    setBusy(true);
    setError(undefined);
    try {
      setCo(await call(api.PUT("/api/checkout/{id}/shipping", { params: { path: { id: co.id } }, body: s })));
      setDraft(s);
      if (remember) rememberShipping(s);
      setEditing(false);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  async function pay() {
    setBusy(true);
    setError(undefined);
    try {
      // A saved address that was never sent for this checkout goes first.
      if (!co.shipping && complete(draft)) {
        setCo(await call(api.PUT("/api/checkout/{id}/shipping", { params: { path: { id: co.id } }, body: draft })));
      }
      const res = await call(api.POST("/api/checkout/{id}/pay", { params: { path: { id: co.id } }, body: { method } }));
      const { order, payment } = res;
      if (method === "virtual_account") navigate(`/orders/${order.id}/done`);
      else if (payment.provider === "fake") navigate(`/pay/${payment.id}?order=${order.id}`);
      else window.location.assign(payment.redirect_url);
    } catch (e) {
      setError(errorText(e));
      setBusy(false);
    }
  }

  const shipping = co.shipping ?? (complete(draft) ? draft : null);

  return (
    <>
      <FlowHeader title={t.checkout.title} back={productId ? `/products/${productId}` : "/"} />
      <main className="page narrow">
        {expired ? (
          <div className="form center-col">
            <p className="err">{t.checkout.expired}</p>
            {productId && (
              <Link className="btn btn-pri full" to={`/products/${productId}`}>
                {t.checkout.expiredBack}
              </Link>
            )}
          </div>
        ) : (
          <div className="form">
            <div className="hold" role="timer">
              <Icon name="clock" size={20} />
              <div className="holdt">
                <b>{t.checkout.hold(mmss(left))}</b>
                <span>{t.checkout.holdHint}</span>
              </div>
            </div>
            <div className="holdbar">
              <i style={{ width: `${Math.max(0, Math.min(100, (left / total) * 100))}%` }} />
            </div>
            <ol className="steps">
              {t.checkout.steps.map((s, i) => (
                <li key={s} className={i === 0 ? "on" : ""}>
                  <span>{i + 1}</span>
                  {s}
                </li>
              ))}
            </ol>
            {co.lines.map((l) => (
              <div className="box oi" key={l.id}>
                <Photo src={l.photo_url} size={64} radius={8} />
                <div className="oit">
                  <div className="oin">{l.title}</div>
                  <div className="oio">
                    {[l.variant_label, t.checkout.qty(l.qty)].filter(Boolean).join(" · ")}
                  </div>
                  <b>{won(l.unit_price_won * l.qty)}</b>
                </div>
              </div>
            ))}
            <h2 className="secttl sm">{t.checkout.shipping}</h2>
            {editing || !shipping ? (
              <AddressForm initial={draft} busy={busy} onSave={(s, r) => void saveAddress(s, r)} onCancel={shipping ? () => setEditing(false) : undefined} />
            ) : (
              <>
                <div className="box addr" data-testid="address">
                  <b>
                    {shipping.recipient} · {shipping.phone}
                  </b>
                  <span>
                    ({shipping.postcode}) {shipping.address1} {shipping.address2}
                  </span>
                  <button type="button" className="linkbtn addr-change" onClick={() => setEditing(true)}>
                    {t.checkout.change}
                  </button>
                </div>
                <h2 className="secttl sm">{t.checkout.pccc}</h2>
                <div className="field ok">
                  <span>{shipping.pccc}</span>
                  <em>
                    <Icon name="check" size={16} stroke={2.4} /> {t.checkout.pcccOk}
                  </em>
                </div>
                <p className="hint">{t.checkout.pcccHint}</p>
              </>
            )}
            <h2 className="secttl sm">{t.checkout.method}</h2>
            <div className="pays" role="radiogroup" aria-label={t.checkout.method}>
              {(["card", "virtual_account"] as const).map((m) => (
                <button key={m} type="button" role="radio" aria-checked={method === m} className={"pay" + (method === m ? " on" : "")} onClick={() => setMethod(m)}>
                  {m === "card" ? t.checkout.card : t.checkout.va}
                </button>
              ))}
            </div>
            <div className="tot">
              <span>{t.checkout.total}</span>
              <b>{won(co.total_won)}</b>
            </div>
            <ErrorBanner text={error} />
            <button type="button" className="btn btn-pri full" disabled={busy || !shipping || editing} onClick={() => void pay()}>
              {busy ? t.checkout.paying : t.checkout.pay(won(co.total_won))}
            </button>
            {(!shipping || editing) && <p className="hint center">{t.checkout.needAddress}</p>}
          </div>
        )}
      </main>
    </>
  );
}
