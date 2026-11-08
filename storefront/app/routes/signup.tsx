import { useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { api, call, errorText } from "../api/client";
import { FlowHeader, safeNext } from "../components/FlowHeader";
import { ErrorBanner, Icon } from "../components/ui";
import { t } from "../i18n/ko";

export function meta() {
  return [{ title: `${t.auth.signup} — ${t.app.name}` }];
}

/** Email sign-up (board 10). Coming from Buy now, it shows what is being bought. */
export default function Signup() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const next = safeNext(params.get("next"));
  const buying = params.get("product");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [agree, setAgree] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      await call(api.POST("/api/auth/register", { body: { name: name.trim(), email: email.trim(), password } }));
      navigate(next, { replace: true });
    } catch (err) {
      setError(errorText(err));
      setBusy(false);
    }
  }

  return (
    <>
      <FlowHeader title={t.auth.signup} />
      <main className="page narrow">
        <form className="form" onSubmit={submit}>
          {buying && (
            <div className="ctx">
              <b>{t.auth.buyingContext(buying)}</b>
              <span>{t.auth.buyingHint}</span>
            </div>
          )}
          <label className="fl">
            <span className="flb">{t.auth.name}</span>
            <input className="field" autoComplete="name" required maxLength={100} value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="fl">
            <span className="flb">{t.auth.email}</span>
            <input className="field" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </label>
          <label className="fl">
            <span className="flb">{t.auth.password}</span>
            <input
              className={"field" + (password.length >= 8 ? " ok" : "")}
              type="password"
              autoComplete="new-password"
              required
              minLength={8}
              maxLength={72}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <span className={"hint" + (password.length >= 8 ? " ok" : "")}>{password.length >= 8 ? t.auth.passwordOk : t.auth.passwordHint}</span>
          </label>
          <label className="agree">
            <input type="checkbox" checked={agree} onChange={(e) => setAgree(e.target.checked)} />
            <span className={"cb" + (agree ? " on" : "")} aria-hidden="true">
              {agree && <Icon name="check" size={14} stroke={2.6} />}
            </span>
            <span>{t.auth.agree}</span>
          </label>
          <ErrorBanner text={error} />
          <button className="btn btn-pri full" disabled={busy || !agree}>
            {t.auth.submitSignup}
          </button>
          <div className="links">
            <span className="mutedl">{t.auth.haveAccount}</span>
            <Link to={`/login?next=${encodeURIComponent(next)}`}>{t.auth.toLogin}</Link>
          </div>
        </form>
      </main>
    </>
  );
}
