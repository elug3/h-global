import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { api, call, errorText, type Product } from "../api/client";
import { COLOR, OptionPicker, SIZE, toOptions, type OptionValue } from "../components/OptionPicker";
import { ProductStatusTag, soldOfStock, Thumb } from "../components/product";
import { ErrorBanner, Stepper, Toggle } from "../components/ui";
import { t } from "../i18n/ko";
import { dateTime, won } from "../lib/format";

const PAGE = 50;
const STATUSES = ["draft", "live", "hidden", "archived"];

/** All products, with search and filters; create one by hand (step 5). */
export function Products() {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const status = params.get("status") ?? "";
  const incomplete = params.get("incomplete") === "true";
  const query = params.get("q") ?? "";
  const [q, setQ] = useState(query);
  const [items, setItems] = useState<Product[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [creating, setCreating] = useState(false);

  const fetchPage = useCallback(
    (offset: number) =>
      call(
        api.GET("/studio-api/products", {
          params: {
            query: {
              status: status || undefined,
              q: query || undefined,
              incomplete: incomplete || undefined,
              limit: PAGE,
              offset,
            },
          },
        }),
      ),
    [status, query, incomplete],
  );

  useEffect(() => {
    let stale = false;
    setLoading(true);
    fetchPage(0)
      .then((res) => {
        if (stale) return;
        setItems(res.items);
        setHasMore(res.items.length === PAGE);
        setError(undefined);
      })
      .catch((e) => !stale && setError(errorText(e)))
      .finally(() => !stale && setLoading(false));
    return () => {
      stale = true;
    };
  }, [fetchPage]);

  async function more() {
    try {
      const res = await fetchPage(items.length);
      setItems((list) => [...list, ...res.items]);
      setHasMore(res.items.length === PAGE);
    } catch (e) {
      setError(errorText(e));
    }
  }

  function set(key: string, value: string | undefined) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
  }

  function search(e: FormEvent) {
    e.preventDefault();
    set("q", q.trim() || undefined);
  }

  return (
    <div className="page page-wide">
      <header className="page-head">
        <h1>{t.products.title}</h1>
        <div className="grow" />
        <form className="row" onSubmit={search} role="search">
          <input
            className="search"
            type="search"
            aria-label={t.products.search}
            placeholder={t.products.searchPlaceholder}
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <button className="btn btn-secondary">{t.app.search}</button>
        </form>
        <button className="btn btn-primary" onClick={() => setCreating((v) => !v)} aria-expanded={creating}>
          {t.products.create}
        </button>
      </header>
      {creating && <CreateProduct onCreated={(p) => navigate(`/products/${p.id}`)} onCancel={() => setCreating(false)} />}
      <div className="row filter-row">
        <div className="filter-tabs" role="tablist" aria-label={t.products.colStatus}>
          {["", ...STATUSES].map((s) => (
            <button key={s || "all"} role="tab" aria-selected={s === status} className={s === status ? "on" : ""} onClick={() => set("status", s)}>
              {s ? t.productStatus[s] : t.products.all}
            </button>
          ))}
        </div>
        <Toggle checked={incomplete} onChange={(v) => set("incomplete", v ? "true" : undefined)} label={t.products.incompleteOnly} />
      </div>
      <ErrorBanner text={error} />
      {loading && items.length === 0 ? (
        <p className="muted">{t.app.loading}</p>
      ) : items.length === 0 ? (
        <p className="muted empty">{t.products.empty}</p>
      ) : (
        <div className="card table-card">
          <table className="table" data-testid="products">
            <thead>
              <tr>
                <th>{t.products.colProduct}</th>
                <th className="num-col">{t.products.colPrice}</th>
                <th>{t.products.colStatus}</th>
                <th>{t.products.colStock}</th>
                <th>{t.products.colSource}</th>
                <th>{t.products.colUpdated}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((p) => (
                <tr key={p.id} className="clickable" onClick={() => navigate(`/products/${p.id}`)}>
                  <td className="cell-product">
                    <Thumb product={p} size={40} />
                    <span className="cell-title">
                      <Link to={`/products/${p.id}`} className="strong ellipsis" onClick={(e) => e.stopPropagation()}>
                        {p.title}
                      </Link>
                      {!p.details_complete && <span className="tag tag-warn">{t.products.incomplete}</span>}
                    </span>
                  </td>
                  <td className="num-col strong">{won(p.price_won)}</td>
                  <td>
                    <ProductStatusTag status={p.status} />
                  </td>
                  <td>{soldOfStock(p)}</td>
                  <td className="muted">{t.products.source[p.source] ?? p.source}</td>
                  <td className="muted">{dateTime(p.updated_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {hasMore && (
            <div className="table-foot">
              <button className="btn btn-secondary btn-sm" onClick={() => void more()}>
                {t.products.more}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function CreateProduct({ onCreated, onCancel }: { onCreated: (p: Product) => void; onCancel: () => void }) {
  const [title, setTitle] = useState("");
  const [price, setPrice] = useState("");
  const [colors, setColors] = useState<OptionValue[]>([]);
  const [sizes, setSizes] = useState<OptionValue[]>([]);
  const [qty, setQty] = useState(1);
  const [unknownQty, setUnknownQty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      onCreated(
        await call(
          api.POST("/studio-api/products", {
            body: {
              title: title.trim(),
              price_won: Number(price),
              status: "draft",
              options: toOptions(colors, sizes),
              stock: unknownQty ? { mode: "until_sold_out", qty: 0 } : { mode: "counted", qty },
            },
          }),
        ),
      );
    } catch (err) {
      setError(errorText(err));
      setBusy(false);
    }
  }

  return (
    <form className="card section create-product" onSubmit={submit} aria-label={t.products.createTitle}>
      <div>
        <h2>{t.products.createTitle}</h2>
        <p className="muted small">{t.products.createHint}</p>
      </div>
      <div className="two">
        <label className="field">
          <span>{t.products.name}</span>
          <input required maxLength={200} value={title} onChange={(e) => setTitle(e.target.value)} />
        </label>
        <label className="field">
          <span>{t.products.price}</span>
          <input required inputMode="numeric" pattern="[0-9]*" value={price} onChange={(e) => setPrice(e.target.value.replace(/\D/g, ""))} />
        </label>
      </div>
      <OptionPicker name={COLOR} label={t.console.color} withColor values={colors} onChange={setColors} />
      <OptionPicker name={SIZE} label={t.console.size} values={sizes} onChange={setSizes} />
      <div className="field">
        <span>{t.console.quantity}</span>
        <div className="row">
          {!unknownQty && <Stepper label={t.console.quantity} value={qty} min={0} onChange={setQty} />}
          <Toggle checked={unknownQty} onChange={setUnknownQty} label={t.console.unknownQty} />
        </div>
      </div>
      <ErrorBanner text={error} />
      <div className="row">
        <button className="btn btn-primary" disabled={busy || !title.trim() || !price}>
          {t.products.createSubmit}
        </button>
        <button type="button" className="btn btn-secondary" onClick={onCancel}>
          {t.app.cancel}
        </button>
      </div>
    </form>
  );
}
