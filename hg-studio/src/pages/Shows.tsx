import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router";
import { api, call, errorText, type Show } from "../api/client";
import { useAuth } from "../auth";
import { ErrorBanner, StatusTag } from "../components/ui";
import { t } from "../i18n/ko";
import { dateTime } from "../lib/format";
import { useLoad } from "../lib/hooks";

export function Shows() {
  const { can } = useAuth();
  const navigate = useNavigate();
  const { data, error, loading, reload } = useLoad(
    () => call(api.GET("/studio-api/shows", { params: { query: { limit: 50 } } })),
    [],
  );
  const [title, setTitle] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string>();

  async function create(e: FormEvent) {
    e.preventDefault();
    setCreating(true);
    setCreateError(undefined);
    try {
      const sh = await call(api.POST("/studio-api/shows", { body: { title } }));
      navigate(`/shows/${sh.id}/setup`);
    } catch (err) {
      setCreateError(errorText(err));
    } finally {
      setCreating(false);
    }
  }

  const shows = data?.items ?? [];
  return (
    <div className="page">
      <header className="page-head">
        <h1>{t.shows.title}</h1>
      </header>
      {can("manager") && (
        <form className="card create-show" onSubmit={create}>
          <label className="field grow">
            <span>{t.shows.createTitle}</span>
            <input
              required
              maxLength={200}
              placeholder={t.shows.titlePlaceholder}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </label>
          <button className="btn btn-primary" disabled={creating || !title.trim()}>
            {t.shows.create}
          </button>
          {createError && <p className="error">{createError}</p>}
        </form>
      )}
      <ErrorBanner text={error} onRetry={reload} />
      {loading && !data ? (
        <p className="muted">{t.app.loading}</p>
      ) : shows.length === 0 ? (
        <p className="muted empty">{t.shows.empty}</p>
      ) : (
        <ul className="show-list">
          {shows.map((sh: Show) => (
            <li key={sh.id} className="card show-row">
              <StatusTag status={sh.status} />
              <div className="show-row-main">
                <b>{sh.title}</b>
                <span className="muted small">
                  {sh.status === "live" || sh.status === "ended"
                    ? `${t.shows.started} ${dateTime(sh.started_at)}`
                    : sh.scheduled_at
                      ? `${t.shows.scheduledAt} ${dateTime(sh.scheduled_at)}`
                      : ""}
                </span>
              </div>
              <span className="muted small">{t.shows.items(sh.next_number - 1)}</span>
              <Link className="btn btn-secondary btn-sm" to={`/shows/${sh.id}/setup`}>
                {t.shows.openSetup}
              </Link>
              {(sh.status === "live" || sh.status === "scheduled" || sh.status === "draft") && (
                <Link className={"btn btn-sm " + (sh.status === "live" ? "btn-live" : "btn-primary")} to={`/shows/${sh.id}/live`}>
                  {t.shows.openConsole}
                </Link>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
