import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ApiError, api, call, errorText, type Ingest, type Product, type StudioItem, type StudioShow } from "../api/client";
import { useAuth } from "../auth";
import { Icon } from "../components/Icon";
import { stockText, Thumb } from "../components/product";
import { CopyButton, ErrorBanner, NumberBadge, StatusTag, Stepper, Toggle } from "../components/ui";
import { t } from "../i18n/ko";
import { fromLocalInput, toLocalInput, won } from "../lib/format";
import { useLoad } from "../lib/hooks";

export function ShowSetup() {
  const { id = "" } = useParams();
  const { can } = useAuth();
  const navigate = useNavigate();
  const show = useLoad(() => call(api.GET("/studio-api/shows/{id}", { params: { path: { id } } })), [id]);
  const [actionError, setActionError] = useState<string>();

  if (!show.data) {
    return (
      <div className="page">
        <ErrorBanner text={show.error} onRetry={show.reload} />
        {!show.error && <p className="muted">{t.app.loading}</p>}
      </div>
    );
  }
  const sh = show.data;
  const editable = can("manager");
  const before = sh.status === "draft" || sh.status === "scheduled";

  async function start() {
    if (!confirm(t.setup.startConfirm)) return;
    try {
      await call(api.POST("/studio-api/shows/{id}/start", { params: { path: { id } } }));
      navigate(`/shows/${id}/live`);
    } catch (e) {
      setActionError(errorText(e));
    }
  }

  return (
    <div className="page">
      <header className="page-head">
        <Link to="/shows" className="back" aria-label={t.shows.title}>
          <Icon name="back" />
        </Link>
        <h1>{sh.title}</h1>
        <StatusTag status={sh.status} />
        <div className="grow" />
        <Link className="btn btn-secondary" to={`/shows/${id}/live`}>
          {t.setup.openConsole}
        </Link>
        {before && (
          <button className="btn btn-live" onClick={() => void start()}>
            {t.setup.start}
          </button>
        )}
      </header>
      <ErrorBanner text={actionError} />
      <div className="setup-grid">
        <div className="col">
          <DetailsForm show={sh} editable={editable} onSaved={(s) => show.setData({ ...sh, ...s })} />
          {editable && <IngestCard showId={id} />}
        </div>
        <Preselected show={sh} editable={editable && before} reload={show.reload} />
      </div>
    </div>
  );
}

function DetailsForm({
  show,
  editable,
  onSaved,
}: {
  show: StudioShow;
  editable: boolean;
  onSaved: (s: Partial<StudioShow>) => void;
}) {
  const [title, setTitle] = useState(show.title);
  const [youtube, setYoutube] = useState(show.youtube_video_id ? `https://youtu.be/${show.youtube_video_id}` : "");
  const [scheduledAt, setScheduledAt] = useState(toLocalInput(show.scheduled_at));
  const [host, setHost] = useState(show.host_name);
  const [days, setDays] = useState(show.display_days_after);
  const [scheduled, setScheduled] = useState(show.status === "scheduled");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();
  const before = show.status === "draft" || show.status === "scheduled";

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setMsg(undefined);
    try {
      const s = await call(
        api.PATCH("/studio-api/shows/{id}", {
          params: { path: { id: show.id } },
          body: {
            title,
            youtube_url: youtube,
            scheduled_at: fromLocalInput(scheduledAt),
            host_name: host,
            display_days_after: days,
            ...(before ? { status: scheduled ? "scheduled" : "draft" } : {}),
          },
        }),
      );
      onSaved(s);
      setMsg({ ok: true, text: t.setup.saved });
    } catch (err) {
      setMsg({ ok: false, text: errorText(err) });
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card section" onSubmit={save}>
      <h2>{t.setup.details}</h2>
      <fieldset disabled={!editable}>
        <label className="field">
          <span>{t.shows.titleField}</span>
          <input required maxLength={200} value={title} onChange={(e) => setTitle(e.target.value)} />
        </label>
        <label className="field">
          <span>{t.setup.youtube}</span>
          <input value={youtube} placeholder="https://youtu.be/…" onChange={(e) => setYoutube(e.target.value)} />
          <small className="muted">{t.setup.youtubeHint}</small>
        </label>
        <div className="two">
          <label className="field">
            <span>{t.setup.scheduledAt}</span>
            <input type="datetime-local" value={scheduledAt} onChange={(e) => setScheduledAt(e.target.value)} />
          </label>
          <label className="field">
            <span>{t.setup.host}</span>
            <input value={host} maxLength={100} onChange={(e) => setHost(e.target.value)} />
          </label>
        </div>
        <div className="field">
          <span>{t.setup.days}</span>
          <Stepper label={t.setup.days} value={days} min={0} max={365} format={t.setup.daysUnit} onChange={setDays} />
          <small className="muted">{t.setup.daysHint}</small>
        </div>
        {before && (
          <div className="field">
            <Toggle checked={scheduled} onChange={setScheduled} label={t.setup.markScheduled} />
            <small className="muted">{t.setup.markScheduledHint}</small>
          </div>
        )}
        {editable && (
          <div className="row">
            <button className="btn btn-primary" disabled={busy}>
              {busy ? t.app.saving : t.app.save}
            </button>
            {msg && <span className={msg.ok ? "ok" : "error"}>{msg.text}</span>}
          </div>
        )}
      </fieldset>
    </form>
  );
}

function IngestCard({ showId }: { showId: string }) {
  const [ingest, setIngest] = useState<Ingest>();
  const [error, setError] = useState<string>();
  const [disabled, setDisabled] = useState(false);

  useEffect(() => {
    call(api.GET("/studio-api/shows/{id}/ingest", { params: { path: { id: showId } } }))
      .then(setIngest)
      .catch((e) => (e instanceof ApiError && e.code === "relay_disabled" ? setDisabled(true) : setError(errorText(e))));
  }, [showId]);

  async function rotate() {
    if (!confirm(t.setup.rotateConfirm)) return;
    try {
      setIngest(await call(api.POST("/studio-api/shows/{id}/ingest/rotate", { params: { path: { id: showId } } })));
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <section className="card section">
      <h2>{t.setup.ingest}</h2>
      {disabled ? (
        <p className="muted">{t.setup.relayDisabled}</p>
      ) : (
        <>
          <p className="muted small">{t.setup.ingestHint}</p>
          <ErrorBanner text={error} />
          {ingest && (
            <>
              <div className="field">
                <span>{t.setup.ingestUrl}</span>
                <div className="copy-row">
                  <code className="mono" data-testid="ingest-url">
                    {ingest.url}
                  </code>
                  <CopyButton text={ingest.url} />
                </div>
              </div>
              <button className="btn btn-secondary btn-sm" onClick={() => void rotate()}>
                <Icon name="refresh" size={16} />
                {t.setup.rotate}
              </button>
            </>
          )}
        </>
      )}
    </section>
  );
}

function Preselected({ show, editable, reload }: { show: StudioShow; editable: boolean; reload: () => Promise<void> }) {
  const [items, setItems] = useState<StudioItem[]>(show.items);
  const [error, setError] = useState<string>();
  const [dragging, setDragging] = useState<string>();
  useEffect(() => setItems(show.items), [show.items]);

  async function saveOrder(next: StudioItem[]) {
    setItems(next);
    try {
      await call(
        api.PUT("/studio-api/shows/{id}/items/order", {
          params: { path: { id: show.id } },
          body: { item_ids: next.map((i) => i.id) },
        }),
      );
      await reload();
    } catch (e) {
      setError(errorText(e));
      await reload();
    }
  }

  function move(from: number, to: number) {
    if (to < 0 || to >= items.length || from === to) return;
    const next = [...items];
    const [it] = next.splice(from, 1);
    next.splice(to, 0, it);
    void saveOrder(next);
  }

  async function remove(itemId: string) {
    try {
      await call(
        api.DELETE("/studio-api/shows/{id}/items/{item_id}", { params: { path: { id: show.id, item_id: itemId } } }),
      );
      await reload();
    } catch (e) {
      setError(errorText(e));
    }
  }

  async function add(productId: string) {
    try {
      await call(api.POST("/studio-api/shows/{id}/items", { params: { path: { id: show.id } }, body: { product_id: productId } }));
      await reload();
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <section className="card section">
      <div className="section-head">
        <div>
          <h2>{t.setup.preselected}</h2>
          <p className="muted small">{editable ? t.setup.preselectedHint : t.setup.preselectedLocked}</p>
        </div>
        <span className="count">{items.length}</span>
      </div>
      <ErrorBanner text={error} />
      {items.length === 0 ? (
        <p className="muted empty">{t.setup.noItems}</p>
      ) : (
        <ol className="pre-list">
          {items.map((it, idx) => (
            <li
              key={it.id}
              className={"pre-row" + (dragging === it.id ? " dragging" : "")}
              draggable={editable}
              onDragStart={() => setDragging(it.id)}
              onDragEnd={() => setDragging(undefined)}
              onDragOver={(e) => editable && e.preventDefault()}
              onDrop={() => {
                const from = items.findIndex((x) => x.id === dragging);
                if (from >= 0) move(from, idx);
                setDragging(undefined);
              }}
            >
              {editable && (
                <span className="grip" aria-hidden="true">
                  <Icon name="grip" size={18} />
                </span>
              )}
              <NumberBadge n={it.number} size="s" />
              <Thumb product={it.product} />
              <div className="pre-main">
                <b>{it.product.title}</b>
                <span className="muted small">
                  {won(it.product.price_won)} · {stockText(it.product)}
                </span>
              </div>
              {editable && (
                <div className="row-actions">
                  <button className="icon-btn" aria-label={t.setup.moveUp} onClick={() => move(idx, idx - 1)}>
                    <Icon name="up" size={18} />
                  </button>
                  <button className="icon-btn" aria-label={t.setup.moveDown} onClick={() => move(idx, idx + 1)}>
                    <Icon name="down" size={18} />
                  </button>
                  <button className="icon-btn" aria-label={t.app.remove} onClick={() => void remove(it.id)}>
                    <Icon name="close" size={18} />
                  </button>
                </div>
              )}
            </li>
          ))}
        </ol>
      )}
      {editable && <AddProducts exclude={items.map((i) => i.product_id)} onAdd={add} />}
    </section>
  );
}

function AddProducts({ exclude, onAdd }: { exclude: string[]; onAdd: (productId: string) => Promise<void> }) {
  const [q, setQ] = useState("");
  const [results, setResults] = useState<Product[]>();
  const [newTitle, setNewTitle] = useState("");
  const [newPrice, setNewPrice] = useState("");
  const [newQty, setNewQty] = useState(1);
  const [error, setError] = useState<string>();

  async function search(e: FormEvent) {
    e.preventDefault();
    try {
      const res = await call(
        api.GET("/studio-api/products", { params: { query: { q, status: "draft,live,hidden", limit: 20 } } }),
      );
      setResults(res.items);
    } catch (err) {
      setError(errorText(err));
    }
  }

  async function create(e: FormEvent) {
    e.preventDefault();
    setError(undefined);
    try {
      const p = await call(
        api.POST("/studio-api/products", {
          body: { title: newTitle, price_won: Number(newPrice), status: "draft", stock: { mode: "counted", qty: newQty } },
        }),
      );
      await onAdd(p.id);
      setNewTitle("");
      setNewPrice("");
      setNewQty(1);
    } catch (err) {
      setError(errorText(err));
    }
  }

  return (
    <div className="add-products">
      <form className="row" onSubmit={search}>
        <input
          className="grow"
          aria-label={t.setup.searchProducts}
          placeholder={t.setup.searchPlaceholder}
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <button className="btn btn-secondary">{t.app.search}</button>
      </form>
      {results && (
        <ul className="results">
          {results.length === 0 && <li className="muted small">{t.setup.noResults}</li>}
          {results
            .filter((p) => !exclude.includes(p.id))
            .map((p) => (
              <li key={p.id} className="result-row">
                <Thumb product={p} size={36} />
                <span className="grow">
                  <b>{p.title}</b> <span className="muted small">{won(p.price_won)}</span>
                </span>
                <button className="btn btn-secondary btn-sm" onClick={() => void onAdd(p.id)}>
                  {t.app.add}
                </button>
              </li>
            ))}
        </ul>
      )}
      <details className="new-product">
        <summary>{t.setup.newProduct}</summary>
        <form className="new-product-form" onSubmit={create}>
          <label className="field grow">
            <span>{t.setup.productTitle}</span>
            <input required maxLength={200} value={newTitle} onChange={(e) => setNewTitle(e.target.value)} />
          </label>
          <label className="field">
            <span>{t.setup.price}</span>
            <input
              required
              inputMode="numeric"
              pattern="[0-9]*"
              value={newPrice}
              onChange={(e) => setNewPrice(e.target.value.replace(/\D/g, ""))}
            />
          </label>
          <div className="field">
            <span>{t.setup.stock}</span>
            <Stepper label={t.setup.stock} value={newQty} min={0} onChange={setNewQty} />
          </div>
          <button className="btn btn-primary">{t.app.add}</button>
        </form>
      </details>
      <ErrorBanner text={error} />
    </div>
  );
}
