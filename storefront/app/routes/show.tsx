import { useEffect, useRef, useState } from "react";
import { Link, useLoaderData, useRevalidator } from "react-router";
import { load, originOf, serverApi, type ItemView, type ShowView } from "../api/client";
import { BuyPanel } from "../components/BuyPanel";
import { ItemCard } from "../components/ItemCard";
import { Icon, LivePill } from "../components/ui";
import { t } from "../i18n/ko";
import { date, dateTime, offset, time, won, ytThumb } from "../lib/format";
import type { Route } from "./+types/show";

export async function loader({ request, params }: Route.LoaderArgs) {
  const show = await load(request, serverApi(request).GET("/api/shows/{id}", { params: { path: { id: params.id } } }));
  return { show, origin: originOf(request) };
}

export function meta({ loaderData: data }: Route.MetaArgs) {
  if (!data) return [];
  const { show, origin } = data;
  const first = show.items.find((i) => i.product.photos[0]);
  const image = first ? origin + first.product.photos[0] : show.youtube_video_id ? ytThumb(show.youtube_video_id) : undefined;
  const desc = show.status === "live" ? t.home.liveNow(show.items.length) : `${t.live.all} ${show.items.length}개`;
  return [
    { title: `${show.title} — ${t.app.name}` },
    { name: "description", content: desc },
    { property: "og:title", content: show.title },
    { property: "og:description", content: desc },
    ...(image ? [{ property: "og:image", content: image }] : []),
  ];
}

type Event = { item?: ItemView };

/**
 * The live page (boards 2, 3, 6): the video stays on top, items arrive over
 * Server-Sent Events, and a card opens the product sheet without leaving the
 * stream. After the stream it is the recap, with the same numbers.
 */
export default function ShowPage() {
  const { show } = useLoaderData<typeof loader>();
  const revalidator = useRevalidator();
  const [items, setItems] = useState<ItemView[]>(show.items);
  const [openId, setOpenId] = useState<string>();
  const [toast, setToast] = useState<ItemView>();
  const [start, setStart] = useState<number>();
  const live = show.status === "live";

  useEffect(() => setItems(show.items), [show.items]);

  // Live updates: the full item comes with every event, so no refetching.
  const revalidate = useRef(revalidator.revalidate);
  revalidate.current = revalidator.revalidate;
  useEffect(() => {
    if (!live) return;
    const es = new EventSource(`/api/shows/${show.id}/events?last_event_id=${show.last_event_id}`);
    const upsert = (it: ItemView) =>
      setItems((list) => {
        const rest = list.filter((x) => x.id !== it.id);
        return [...rest, it].sort((a, b) => a.number - b.number);
      });
    const onItem = (ev: MessageEvent) => {
      const data = JSON.parse(ev.data) as Event;
      if (!data.item) return;
      upsert(data.item);
      if (ev.type === "live.item_published") setToast(data.item);
    };
    for (const ty of ["live.item_published", "live.item_updated", "live.item_pinned", "live.item_unpinned"]) es.addEventListener(ty, onItem);
    for (const ty of ["live.show_started", "live.show_ended", "live.show_updated", "reset"]) es.addEventListener(ty, () => void revalidate.current());
    return () => es.close();
  }, [live, show.id, show.last_event_id]);

  useEffect(() => {
    if (!toast) return;
    const id = setTimeout(() => setToast(undefined), 8000);
    return () => clearTimeout(id);
  }, [toast]);

  const pinned = items.filter((i) => i.pinned);
  const open = items.find((i) => i.id === openId);
  const until = show.ended_at ? new Date(Date.parse(show.ended_at) + show.display_days_after * 86_400_000).toISOString() : undefined;

  function scene(it: ItemView) {
    if (it.broadcast_offset_sec == null) return;
    if (live) window.open(it.replay_url, "_blank", "noopener");
    else {
      setStart(it.broadcast_offset_sec);
      setOpenId(undefined);
      window.scrollTo({ top: 0, behavior: "smooth" });
    }
  }

  return (
    <main className="live-page">
      <div className="live-video">
        <Player show={show} start={start} />
        <div className="live-title">
          <h1>{show.title}</h1>
          <span className="muted small">
            {live ? t.live.started(time(show.started_at)) : show.status === "ended" ? t.live.ended(date(show.started_at)) : t.live.notStarted}
          </span>
          {until && <p className="note">{t.live.recapUntil(dateTime(until))}</p>}
        </div>
      </div>
      <div className="live-items">
        {pinned.length > 0 && (
          <section>
            <div className="sech">
              <h2>
                <Icon name="star" size={16} filled /> {t.live.pinned}
              </h2>
              <span className="muted small">{pinned.length}개</span>
            </div>
            <div className="hscroll pins">
              {pinned.map((it) => (
                <ItemCard key={it.id} item={it} tall onOpen={(x) => setOpenId(x.id)} />
              ))}
            </div>
          </section>
        )}
        <section>
          <div className="sech">
            <h2>
              {t.live.all} <em>{items.length}</em>
            </h2>
            <span className="muted small">{t.live.sort}</span>
          </div>
          {items.length === 0 ? (
            <p className="muted empty">{t.live.empty}</p>
          ) : (
            <div className="grid2" data-testid="items">
              {items.map((it) => (
                <div key={it.id} id={`no-${it.number}`}>
                  <ItemCard item={it} onOpen={(x) => setOpenId(x.id)} />
                </div>
              ))}
            </div>
          )}
        </section>
      </div>
      {toast && !open && (
        <button
          type="button"
          className="toast"
          onClick={() => {
            document.getElementById(`no-${toast.number}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
            setToast(undefined);
          }}
        >
          {t.live.justListed(toast.number)}
          <span>{t.live.see}</span>
        </button>
      )}
      {open && <Sheet item={open} onClose={() => setOpenId(undefined)} onScene={() => scene(open)} />}
    </main>
  );
}

/** YouTube player; the recap can jump to an item's moment. */
function Player({ show, start }: { show: ShowView; start?: number }) {
  const vid = show.youtube_video_id;
  return (
    <div className="video">
      {vid ? (
        <iframe
          key={start ?? "live"}
          src={`https://www.youtube.com/embed/${vid}?playsinline=1&rel=0${show.status === "live" ? "&autoplay=1&mute=1" : ""}${start != null ? `&start=${start}&autoplay=1` : ""}`}
          title={show.title}
          allow="autoplay; encrypted-media; picture-in-picture"
          allowFullScreen
        />
      ) : (
        <div className="video-empty">{t.live.noVideo}</div>
      )}
      {show.status === "live" && (
        <span className="vtop">
          <LivePill size="s" />
        </span>
      )}
    </div>
  );
}

/** The product sheet over the live page (board 3). */
function Sheet({ item, onClose, onScene }: { item: ItemView; onClose: () => void; onScene: () => void }) {
  const p = item.product;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="sheet-wrap" role="dialog" aria-modal="true" aria-label={p.title}>
      <button type="button" className="dim" aria-label={t.app.close} onClick={onClose} />
      <div className="sheet">
        <div className="handle" />
        <div className="sh-top">
          {p.photos[0] ? <img className="shimg" src={p.photos[0]} alt="" /> : <span className="shimg" />}
          <div className="grow" />
          <button type="button" className="icon-link" aria-label={t.app.close} onClick={onClose}>
            <Icon name="close" size={20} />
          </button>
        </div>
        <BuyPanel key={p.id} product={p} number={item.number} header />
        {item.broadcast_offset_sec != null && item.replay_url && (
          <button type="button" className="bcast" onClick={onScene}>
            <Icon name="play" size={14} filled /> {t.product.scene(offset(item.broadcast_offset_sec))}
          </button>
        )}
        <Link className="more" to={`/products/${p.id}`}>
          {t.product.details} <Icon name="chev" size={14} />
        </Link>
        <span className="sr-only">{won(p.price_won)}</span>
      </div>
    </div>
  );
}
