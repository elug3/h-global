import { useEffect, useState } from "react";
import { api, call, errorText, upload, type Capture, type FrameInfo, type RelayStatus } from "../../api/client";
import { t } from "../../i18n/ko";
import { Icon } from "../Icon";
import { ErrorBanner, LivePill } from "../ui";

function isTyping(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName);
}

/**
 * The stream column: the relay's live preview, Capture (Space), stepping back
 * through recent frames, and the paste/upload fallbacks.
 */
export function StreamPanel({
  showId,
  live,
  relay,
  relayDisabled,
  captures,
  selected,
  onCapture,
  onSelect,
}: {
  showId: string;
  live: boolean;
  relay?: RelayStatus;
  relayDisabled: boolean;
  captures: Capture[];
  selected?: string;
  onCapture: (c: Capture) => void;
  onSelect: (c: Capture) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [frames, setFrames] = useState<FrameInfo[]>();
  const receiving = !!relay?.receiving;

  async function captureSeq(seq: number) {
    const c = await call(
      api.POST("/studio-api/shows/{id}/captures/relay", { params: { path: { id: showId } }, body: { seq } }),
    );
    onCapture(c);
  }

  async function captureNow() {
    if (busy || !live) return;
    setBusy(true);
    setError(undefined);
    try {
      const res = await call(
        api.GET("/studio-api/shows/{id}/frames", { params: { path: { id: showId }, query: { limit: 1 } } }),
      );
      const latest = res.items[res.items.length - 1];
      if (!latest) throw new Error(t.errors.no_frames);
      await captureSeq(latest.seq);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  async function showFrames() {
    setError(undefined);
    try {
      const res = await call(
        api.GET("/studio-api/shows/{id}/frames", { params: { path: { id: showId }, query: { limit: 12 } } }),
      );
      setFrames(res.items);
    } catch (e) {
      setError(errorText(e));
    }
  }

  async function uploadImage(file: Blob) {
    setBusy(true);
    setError(undefined);
    try {
      onCapture(await upload<Capture>(`/studio-api/shows/${showId}/captures`, file));
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  // Space captures; Ctrl+V pastes an image, unless the user is typing.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code === "Space" && !isTyping(e.target) && receiving) {
        e.preventDefault();
        void captureNow();
      }
    };
    const onPaste = (e: ClipboardEvent) => {
      if (isTyping(e.target) || !live) return;
      const file = [...(e.clipboardData?.files ?? [])].find((f) => f.type.startsWith("image/"));
      if (file) {
        e.preventDefault();
        void uploadImage(file);
      }
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("paste", onPaste);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("paste", onPaste);
    };
  });

  return (
    <section className="col-stream" aria-label={t.console.stream}>
      <h2 className="col-title">{t.console.stream}</h2>
      <div className="preview">
        {receiving ? (
          <img src={`/studio-api/shows/${showId}/preview`} alt={t.console.stream} />
        ) : (
          <div className="preview-empty">
            <Icon name="radio" size={32} />
            <p>{relayDisabled ? t.setup.relayDisabled : t.console.waitingHint}</p>
          </div>
        )}
        {receiving && (
          <div className="preview-tag">
            <LivePill size="s" />
          </div>
        )}
      </div>
      <button
        className="btn btn-primary btn-big btn-full"
        disabled={!live || !receiving || busy}
        onClick={() => void captureNow()}
      >
        <Icon name="camera" size={22} />
        {t.console.capture}
        <kbd>{t.console.captureKey}</kbd>
      </button>
      <div className="row">
        <button className="btn btn-secondary grow" disabled={!live || !receiving} onClick={() => void showFrames()}>
          {t.console.stepBack}
        </button>
        <label className={"btn btn-secondary grow" + (!live ? " disabled" : "")}>
          <Icon name="upload" size={18} />
          {t.console.upload}
          <input
            type="file"
            accept="image/jpeg,image/png"
            hidden
            disabled={!live}
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void uploadImage(f);
              e.target.value = "";
            }}
          />
        </label>
      </div>
      <p className="muted small">
        {t.console.paste} <kbd className="kbd-light">{t.console.pasteKey}</kbd>
      </p>
      <ErrorBanner text={error} />
      {frames && (
        <div className="frames" aria-label={t.console.stepBack}>
          {frames.map((f) => (
            <button key={f.seq} className="frame" onClick={() => void captureSeq(f.seq).catch((e) => setError(errorText(e)))}>
              <img src={f.url} alt="" />
              <span>{f.offset_sec !== null ? `${f.offset_sec}s` : ""}</span>
            </button>
          ))}
        </div>
      )}
      {captures.length > 0 && (
        <>
          <h3 className="col-sub">{t.console.recentCaptures}</h3>
          <div className="captures">
            {captures.map((c) => (
              <button
                key={c.id}
                className={"capture" + (c.id === selected ? " on" : "")}
                onClick={() => onSelect(c)}
                aria-pressed={c.id === selected}
              >
                <img src={c.image_url} alt="" />
              </button>
            ))}
          </div>
        </>
      )}
    </section>
  );
}
