import { useNavigate } from "react-router";
import { t } from "../i18n/ko";
import { Icon } from "./ui";

/** Header for focused flows (sign-in, checkout, payment): back and a title. */
export function FlowHeader({ title, back }: { title: string; back?: string }) {
  const navigate = useNavigate();
  return (
    <header className="mh">
      <button
        type="button"
        className="icon-link"
        aria-label={t.app.back}
        onClick={() => (back ? navigate(back) : window.history.length > 1 ? navigate(-1) : navigate("/"))}
      >
        <Icon name="back" />
      </button>
      <h1 className="mttl">{title}</h1>
      <span style={{ width: 32 }} />
    </header>
  );
}

/** A same-site path to return to after sign-in; anything else goes home. */
export function safeNext(next: string | null): string {
  return next && next.startsWith("/") && !next.startsWith("//") ? next : "/";
}
