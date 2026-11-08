import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ApiError, api, call, errorText, type Ingest, type StudioShow, type Suggestion } from "../api/client";
import { useAuth } from "../auth";
import { Icon } from "../components/Icon";
import { COLOR, SIZE } from "../components/OptionPicker";
import { CopyButton, ErrorBanner, StatusTag, Stepper, Swatch, Toggle } from "../components/ui";
import { t } from "../i18n/ko";
import { fromLocalInput, toLocalInput } from "../lib/format";
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
        </div>
        <div className="col">
          {editable && <IngestCard showId={id} />}
          <OnAirListing />
        </div>
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

/** How products get listed: all on air, from No.1, with recently used options. */
function OnAirListing() {
  const [colors, setColors] = useState<Suggestion[]>([]);
  const [sizes, setSizes] = useState<Suggestion[]>([]);
  useEffect(() => {
    const load = (option: string, set: (s: Suggestion[]) => void) =>
      call(api.GET("/studio-api/catalog/suggestions", { params: { query: { option } } }))
        .then((r) => set(r.items.slice(0, 8)))
        .catch(() => set([]));
    load(COLOR, setColors);
    load(SIZE, setSizes);
  }, []);
  const recent = [...colors, ...sizes].filter((v, i, all) => all.findIndex((x) => x.label === v.label) === i);
  return (
    <section className="card section" data-testid="on-air-listing">
      <h2>{t.setup.onAir}</h2>
      <ol className="howto">
        {t.setup.onAirSteps.map((step, i) => (
          <li key={i}>
            <span className="sn">{i + 1}</span>
            {step}
          </li>
        ))}
      </ol>
      <p className="muted small">{t.setup.noPreparation}</p>
      <div className="field">
        <span>{t.setup.recentOptions}</span>
        {recent.length === 0 ? (
          <p className="muted small">{t.setup.noRecentOptions}</p>
        ) : (
          <div className="chips">
            {recent.map((v) => (
              <span key={v.label} className="chip">
                <Swatch hex1={v.hex1} hex2={v.hex2} size={12} />
                {v.label}
              </span>
            ))}
          </div>
        )}
      </div>
    </section>
  );
}
