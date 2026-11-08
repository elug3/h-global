import { useEffect, useState, type FormEvent } from "react";
import { api, call, errorText, type Capture, type StudioItem } from "../../api/client";
import { t } from "../../i18n/ko";
import { clock, newKey } from "../../lib/format";
import { useNow } from "../../lib/hooks";
import { Icon } from "../Icon";
import { COLOR, OptionPicker, SIZE, toOptions, type OptionValue } from "../OptionPicker";
import { ErrorBanner, Stepper, Toggle } from "../ui";
import { CropImage, type Rect } from "./CropImage";

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
  const [colors, setColors] = useState<OptionValue[]>([]);
  const [sizes, setSizes] = useState<OptionValue[]>([]);
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
    const options = toOptions(colors, sizes);
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
