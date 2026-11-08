import { useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { api, call, errorText } from "../api/client";
import { FlowHeader, safeNext } from "../components/FlowHeader";
import { ErrorBanner } from "../components/ui";
import { t } from "../i18n/ko";

export function meta() {
  return [{ title: `${t.auth.login} — ${t.app.name}` }];
}

/** Email sign-in (board 10). Kakao and phone sign-in come later (D3). */
export default function Login() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const next = safeNext(params.get("next"));
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [forgot, setForgot] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      await call(api.POST("/api/auth/login", { body: { email: email.trim(), password } }));
      navigate(next, { replace: true });
    } catch (err) {
      setError(errorText(err));
      setBusy(false);
    }
  }

  return (
    <>
      <FlowHeader title={t.auth.login} />
      <main className="page narrow">
        <form className="form" onSubmit={submit}>
          <div className="logo big">{t.app.name}</div>
          <p className="lead">{t.auth.lead}</p>
          <label className="fl">
            <span className="flb">{t.auth.email}</span>
            <input className="field" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <label className="fl">
            <span className="flb">{t.auth.password}</span>
            <input className="field" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
          <ErrorBanner text={error} />
          <button className="btn btn-pri full" disabled={busy}>
            {t.auth.submitLogin}
          </button>
          <div className="links">
            <Link to={`/signup?next=${encodeURIComponent(next)}`}>{t.auth.toSignup}</Link>
            <span>·</span>
            <button type="button" className="linkbtn mutedl" onClick={() => setForgot(true)}>
              {t.auth.forgot}
            </button>
          </div>
          {forgot && <p className="note center">{t.auth.forgotHint}</p>}
          <div className="or">
            <span>{t.auth.soon}</span>
          </div>
          <button type="button" className="btn btn-kakao full" disabled>
            {t.auth.kakao}
          </button>
        </form>
      </main>
    </>
  );
}
