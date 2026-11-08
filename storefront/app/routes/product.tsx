import { useLoaderData, useSearchParams } from "react-router";
import { load, originOf, serverApi } from "../api/client";
import { BuyPanel } from "../components/BuyPanel";
import { Icon, LivePill, NumberBadge } from "../components/ui";
import { t } from "../i18n/ko";
import { dateTime, offset, won } from "../lib/format";
import type { Route } from "./+types/product";

export async function loader({ request, params }: Route.LoaderArgs) {
  const api = serverApi(request);
  const id = params.id;
  const [product, broadcasts] = await Promise.all([
    load(request, api.GET("/api/products/{id}", { params: { path: { id } } })),
    api.GET("/api/products/{id}/broadcasts", { params: { path: { id } } }).then((r) => r.data?.items ?? []),
  ]);
  return { product, broadcasts, origin: originOf(request) };
}

/** Title, price and photo for links shared on KakaoTalk (the reason for server rendering). */
export function meta({ loaderData: data }: Route.MetaArgs) {
  if (!data) return [];
  const { product: p, origin } = data;
  const desc = `${won(p.price_won)}${p.description ? " · " + p.description.slice(0, 80) : ""}`;
  return [
    { title: `${p.title} — ${t.app.name}` },
    { name: "description", content: desc },
    { property: "og:type", content: "product" },
    { property: "og:title", content: `${p.title} ${won(p.price_won)}` },
    { property: "og:description", content: desc },
    ...(p.photos[0] ? [{ property: "og:image", content: origin + p.photos[0] }] : []),
  ];
}

/** Product page (board 4). */
export default function ProductPage() {
  const { product: p, broadcasts } = useLoaderData<typeof loader>();
  const [params] = useSearchParams();
  const latest = broadcasts[0];
  const specs = Object.entries(p.specs);
  return (
    <main className="page product-page">
      <div className="pdp-grid">
        <div className="gal" aria-label="사진">
          <div className="gal-track">
            {p.photos.length === 0 ? (
              <span className="photo photo-empty gal-img" />
            ) : (
              p.photos.map((src, i) => <img key={src} className="gal-img" src={src} alt={`${p.title} ${i + 1}`} />)
            )}
          </div>
          {p.photos.length > 1 && <span className="galc">{t.product.photoCount(1, p.photos.length)}</span>}
        </div>
        <div className="pdp-info">
          {latest && (
            <div className="tagrow">
              <LivePill size="xs" label={t.product.liveItem} />
              <NumberBadge n={latest.number} size="s" />
            </div>
          )}
          <h1 className="ptitle">{p.title}</h1>
          <div className="pprice">{won(p.price_won)}</div>
          <BuyPanel product={p} initialVariant={params.get("buy") ?? undefined} initialQty={Number(params.get("qty")) || 1} />
          {broadcasts.map((b) => (
            <a key={b.show_id} className="bcard" href={b.replay_url || undefined} target="_blank" rel="noreferrer">
              <span className="bthumb">
                <Icon name="play" size={16} filled />
              </span>
              <span className="bct">
                <b>{t.product.shownIn(dateTime(b.shown_at))}</b>
                <span className="muted small">
                  {b.show_title} · No.{b.number}
                </span>
                {b.replay_url && (
                  <span className="bcl">
                    {t.product.sceneShort}
                    {b.broadcast_offset_sec != null && ` (${offset(b.broadcast_offset_sec)})`} <Icon name="chev" size={14} />
                  </span>
                )}
              </span>
            </a>
          ))}
          {(specs.length > 0 || p.description) && (
            <section>
              <h2 className="secttl">{t.product.detailsTitle}</h2>
              {p.description && <p className="desc">{p.description}</p>}
              {specs.length > 0 && (
                <dl className="specs">
                  {specs.map(([k, v]) => (
                    <div key={k} className="spec">
                      <dt>{k}</dt>
                      <dd>{v}</dd>
                    </div>
                  ))}
                </dl>
              )}
            </section>
          )}
          {p.inspection.length > 0 && (
            <section>
              <h2 className="secttl">{t.product.inspection}</h2>
              <p className="isub">
                <Icon name="check" size={16} stroke={2.2} /> {t.product.inspectionHint}
              </p>
              <div className="insp">
                {p.inspection.map((m) =>
                  m.kind.includes("video") ? (
                    <video key={m.url} src={m.url} controls preload="metadata" playsInline />
                  ) : (
                    <a key={m.url} href={m.url} target="_blank" rel="noreferrer">
                      <img src={m.url} alt="" loading="lazy" />
                    </a>
                  ),
                )}
              </div>
            </section>
          )}
        </div>
      </div>
    </main>
  );
}
