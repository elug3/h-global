import { useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { ApiError, api, call, errorText, type Product, type Variant } from "../api/client";
import { t } from "../i18n/ko";
import { won } from "../lib/format";
import { ErrorBanner, NumberBadge, Stepper, Swatch } from "./ui";

/** Selected value per option; the variant is the one that has all of them. */
function findVariant(p: Product, sel: Record<string, string>): Variant | undefined {
  if (p.options.length === 0) return p.variants[0];
  return p.variants.find((v) => p.options.every((o) => sel[o.id] && v.option_value_ids.includes(sel[o.id])));
}

/** A value is available if some buyable variant has it and the other choices made so far. */
function available(p: Product, sel: Record<string, string>, optionId: string, valueId: string): boolean {
  return p.variants.some(
    (v) =>
      v.buyable &&
      v.option_value_ids.includes(valueId) &&
      p.options.every((o) => o.id === optionId || !sel[o.id] || v.option_value_ids.includes(sel[o.id])),
  );
}

function initialSelection(p: Product, variantId?: string): Record<string, string> {
  const sel: Record<string, string> = {};
  const v = variantId ? p.variants.find((x) => x.id === variantId) : undefined;
  for (const o of p.options) {
    const fromVariant = v && o.values.find((ov) => v.option_value_ids.includes(ov.id));
    if (fromVariant) sel[o.id] = fromVariant.id;
    else if (o.values.length === 1) sel[o.id] = o.values[0].id;
  }
  return sel;
}

/**
 * Options, quantity and Buy now (boards 3 and 4). Buy now holds the stock
 * for 10 minutes and opens checkout; signed-out visitors sign in first and
 * come back to this product with the same choice.
 */
export function BuyPanel({
  product: p,
  number,
  initialVariant,
  initialQty = 1,
  header = false,
}: {
  product: Product;
  number?: number;
  initialVariant?: string;
  initialQty?: number;
  header?: boolean;
}) {
  const navigate = useNavigate();
  const [sel, setSel] = useState(() => initialSelection(p, initialVariant));
  const [qty, setQty] = useState(initialQty);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const variant = useMemo(() => findVariant(p, sel), [p, sel]);
  const price = variant?.price_won ?? p.price_won;
  // Left in stock: for the chosen variant, else over all variants (null = no count).
  const counted = p.variants.filter((v) => v.buyable);
  const left = variant
    ? variant.available_qty
    : counted.length > 0 && counted.every((v) => v.available_qty != null)
      ? counted.reduce((n, v) => n + (v.available_qty ?? 0), 0)
      : null;
  const missing = p.options.find((o) => !sel[o.id]);

  function choose(optionId: string, valueId: string) {
    setSel({ ...sel, [optionId]: valueId });
    setError(undefined);
  }

  async function buy() {
    if (!variant) {
      setError(missing ? t.product.chooseOption(missing.name) : t.product.unavailable);
      return;
    }
    setBusy(true);
    setError(undefined);
    try {
      const co = await call(api.POST("/api/checkout", { body: { lines: [{ variant_id: variant.id, qty }] } }));
      navigate(`/checkout/${co.id}`);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        const back = `/products/${p.id}?buy=${variant.id}&qty=${qty}`;
        navigate(`/signup?next=${encodeURIComponent(back)}&product=${encodeURIComponent(p.title)}`);
        return;
      }
      setError(errorText(e));
      setBusy(false);
    }
  }

  return (
    <div className="buy">
      {header && (
        <div className="buy-head">
          {number !== undefined && <NumberBadge n={number} size="s" />}
          <div className="buy-title">{p.title}</div>
          <div className="buy-price">{won(price)}</div>
          {left !== null && left > 0 && left <= 5 && <span className="stock">{t.product.left(left)}</span>}
          {p.sold_out && <span className="stock out">{t.product.soldOut}</span>}
        </div>
      )}
      {p.options.map((o) => {
        const chosen = o.values.find((v) => v.id === sel[o.id]);
        const colors = o.values.some((v) => v.hex1);
        return (
          <div className="opt" key={o.id} role="radiogroup" aria-label={o.name}>
            <div className="ol">
              {o.name} {chosen && <b>{chosen.label}</b>}
            </div>
            <div className={colors ? "swrow" : "chips"}>
              {o.values.map((v) => {
                const ok = available(p, sel, o.id, v.id);
                const on = sel[o.id] === v.id;
                return colors ? (
                  <button
                    key={v.id}
                    type="button"
                    role="radio"
                    aria-checked={on}
                    aria-label={v.label + (ok ? "" : ` · ${t.product.soldOut}`)}
                    title={v.label}
                    className={"swbtn" + (on ? " on" : "") + (ok ? "" : " dis")}
                    disabled={!ok}
                    onClick={() => choose(o.id, v.id)}
                  >
                    <Swatch hex1={v.hex1 ?? "#ddd"} hex2={v.hex2} size={32} />
                  </button>
                ) : (
                  <button
                    key={v.id}
                    type="button"
                    role="radio"
                    aria-checked={on}
                    className={"chip" + (on ? " on" : "") + (ok ? "" : " dis")}
                    disabled={!ok}
                    onClick={() => choose(o.id, v.id)}
                  >
                    {v.label}
                    {ok ? "" : ` · ${t.product.soldOut}`}
                  </button>
                );
              })}
            </div>
          </div>
        );
      })}
      <div className="opt row">
        <div className="ol">{t.product.qty}</div>
        <Stepper label={t.product.qty} value={qty} min={1} max={Math.max(1, Math.min(99, (variant ? variant.available_qty : null) ?? 99))} onChange={setQty} />
      </div>
      <ErrorBanner text={error} />
      <button type="button" className="btn btn-pri full" disabled={busy || !p.buyable} onClick={() => void buy()}>
        {busy ? t.product.buying : p.buyable ? `${t.product.buyNow} · ${won(price * qty)}` : t.product.soldOut}
      </button>
    </div>
  );
}
