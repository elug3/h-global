import { useRef, useState, type FormEvent, type ReactNode } from "react";
import { Link, useLocation, useParams } from "react-router";
import { api, call, errorText, upload, type Media, type Product, type Variant, type VariantPatch } from "../api/client";
import { Gallery } from "../components/Gallery";
import { Icon } from "../components/Icon";
import { missingDetails, ProductStatusTag } from "../components/product";
import { ErrorBanner, Swatch } from "../components/ui";
import { t } from "../i18n/ko";
import { won } from "../lib/format";
import { useLoad } from "../lib/hooks";

/** Edit one product: details, photos, inspection media, stock (step 5). */
export function ProductEdit() {
  const { id = "" } = useParams();
  const back = (useLocation().state as { from?: string } | null)?.from ?? "/products";
  const product = useLoad(() => call(api.GET("/studio-api/products/{id}", { params: { path: { id } } })), [id]);

  if (!product.data) {
    return (
      <div className="page">
        <ErrorBanner text={product.error} onRetry={product.reload} />
        {!product.error && <p className="muted">{t.app.loading}</p>}
      </div>
    );
  }
  const p = product.data;
  const missing = missingDetails(p);

  return (
    <div className="page page-wide">
      <header className="page-head">
        <Link to={back} className="back" aria-label={t.app.close}>
          <Icon name="back" />
        </Link>
        <h1 className="ellipsis">{p.title}</h1>
        <ProductStatusTag status={p.status} />
        {!p.details_complete && <span className="tag tag-warn">{t.products.incomplete}</span>}
      </header>
      {missing.length > 0 && (
        <div className="banner banner-warn" data-testid="missing">
          <span>
            {t.products.missing}: <b>{missing.join(", ")}</b>
          </span>
        </div>
      )}
      <div className="setup-grid">
        <Basics p={p} onSaved={product.setData} />
        <div className="col">
          <MediaCard
            title={t.products.photos}
            hint={t.products.photosHint}
            button={t.products.addPhotos}
            accept="image/jpeg,image/png"
            productId={p.id}
            media={p.media.filter((m) => m.kind === "photo")}
            firstLabel={t.products.mainPhoto}
            kindOf={() => "photo"}
            onChange={product.reload}
            testId="photos"
          />
          <Variants p={p} onSaved={product.setData} />
          <MediaCard
            title={t.products.inspectionMedia}
            hint={t.products.inspectionMediaHint}
            button={t.products.addInspection}
            accept="image/jpeg,image/png,video/mp4,video/quicktime,video/webm"
            productId={p.id}
            media={p.media.filter((m) => m.kind !== "photo")}
            kindOf={(f) => (f.type.startsWith("video/") ? "inspection_video" : "inspection_photo")}
            onChange={product.reload}
          />
        </div>
      </div>
    </div>
  );
}

type SpecRow = { name: string; value: string };

function Basics({ p, onSaved }: { p: Product; onSaved: (p: Product) => void }) {
  const [title, setTitle] = useState(p.title);
  const [price, setPrice] = useState(String(p.price_won));
  const [description, setDescription] = useState(p.description);
  const [status, setStatus] = useState(p.status);
  const [tags, setTags] = useState(p.tags.join(", "));
  const [specs, setSpecs] = useState<SpecRow[]>(() => {
    const rows = Object.entries(p.specs).map(([name, value]) => ({ name, value }));
    return rows.length ? rows : [{ name: "", value: "" }];
  });
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setMsg(undefined);
    const specObj: Record<string, string> = {};
    for (const r of specs) if (r.name.trim()) specObj[r.name.trim()] = r.value.trim();
    try {
      const next = await call(
        api.PATCH("/studio-api/products/{id}", {
          params: { path: { id: p.id } },
          body: {
            title: title.trim(),
            price_won: Number(price),
            description: description.trim(),
            status,
            tags: tags
              .split(",")
              .map((x) => x.trim())
              .filter(Boolean),
            specs: specObj,
          },
        }),
      );
      onSaved(next);
      setTitle(next.title);
      setPrice(String(next.price_won));
      setDescription(next.description);
      setTags(next.tags.join(", "));
      setMsg({ ok: true, text: t.setup.saved });
    } catch (err) {
      setMsg({ ok: false, text: errorText(err) });
    } finally {
      setBusy(false);
    }
  }

  const setSpec = (i: number, patch: Partial<SpecRow>) => setSpecs((rows) => rows.map((r, j) => (j === i ? { ...r, ...patch } : r)));

  return (
    <form className="card section" onSubmit={save} aria-label={t.products.basics}>
      <h2>{t.products.basics}</h2>
      <label className="field">
        <span>{t.products.name}</span>
        <input required maxLength={200} value={title} onChange={(e) => setTitle(e.target.value)} />
      </label>
      <div className="two">
        <label className="field">
          <span>{t.products.price}</span>
          <input required inputMode="numeric" pattern="[0-9]*" value={price} onChange={(e) => setPrice(e.target.value.replace(/\D/g, ""))} />
        </label>
        <label className="field">
          <span>{t.products.status}</span>
          <select value={status} onChange={(e) => setStatus(e.target.value as Product["status"])}>
            {["draft", "live", "hidden", "archived"].map((s) => (
              <option key={s} value={s}>
                {t.productStatus[s]}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p className="muted small">{t.products.statusHint}</p>
      <label className="field">
        <span>{t.products.description}</span>
        <textarea rows={6} maxLength={10000} placeholder={t.products.descriptionPlaceholder} value={description} onChange={(e) => setDescription(e.target.value)} />
      </label>
      <div className="field">
        <span>{t.products.specs}</span>
        <div className="specs">
          {specs.map((r, i) => (
            <div className="spec-row" key={i}>
              <input aria-label={t.products.specName} placeholder={t.products.specNamePlaceholder} maxLength={50} value={r.name} onChange={(e) => setSpec(i, { name: e.target.value })} />
              <input aria-label={t.products.specValue} placeholder={t.products.specValuePlaceholder} maxLength={500} value={r.value} onChange={(e) => setSpec(i, { value: e.target.value })} />
              <button type="button" className="icon-btn" aria-label={t.app.remove} onClick={() => setSpecs((rows) => rows.filter((_, j) => j !== i))}>
                <Icon name="close" size={16} />
              </button>
            </div>
          ))}
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setSpecs((rows) => [...rows, { name: "", value: "" }])} disabled={specs.length >= 30}>
            <Icon name="plus" size={14} />
            {t.products.addSpec}
          </button>
        </div>
      </div>
      <label className="field">
        <span>{t.products.tags}</span>
        <input value={tags} placeholder={t.products.tagsHint} onChange={(e) => setTags(e.target.value)} />
      </label>
      <div className="row">
        <button className="btn btn-primary" disabled={busy || !title.trim() || !price}>
          {busy ? t.app.saving : t.app.save}
        </button>
        {msg && <span className={msg.ok ? "ok" : "error"}>{msg.text}</span>}
      </div>
    </form>
  );
}

function MediaCard({
  title,
  hint,
  button,
  accept,
  productId,
  media,
  firstLabel,
  kindOf,
  onChange,
  testId,
}: {
  title: string;
  hint: string;
  button: string;
  accept: string;
  productId: string;
  media: Media[];
  firstLabel?: string;
  kindOf: (f: File) => string;
  onChange: () => Promise<void>;
  testId?: string;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  async function add(files: FileList | null) {
    if (!files?.length) return;
    setBusy(true);
    setError(undefined);
    try {
      for (const f of Array.from(files)) await upload<Media>(`/studio-api/products/${productId}/media`, f, { kind: kindOf(f) });
    } catch (e) {
      setError(errorText(e));
    }
    if (input.current) input.current.value = "";
    await onChange();
    setBusy(false);
  }

  async function remove(mediaId: string) {
    if (!confirm(t.products.deleteMediaConfirm)) return;
    setError(undefined);
    try {
      await call(api.DELETE("/studio-api/products/{id}/media/{media_id}", { params: { path: { id: productId, media_id: mediaId } } }));
      await onChange();
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <section className="card section" data-testid={testId}>
      <div className="section-head">
        <div>
          <h2>{title}</h2>
          <p className="muted small">{hint}</p>
        </div>
        <span className="count">{media.length}</span>
      </div>
      <Gallery media={media} onDelete={(id) => void remove(id)} empty={t.products.noMedia} firstLabel={firstLabel} />
      <input ref={input} type="file" hidden multiple accept={accept} onChange={(e) => void add(e.target.files)} data-testid={testId ? `${testId}-file` : undefined} />
      <ErrorBanner text={error} />
      <button className="btn btn-secondary btn-sm" disabled={busy} onClick={() => input.current?.click()}>
        <Icon name="upload" size={16} />
        {busy ? t.orders.uploading : button}
      </button>
    </section>
  );
}

function Variants({ p, onSaved }: { p: Product; onSaved: (p: Product) => void }) {
  const [error, setError] = useState<string>();
  const values = new Map(p.options.flatMap((o) => o.values.map((v) => [v.id, v] as const)));
  const allSoldOut = p.variants.every((v) => v.sold_out || (v.stock_mode === "counted" && v.available_qty === 0));

  async function soldOutAll() {
    if (!confirm(t.products.soldOutAllConfirm)) return;
    try {
      onSaved(await call(api.POST("/studio-api/products/{id}/sold-out", { params: { path: { id: p.id } } })));
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <section className="card section">
      <div className="section-head">
        <div>
          <h2>{t.products.variants}</h2>
          {p.options.length > 0 && (
            <p className="muted small">
              {p.options.map((o) => `${o.name}: ${o.values.map((v) => v.label).join(", ")}`).join(" · ")} — {t.products.optionsFixed}
            </p>
          )}
        </div>
        {!allSoldOut && (
          <button className="btn btn-secondary btn-sm" onClick={() => void soldOutAll()}>
            {t.products.soldOutAll}
          </button>
        )}
      </div>
      <ErrorBanner text={error} />
      <table className="table table-tight" data-testid="variants">
        <thead>
          <tr>
            <th>{t.products.variant}</th>
            <th>{t.products.variantPrice}</th>
            <th>{t.products.stockQty}</th>
            <th className="num-col" title={t.products.held}>
              {t.products.held}
            </th>
            <th className="num-col">{t.products.sold}</th>
            <th className="num-col">{t.products.available}</th>
            <th>{t.products.soldOut}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {p.variants.map((v) => (
            <VariantRow
              key={`${v.id}:${v.stock_mode}:${v.stock_qty}:${v.price_override_won}:${v.sold_out}`}
              v={v}
              base={p.price_won}
              label={
                <span className="row-tight">
                  {v.option_value_ids.map((vid) => {
                    const ov = values.get(vid);
                    return ov ? <Swatch key={vid} hex1={ov.hex1} hex2={ov.hex2} size={12} /> : null;
                  })}
                  {v.label || t.products.single}
                </span>
              }
              onSave={async (patch) => {
                setError(undefined);
                try {
                  onSaved(
                    await call(
                      api.PATCH("/studio-api/products/{id}/variants/{variant_id}", {
                        params: { path: { id: p.id, variant_id: v.id } },
                        body: patch,
                      }),
                    ),
                  );
                } catch (e) {
                  setError(errorText(e));
                }
              }}
            />
          ))}
        </tbody>
      </table>
    </section>
  );
}

function VariantRow({ v, base, label, onSave }: { v: Variant; base: number; label: ReactNode; onSave: (p: VariantPatch) => Promise<void> }) {
  const [mode, setMode] = useState(v.stock_mode);
  const [qty, setQty] = useState(String(v.stock_qty));
  const [price, setPrice] = useState(v.price_override_won == null ? "" : String(v.price_override_won));
  const [soldOut, setSoldOut] = useState(v.sold_out);
  const [busy, setBusy] = useState(false);

  const patch: VariantPatch = {};
  if (mode !== v.stock_mode) patch.stock_mode = mode;
  if (mode === "counted" && Number(qty) !== v.stock_qty) patch.stock_qty = Number(qty);
  const curPrice = v.price_override_won == null ? "" : String(v.price_override_won);
  if (price !== curPrice) {
    if (price === "") patch.clear_price_override = true;
    else patch.price_override_won = Number(price);
  }
  if (soldOut !== v.sold_out) patch.sold_out = soldOut;
  const dirty = Object.keys(patch).length > 0;

  return (
    <tr className={v.buyable ? "" : "dim"}>
      <td>{label}</td>
      <td>
        <input
          className="input-sm w-price"
          inputMode="numeric"
          aria-label={t.products.variantPrice}
          placeholder={won(base)}
          value={price}
          onChange={(e) => setPrice(e.target.value.replace(/\D/g, ""))}
        />
      </td>
      <td>
        <div className="row-tight">
          <select className="input-sm" aria-label={t.products.stockMode} value={mode} onChange={(e) => setMode(e.target.value as Variant["stock_mode"])}>
            <option value="counted">{t.products.counted}</option>
            <option value="until_sold_out">{t.products.untilSoldOut}</option>
          </select>
          {mode === "counted" && (
            <input
              className="input-sm w-qty"
              inputMode="numeric"
              aria-label={t.products.stockQty}
              value={qty}
              onChange={(e) => setQty(e.target.value.replace(/\D/g, ""))}
            />
          )}
        </div>
      </td>
      <td className="num-col">{v.held_qty}</td>
      <td className="num-col">{v.sold_qty}</td>
      <td className="num-col strong">{v.available_qty ?? t.products.unlimited}</td>
      <td>
        <input type="checkbox" aria-label={t.products.soldOut} checked={soldOut} onChange={(e) => setSoldOut(e.target.checked)} />
      </td>
      <td>
        <button
          className="btn btn-primary btn-xs"
          aria-label={`${v.label || t.products.single} ${t.products.saveVariant}`}
          disabled={!dirty || busy || (mode === "counted" && qty === "")}
          onClick={async () => {
            setBusy(true);
            await onSave(patch);
            setBusy(false);
          }}
        >
          {t.products.saveVariant}
        </button>
      </td>
    </tr>
  );
}
