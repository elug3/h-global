import { Link, useLoaderData } from "react-router";
import { serverApi, type Show, type ShowView } from "../api/client";
import { ProductCard } from "../components/ItemCard";
import { Icon, LivePill } from "../components/ui";
import { t } from "../i18n/ko";
import { date, dateTime, ytThumb } from "../lib/format";
import type { Route } from "./+types/home";

export async function loader({ request }: Route.LoaderArgs) {
  const api = serverApi(request);
  const [current, past, products] = await Promise.all([
    api.GET("/api/shows/current").then((r) => r.data?.show ?? null),
    api.GET("/api/shows", { params: { query: { status: "ended", limit: 10 } } }).then((r) => r.data?.items ?? []),
    api.GET("/api/products", { params: { query: { limit: 20 } } }).then((r) => r.data?.items ?? []),
  ]);
  return { current, past, products, now: Date.now() };
}

/**
 * Home (boards 1 and 15): the live banner during a stream; otherwise the next
 * stream, only if staff marked one "방송 예정으로 공개"; then past streams and
 * products still on sale.
 */
export default function Home() {
  const { current, past, products, now } = useLoaderData<typeof loader>();
  return (
    <main className="page">
      {current?.status === "live" && <LiveBanner show={current} />}
      {current?.status === "scheduled" && <NextShow show={current} now={now} />}
      {past.length > 0 && (
        <section>
          <div className="sech">
            <h2>{t.home.past}</h2>
          </div>
          <div className="hscroll">
            {past.map((s: Show) => (
              <Link key={s.id} to={`/shows/${s.id}`} className="recap">
                {s.youtube_video_id ? <img src={ytThumb(s.youtube_video_id)} alt="" loading="lazy" /> : <span className="recap-empty" />}
                <span className="rt">{s.title}</span>
                <span className="rs">{t.home.pastItem(date(s.started_at ?? s.scheduled_at))}</span>
              </Link>
            ))}
          </div>
        </section>
      )}
      <section>
        <div className="sech">
          <h2>{t.home.onSale}</h2>
        </div>
        {products.length === 0 ? (
          <p className="muted empty">{t.home.empty}</p>
        ) : (
          <div className="grid2">
            {products.map((p) => (
              <ProductCard key={p.id} product={p} />
            ))}
          </div>
        )}
      </section>
    </main>
  );
}

function LiveBanner({ show }: { show: ShowView }) {
  return (
    <Link to={`/shows/${show.id}`} className="livebanner" data-testid="live-banner">
      <span className="lb-video">
        {show.youtube_video_id && <img src={ytThumb(show.youtube_video_id)} alt="" />}
        <span className="vtop">
          <LivePill size="s" />
        </span>
      </span>
      <span className="lbb">
        <span>
          <span className="lbt">{show.title}</span>
          <span className="lbs">{t.home.liveNow(show.items.length)}</span>
        </span>
        <span className="btn btn-live">{t.home.watchAndShop}</span>
      </span>
    </Link>
  );
}

function NextShow({ show, now }: { show: ShowView; now: number }) {
  const at = show.scheduled_at ? Date.parse(show.scheduled_at) : undefined;
  const mins = at ? Math.max(0, Math.round((at - now) / 60000)) : undefined;
  return (
    <section className="nextbig" data-testid="next-show">
      <div className="nb-top">
        <span className="nb-tag">{t.home.next}</span>
        {mins !== undefined && (
          <span className="nb-cd">
            <Icon name="clock" size={16} />
            {mins === 0 ? t.home.startsSoon : t.home.countdown(Math.floor(mins / 60), mins % 60)}
          </span>
        )}
      </div>
      <div className="nb-t">{show.title}</div>
      {show.scheduled_at && <div className="nb-s">{dateTime(show.scheduled_at)} · YouTube 라이브</div>}
      <div className="nb-more">{t.home.nextHint}</div>
      <div className="nb-btns">
        <span className="btn btn-white grow dis" aria-disabled="true">
          <Icon name="bell" size={16} />
          {t.home.remind}
        </span>
        {show.watch_url && (
          <a className="btn btn-ghost" href={show.watch_url} target="_blank" rel="noreferrer">
            {t.home.onYoutube}
          </a>
        )}
      </div>
    </section>
  );
}
