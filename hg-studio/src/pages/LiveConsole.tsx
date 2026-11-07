import { useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { ApiError, api, call, errorText, type Capture, type RelayStatus } from "../api/client";
import { useAuth } from "../auth";
import { PublishedList } from "../components/console/PublishedList";
import { QuickListPanel } from "../components/console/QuickListPanel";
import { StreamPanel } from "../components/console/StreamPanel";
import { Icon } from "../components/Icon";
import { ErrorBanner, LivePill, StatusTag } from "../components/ui";
import { t } from "../i18n/ko";
import { clock } from "../lib/format";
import { useEventStream, useInterval, useLoad, useNow } from "../lib/hooks";

/**
 * The live console (plan 6, screen 2): stream and capture, quick listing,
 * and the published list, side by side so the CS rep never changes pages.
 */
export function LiveConsole() {
  const { id = "" } = useParams();
  const { can } = useAuth();
  const now = useNow();
  const show = useLoad(() => call(api.GET("/studio-api/shows/{id}", { params: { path: { id } } })), [id]);
  const [relay, setRelay] = useState<RelayStatus>();
  const [relayDisabled, setRelayDisabled] = useState(false);
  const [captures, setCaptures] = useState<Capture[]>([]);
  const [selected, setSelected] = useState<Capture>();
  const [capturedAt, setCapturedAt] = useState<number>();
  const [actionError, setActionError] = useState<string>();
  const reloadTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

  const sh = show.data;
  const live = sh?.status === "live";

  // Any change to the show (a listing, a sale, a pin) reloads it; changes
  // that arrive together are merged into one reload.
  const scheduleReload = () => {
    clearTimeout(reloadTimer.current);
    reloadTimer.current = setTimeout(() => void show.reload(), 300);
  };
  useEventStream(sh ? `/studio-api/shows/${id}/events` : undefined, sh?.last_event_id, scheduleReload, () => void show.reload());

  useInterval(() => {
    if (!live || relayDisabled) return;
    call(api.GET("/studio-api/shows/{id}/relay", { params: { path: { id } } }))
      .then(setRelay)
      .catch((e) => {
        if (e instanceof ApiError && e.code === "relay_disabled") setRelayDisabled(true);
      });
  }, 2000);

  async function transition(path: "start" | "end") {
    const ask = path === "start" ? t.setup.startConfirm : t.console.endConfirm;
    if (!confirm(ask)) return;
    setActionError(undefined);
    try {
      await call(
        path === "start"
          ? api.POST("/studio-api/shows/{id}/start", { params: { path: { id } } })
          : api.POST("/studio-api/shows/{id}/end", { params: { path: { id } } }),
      );
      await show.reload();
    } catch (e) {
      setActionError(errorText(e));
    }
  }

  if (!sh) {
    return (
      <div className="page">
        <ErrorBanner text={show.error} onRetry={show.reload} />
        {!show.error && <p className="muted">{t.app.loading}</p>}
      </div>
    );
  }

  const started = sh.started_at ? (now - Date.parse(sh.started_at)) / 1000 : 0;
  const push = relay?.youtube_push
    ? relay.push_running && relay.push_restarts === 0
      ? t.console.youtubeOn
      : t.console.youtubeRetrying(relay.push_restarts)
    : t.console.youtubeOff;

  return (
    <div className="console">
      <header className="console-top">
        <Link to={`/shows/${id}/setup`} className="back" aria-label={t.setup.title}>
          <Icon name="back" />
        </Link>
        <div className="console-title">
          <b>{sh.title}</b>
          <span className="muted small">{t.app.name}</span>
        </div>
        <div className="grow" />
        {live ? (
          <>
            <LivePill />
            <span className="mono" title={t.console.elapsed}>
              {clock(started)}
            </span>
            <span className={"chip-status" + (relay?.receiving ? " ok" : "")} data-testid="relay-status">
              {relayDisabled ? t.console.relayOff : relay?.receiving ? t.console.relayReceiving : t.console.relayWaiting}
            </span>
            {!relayDisabled && <span className="chip-status">{push}</span>}
            <span className="muted small">
              {t.console.viewers}: {t.console.viewersSoon}
            </span>
            <button className="btn btn-secondary btn-sm" onClick={() => void transition("end")}>
              {t.console.end}
            </button>
          </>
        ) : (
          <StatusTag status={sh.status} />
        )}
      </header>
      {!live && (
        <div className="banner">
          <span>{sh.status === "ended" ? t.console.ended : t.console.notLive}</span>
          {(sh.status === "draft" || sh.status === "scheduled") && can("cs") && (
            <button className="btn btn-live btn-sm" onClick={() => void transition("start")}>
              {t.setup.start}
            </button>
          )}
        </div>
      )}
      <ErrorBanner text={actionError} />
      <div className="console-cols">
        <StreamPanel
          showId={id}
          live={live}
          relay={relay}
          relayDisabled={relayDisabled}
          captures={captures}
          selected={selected?.id}
          onCapture={(c) => {
            setCaptures((list) => [c, ...list].slice(0, 8));
            setSelected(c);
            setCapturedAt(Date.now());
          }}
          onSelect={(c) => setSelected(c)}
        />
        <QuickListPanel
          showId={id}
          nextNumber={sh.next_number}
          live={live}
          capture={selected}
          capturedAt={capturedAt}
          onPublished={() => void show.reload()}
        />
        <PublishedList items={sh.items} onChanged={() => void show.reload()} />
      </div>
    </div>
  );
}
