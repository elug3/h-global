import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { api, call, errorText, type Order, type OrderCounts } from "../api/client";
import { linesSummary, ORDER_STATUSES, OrderStatusTag, TODO_STATUSES } from "../components/order";
import { ErrorBanner } from "../components/ui";
import { t } from "../i18n/ko";
import { dateTime, won } from "../lib/format";
import { ORDER_EVENTS, useDebounced, useEventStream } from "../lib/hooks";

const PAGE = 50;

type Tab = { key: string; label: string; statuses: readonly string[]; hint?: string };
const TABS: Tab[] = [
  { key: "todo", label: t.orders.tabTodo, statuses: TODO_STATUSES, hint: t.orders.tabTodoHint },
  ...ORDER_STATUSES.map((s) => ({ key: s, label: t.orderStatus[s], statuses: [s] })),
  { key: "all", label: t.orders.tabAll, statuses: [] },
];

function countFor(tab: Tab, counts?: OrderCounts): number | undefined {
  if (!counts) return undefined;
  const c = counts as Record<string, number>;
  const list = tab.statuses.length ? tab.statuses : ORDER_STATUSES;
  return list.reduce((n, s) => n + (c[s] ?? 0), 0);
}

/** Orders by status, newest first, with search and live updates (step 4). */
export function Orders() {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const tab = TABS.find((x) => x.key === params.get("tab")) ?? TABS[0];
  const query = params.get("q") ?? "";
  const [q, setQ] = useState(query);
  const [orders, setOrders] = useState<Order[]>([]);
  const [counts, setCounts] = useState<OrderCounts>();
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  const fetchPage = useCallback(
    (offset: number, limit: number) =>
      call(
        api.GET("/studio-api/orders", {
          params: {
            query: {
              status: tab.statuses.length ? tab.statuses.join(",") : undefined,
              q: query || undefined,
              limit,
              offset,
            },
          },
        }),
      ),
    [tab, query],
  );

  // First page (or everything shown so far, after a live update).
  const reload = useCallback(
    async (keep = 0) => {
      setLoading(true);
      try {
        const limit = Math.min(200, Math.max(PAGE, keep));
        const [res, c] = await Promise.all([fetchPage(0, limit), call(api.GET("/studio-api/orders/counts"))]);
        setOrders(res.items);
        setHasMore(res.items.length === limit);
        setCounts(c.counts);
        setError(undefined);
      } catch (e) {
        setError(errorText(e));
      } finally {
        setLoading(false);
      }
    },
    [fetchPage],
  );

  useEffect(() => {
    void reload();
  }, [reload]);

  const live = useDebounced(() => void reload(orders.length), 400);
  useEventStream("/studio-api/orders/events", "now", ORDER_EVENTS, live, live);

  async function more() {
    try {
      const res = await fetchPage(orders.length, PAGE);
      setOrders((list) => [...list, ...res.items]);
      setHasMore(res.items.length === PAGE);
    } catch (e) {
      setError(errorText(e));
    }
  }

  function select(key: string) {
    const next = new URLSearchParams(params);
    next.set("tab", key);
    setParams(next, { replace: true });
  }

  function search(e: FormEvent) {
    e.preventDefault();
    const next = new URLSearchParams(params);
    if (q.trim()) next.set("q", q.trim());
    else next.delete("q");
    setParams(next, { replace: true });
  }

  return (
    <div className="page page-wide">
      <header className="page-head">
        <h1>{t.orders.title}</h1>
        <span className="muted small">{t.orders.liveHint}</span>
        <div className="grow" />
        <form className="row" onSubmit={search} role="search">
          <input
            className="search"
            type="search"
            aria-label={t.orders.search}
            placeholder={t.orders.searchPlaceholder}
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <button className="btn btn-secondary">{t.app.search}</button>
        </form>
      </header>
      <div className="filter-tabs" role="tablist" aria-label={t.orders.status}>
        {TABS.map((x) => {
          const n = countFor(x, counts);
          return (
            <button
              key={x.key}
              role="tab"
              aria-selected={x.key === tab.key}
              className={x.key === tab.key ? "on" : ""}
              title={x.hint}
              onClick={() => select(x.key)}
            >
              {x.label}
              {n !== undefined && <em className={x.key === "todo" && n > 0 ? "hot" : ""}>{n}</em>}
            </button>
          );
        })}
      </div>
      <ErrorBanner text={error} onRetry={() => void reload()} />
      {loading && orders.length === 0 ? (
        <p className="muted">{t.app.loading}</p>
      ) : orders.length === 0 ? (
        <p className="muted empty">{t.orders.empty}</p>
      ) : (
        <div className="card table-card">
          <table className="table" data-testid="orders">
            <thead>
              <tr>
                <th>{t.orders.number}</th>
                <th>{t.orders.orderedAt}</th>
                <th>{t.orders.items}</th>
                <th>{t.orders.recipient}</th>
                <th className="num-col">{t.orders.total}</th>
                <th>{t.orders.method}</th>
                <th>{t.orders.status}</th>
              </tr>
            </thead>
            <tbody>
              {orders.map((o) => (
                <tr key={o.id} className="clickable" onClick={() => navigate(`/orders/${o.id}`)}>
                  <td>
                    <Link className="mono strong" to={`/orders/${o.id}`} onClick={(e) => e.stopPropagation()}>
                      {o.number}
                    </Link>
                  </td>
                  <td className="muted">{dateTime(o.created_at)}</td>
                  <td className="cell-product">
                    {o.lines[0]?.photo_url ? (
                      <img className="thumb" src={o.lines[0].photo_url} width={32} height={32} alt="" />
                    ) : (
                      <span className="thumb thumb-empty" style={{ width: 32, height: 32 }} />
                    )}
                    <span className="ellipsis">{linesSummary(o)}</span>
                  </td>
                  <td>{o.shipping.recipient}</td>
                  <td className="num-col strong">{won(o.total_won)}</td>
                  <td className="muted">{t.orders.methods[o.payment_method] ?? o.payment_method}</td>
                  <td>
                    <OrderStatusTag status={o.status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {hasMore && (
            <div className="table-foot">
              <button className="btn btn-secondary btn-sm" onClick={() => void more()}>
                {t.orders.more}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
