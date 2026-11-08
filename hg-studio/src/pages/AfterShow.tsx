import { useState } from "react";
import { Link, useParams } from "react-router";
import { api, call, errorText, type ItemSales, type ShowSales, type StudioItem, type StudioShow } from "../api/client";
import { useAuth } from "../auth";
import { Icon } from "../components/Icon";
import { isSoldOut, missingDetails, soldOfStock, Thumb } from "../components/product";
import { CopyButton, ErrorBanner, NumberBadge, StatusTag, Stepper, Toggle } from "../components/ui";
import { t } from "../i18n/ko";
import { clock, dateTime, won } from "../lib/format";
import { useLoad } from "../lib/hooks";

/** After the stream (plan 5.7): sales, what stays listed, details to fill, recap (step 5). */
export function AfterShow() {
  const { id = "" } = useParams();
  const show = useLoad(() => call(api.GET("/studio-api/shows/{id}", { params: { path: { id } } })), [id]);
  const sales = useLoad(() => call(api.GET("/studio-api/shows/{id}/sales", { params: { path: { id } } })), [id]);

  if (!show.data) {
    return (
      <div className="page">
        <ErrorBanner text={show.error} onRetry={show.reload} />
        {!show.error && <p className="muted">{t.app.loading}</p>}
      </div>
    );
  }
  const sh = show.data;
  const ended = sh.status === "ended";
  const items = [...sh.items].sort((a, b) => a.number - b.number);
  const perItem = new Map((sales.data?.items ?? []).map((s) => [s.item_id, s]));
  const replaceItem = (it: StudioItem) => show.setData({ ...sh, items: sh.items.map((x) => (x.id === it.id ? it : x)) });

  return (
    <div className="page page-wide">
      <header className="page-head">
        <Link to="/shows" className="back" aria-label={t.shows.title}>
          <Icon name="back" />
        </Link>
        <div className="head-title">
          <h1>{sh.title}</h1>
          <span className="muted small">
            {t.after.title}
            {sh.ended_at ? ` · ${dateTime(sh.ended_at)}` : ""}
          </span>
        </div>
        <StatusTag status={sh.status} />
        <div className="grow" />
        <Link className="btn btn-secondary btn-sm" to={`/shows/${id}/setup`}>
          {t.setup.title}
        </Link>
      </header>
      {!ended && <div className="banner">{t.after.notEnded}</div>}
      <ErrorBanner text={sales.error} onRetry={sales.reload} />
      <Stats show={sh} sales={sales.data} />
      <p className="muted small attribution">{t.after.attribution}</p>
      <div className="after-grid">
        <ItemsTable show={sh} items={items} perItem={perItem} onItem={replaceItem} />
        <div className="col">
          <Period show={sh} onSaved={() => void show.reload()} />
          <Completion showId={id} items={items} />
          <Recap show={sh} items={items} />
        </div>
      </div>
    </div>
  );
}

function Stats({ show, sales }: { show: StudioShow; sales?: ShowSales }) {
  const dur = show.started_at ? (Date.parse(show.ended_at ?? new Date().toISOString()) - Date.parse(show.started_at)) / 1000 : 0;
  const tiles: [string, string, string?][] = [
    [t.after.sales, sales ? won(sales.sales_won) : "…", t.after.salesHint],
    [t.after.orders, sales ? String(sales.orders) : "…"],
    [t.after.units, sales ? String(sales.units) : "…"],
    [t.after.pending, sales ? String(sales.pending_orders) : "…"],
    [t.after.canceled, sales ? String(sales.canceled_orders) : "…"],
    [t.after.duration, show.started_at ? clock(dur) : "—"],
  ];
  return (
    <div className="stats" data-testid="stats">
      {tiles.map(([label, value, hint]) => (
        <div className="card stat" key={label}>
          <span className="muted small">{label}</span>
          <b>{value}</b>
          {hint && <span className="muted small">{hint}</span>}
        </div>
      ))}
    </div>
  );
}

function untilText(it: StudioItem): string {
  if (it.hidden_at) return t.after.hiddenAt(dateTime(it.hidden_at));
  if (it.keep_listed) return t.after.kept;
  return it.hide_at ? dateTime(it.hide_at) : "—";
}

function ItemsTable({
  show,
  items,
  perItem,
  onItem,
}: {
  show: StudioShow;
  items: StudioItem[];
  perItem: Map<string, ItemSales>;
  onItem: (it: StudioItem) => void;
}) {
  const { can } = useAuth();
  const [error, setError] = useState<string>();
  const manager = can("manager");

  async function keep(it: StudioItem, v: boolean) {
    setError(undefined);
    try {
      onItem(await call(api.PATCH("/studio-api/items/{id}", { params: { path: { id: it.id } }, body: { keep_listed: v } })));
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <section className="card section">
      <div className="section-head">
        <h2>{t.after.items}</h2>
        <span className="count">{items.length}</span>
      </div>
      {!manager && <p className="muted small">{t.after.managerOnly}</p>}
      <ErrorBanner text={error} />
      <table className="table table-tight" data-testid="after-items">
        <thead>
          <tr>
            <th>{t.after.colItem}</th>
            <th className="num-col">{t.after.colOrders}</th>
            <th className="num-col">{t.after.colUnits}</th>
            <th className="num-col">{t.after.colSales}</th>
            <th>{t.after.colStock}</th>
            <th>{t.after.colUntil}</th>
            <th>{t.after.colKeep}</th>
            <th>{t.after.colReplay}</th>
          </tr>
        </thead>
        <tbody>
          {items.map((it) => {
            const s = perItem.get(it.id);
            return (
              <tr key={it.id} className={it.hidden_at ? "dim" : ""}>
                <td className="cell-product">
                  <NumberBadge n={it.number} size="xs" />
                  <Thumb product={it.product} size={36} />
                  <span className="cell-title">
                    <Link to={`/products/${it.product_id}`} state={{ from: `/shows/${show.id}/after` }} className="strong ellipsis">
                      {it.product.title}
                    </Link>
                    <span className="muted small">{won(it.product.price_won)}</span>
                  </span>
                </td>
                <td className="num-col">{s?.orders ?? 0}</td>
                <td className="num-col">{s?.units ?? 0}</td>
                <td className="num-col strong">{won(s?.sales_won ?? 0)}</td>
                <td>{isSoldOut(it.product) ? t.console.soldOutLabel : soldOfStock(it.product)}</td>
                <td className="small">{untilText(it)}</td>
                <td>
                  <Toggle
                    checked={it.keep_listed}
                    onChange={(v) => void keep(it, v)}
                    label={<span className="sr-only">{t.after.keepListedLabel(it.number)}</span>}
                    disabled={!manager || !!it.hidden_at}
                  />
                </td>
                <td>
                  {it.replay_url ? (
                    <a className="btn btn-secondary btn-xs" href={it.replay_url} target="_blank" rel="noreferrer">
                      {t.after.replay}
                    </a>
                  ) : (
                    <span className="muted">—</span>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </section>
  );
}

function Period({ show, onSaved }: { show: StudioShow; onSaved: () => void }) {
  const { can } = useAuth();
  const [days, setDays] = useState(show.display_days_after);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();
  const until = show.ended_at ? new Date(Date.parse(show.ended_at) + days * 86_400_000).toISOString() : undefined;

  async function save() {
    setBusy(true);
    setMsg(undefined);
    try {
      await call(api.PATCH("/studio-api/shows/{id}", { params: { path: { id: show.id } }, body: { display_days_after: days } }));
      setMsg({ ok: true, text: t.setup.saved });
      onSaved();
    } catch (e) {
      setMsg({ ok: false, text: errorText(e) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="card section">
      <h2>{t.after.period}</h2>
      <div className="row">
        <Stepper label={t.after.period} value={days} min={0} max={365} format={t.setup.daysUnit} onChange={setDays} />
        {can("manager") && (
          <button className="btn btn-primary btn-sm" disabled={busy || days === show.display_days_after} onClick={() => void save()}>
            {t.app.save}
          </button>
        )}
      </div>
      {until && <p className="strong">{t.after.periodUntil(dateTime(until))}</p>}
      <p className="muted small">{t.after.periodHint}</p>
      {msg && <span className={msg.ok ? "ok" : "error"}>{msg.text}</span>}
    </section>
  );
}

function Completion({ showId, items }: { showId: string; items: StudioItem[] }) {
  const todo = items.filter((it) => !it.product.details_complete);
  return (
    <section className="card section" data-testid="completion">
      <div className="section-head">
        <div>
          <h2>{t.after.completion}</h2>
          <p className="muted small">{t.after.completionHint}</p>
        </div>
        <span className={"count" + (todo.length ? " count-warn" : "")}>{todo.length}</span>
      </div>
      {todo.length === 0 ? (
        <p className="ok">{t.after.completionDone}</p>
      ) : (
        <ul className="todo-list">
          {todo.map((it) => (
            <li key={it.id}>
              <NumberBadge n={it.number} size="xs" />
              <Thumb product={it.product} size={36} />
              <span className="cell-title grow">
                <b className="ellipsis">{it.product.title}</b>
                <span className="muted small">{missingDetails(it.product).join(", ")}</span>
              </span>
              <Link className="btn btn-secondary btn-xs" to={`/products/${it.product_id}`} state={{ from: `/shows/${showId}/after` }}>
                {t.after.fill}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** "12:34" or "1:02:03" from seconds. */
function offsetText(sec: number): string {
  const c = clock(sec);
  return c.startsWith("00:") ? c.slice(3) : c.replace(/^0/, "");
}

export function recapText(show: StudioShow, items: StudioItem[]): string {
  const lines = [t.after.recapHeader(show.title)];
  if (show.watch_url) lines.push(show.watch_url);
  lines.push("");
  for (const it of items) {
    if (it.hidden_at) continue;
    let line = `No.${it.number} ${it.product.title} ${won(it.product.price_won)}`;
    if (it.broadcast_offset_sec != null && show.watch_url) line += ` · ${offsetText(it.broadcast_offset_sec)}`;
    if (isSoldOut(it.product)) line += ` (${t.after.recapSoldOut})`;
    lines.push(line);
  }
  if (show.ended_at) {
    const until = new Date(Date.parse(show.ended_at) + show.display_days_after * 86_400_000).toISOString();
    lines.push("", t.after.recapUntil(dateTime(until)));
  }
  return lines.join("\n");
}

function Recap({ show, items }: { show: StudioShow; items: StudioItem[] }) {
  const text = recapText(show, items);
  return (
    <section className="card section">
      <div className="section-head">
        <div>
          <h2>{t.after.recap}</h2>
          <p className="muted small">{t.after.recapHint}</p>
        </div>
        <CopyButton text={text} />
      </div>
      {!show.watch_url && <p className="error small">{t.after.noYoutube}</p>}
      <textarea className="recap" readOnly rows={Math.min(16, text.split("\n").length + 1)} value={text} aria-label={t.after.recap} data-testid="recap" />
    </section>
  );
}
