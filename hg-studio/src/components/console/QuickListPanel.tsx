import { useEffect, useState, type FormEvent } from "react";
import { api, call, errorText, type Capture, type NewOption, type StudioItem, type Suggestion } from "../../api/client";
import { t } from "../../i18n/ko";
import { clock, newKey } from "../../lib/format";
import { useNow } from "../../lib/hooks";
import { Icon } from "../Icon";
import { ErrorBanner, Stepper, Swatch, Toggle } from "../ui";
import { CropImage, type Rect } from "./CropImage";

// Option names are Korean, like the storefront; suggestions are per name.
const COLOR = "색상";
const SIZE = "사이즈";

type Value = { label: string; hex1?: string | null; hex2?: string | null };

/** Quick listing: crop → price and options → publish (plan 5.2). */
export function QuickListPanel({
  showId,
  nextNumber,
  live,
  capture,
  capturedAt,
  onPublished,
}: {
  showId: string;
  nextNumber: number;
  live: boolean;
  capture?: Capture;
  capturedAt?: number;
  onPublished: (item: StudioItem) => void;
}) {
  const now = useNow();
  const [crop, setCrop] = useState<Rect>();
  const [title, setTitle] = useState("");
  const [price, setPrice] = useState("");
  const [colors, setColors] = useState<Value[]>([]);
  const [sizes, setSizes] = useState<Value[]>([]);
  const [qty, setQty] = useState(1);
  const [unknownQty, setUnknownQty] = useState(false);
  const [key, setKey] = useState(newKey());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [done, setDone] = useState<string>();

  // A new capture starts a new listing.
  useEffect(() => {
    setCrop(undefined);
    setError(undefined);
    setDone(undefined);
    setKey(newKey());
  }, [capture?.id]);

  async function publish(e: FormEvent) {
    e.preventDefault();
    if (!capture) return;
    setBusy(true);
    setError(undefined);
    const options: NewOption[] = [];
    if (colors.length) options.push({ name: COLOR, values: colors.map(toNew) });
    if (sizes.length) options.push({ name: SIZE, values: sizes.map(toNew) });
    try {
      const item = await call(
        api.POST("/studio-api/shows/{id}/items/quick", {
          params: { path: { id: showId }, header: { "Idempotency-Key": key } },
          body: {
            capture_id: capture.id,
            crop,
            title: title.trim() || undefined,
            price_won: Number(price),
            options,
            stock: unknownQty ? { mode: "until_sold_out", qty: 0 } : { mode: "counted", qty },
          },
        }),
      );
      setDone(t.console.published(item.number));
      setTitle("");
      setPrice("");
      setColors([]);
      setSizes([]);
      setQty(1);
      setUnknownQty(false);
      setKey(newKey());
      onPublished(item);
    } catch (err) {
      setError(errorText(err)); // the same key is reused on retry: no double listing
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="ql" aria-label={t.console.quickListing}>
      <div className="ql-head">
        <h2>{t.console.quickListing}</h2>
        {capturedAt && (
          <span className="timer" title="캡처 후 경과 시간">
            <Icon name="clock" size={14} />
            {clock((now - capturedAt) / 1000).slice(3)}
          </span>
        )}
      </div>
      {!capture ? (
        <p className="muted empty">{t.console.noCapture}</p>
      ) : (
        <form onSubmit={publish} className="ql-form">
          <div className="ql-step">
            <span className="sn">1</span>
            {t.console.step1}
            {crop && (
              <button type="button" className="link small" onClick={() => setCrop(undefined)}>
                {t.console.clearCrop}
              </button>
            )}
          </div>
          <CropImage
            src={capture.image_url}
            naturalWidth={capture.width}
            naturalHeight={capture.height}
            crop={crop}
            onCrop={setCrop}
          />
          <p className="muted small">{t.console.step1Hint}</p>

          <div className="ql-step">
            <span className="sn">2</span>
            {t.console.step2}
          </div>
          <label className="field">
            <span>{t.console.name}</span>
            <input value={title} maxLength={200} placeholder={t.console.namePlaceholder(nextNumber)} onChange={(e) => setTitle(e.target.value)} />
          </label>
          <label className="field">
            <span>{t.console.price}</span>
            <div className="price-input">
              <span>₩</span>
              <input
                required
                inputMode="numeric"
                pattern="[0-9]*"
                placeholder={t.console.pricePlaceholder}
                value={price}
                onChange={(e) => setPrice(e.target.value.replace(/\D/g, ""))}
                aria-label={t.console.price}
              />
            </div>
          </label>
          <OptionPicker name={COLOR} label={t.console.color} withColor values={colors} onChange={setColors} refresh={nextNumber} />
          <OptionPicker name={SIZE} label={t.console.size} values={sizes} onChange={setSizes} refresh={nextNumber} />
          <div className="field">
            <span>{t.console.quantity}</span>
            <div className="row">
              {!unknownQty && <Stepper label={t.console.quantity} value={qty} min={0} onChange={setQty} />}
              <Toggle checked={unknownQty} onChange={setUnknownQty} label={t.console.unknownQty} />
            </div>
          </div>
          <ErrorBanner text={error} />
          {done && <p className="ok">{done}</p>}
          <button className="btn btn-primary btn-full" disabled={!live || busy || !price}>
            {busy ? t.console.publishing : t.console.publish(nextNumber)}
          </button>
          <p className="muted small center">{t.console.publishHint}</p>
        </form>
      )}
    </section>
  );
}

function toNew(v: Value) {
  return { label: v.label, hex1: v.hex1 ?? "", hex2: v.hex2 ?? "" };
}

/**
 * Chips for one option. Recently used values come from hg-server (one
 * spelling, one swatch color per value); new values can be added.
 */
function OptionPicker({
  name,
  label,
  values,
  onChange,
  withColor = false,
  refresh,
}: {
  name: string;
  label: string;
  values: Value[];
  onChange: (v: Value[]) => void;
  withColor?: boolean;
  refresh: unknown; // reload suggestions when this changes (after each listing)
}) {
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [adding, setAdding] = useState("");
  const [hex, setHex] = useState("#111111");

  useEffect(() => {
    call(api.GET("/studio-api/catalog/suggestions", { params: { query: { option: name } } }))
      .then((r) => setSuggestions(r.items))
      .catch(() => setSuggestions([]));
  }, [name, refresh]);

  const has = (l: string) => values.some((v) => v.label.toLowerCase() === l.toLowerCase());
  const toggle = (v: Value) =>
    onChange(has(v.label) ? values.filter((x) => x.label.toLowerCase() !== v.label.toLowerCase()) : [...values, v]);
  const chips: Value[] = [...suggestions.map((s) => ({ label: s.label, hex1: s.hex1, hex2: s.hex2 }))];
  for (const v of values) if (!chips.some((c) => c.label.toLowerCase() === v.label.toLowerCase())) chips.push(v);

  return (
    <div className="field">
      <span>{label}</span>
      <div className="chips">
        {chips.map((c) => (
          <button
            type="button"
            key={c.label}
            className={"chip" + (has(c.label) ? " on" : "")}
            aria-pressed={has(c.label)}
            onClick={() => toggle(c)}
          >
            <Swatch hex1={c.hex1} hex2={c.hex2} size={12} />
            {c.label}
          </button>
        ))}
        <span className="chip-add">
          <input
            value={adding}
            maxLength={40}
            placeholder={withColor ? t.console.newColor : t.console.newSize}
            aria-label={withColor ? t.console.newColor : t.console.newSize}
            onChange={(e) => setAdding(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                (e.currentTarget.nextElementSibling?.nextElementSibling as HTMLButtonElement | null)?.click();
              }
            }}
          />
          {withColor ? (
            <input type="color" value={hex} aria-label="색상 코드" onChange={(e) => setHex(e.target.value)} />
          ) : (
            <span />
          )}
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            disabled={!adding.trim()}
            onClick={() => {
              const l = adding.trim();
              if (l && !has(l)) onChange([...values, { label: l, hex1: withColor ? hex : null }]);
              setAdding("");
            }}
          >
            <Icon name="plus" size={14} />
            {t.console.addValue}
          </button>
        </span>
      </div>
    </div>
  );
}
