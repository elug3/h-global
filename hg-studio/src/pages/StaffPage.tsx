import { useState, type FormEvent } from "react";
import { api, call, errorText, type Staff, type StaffPatch } from "../api/client";
import { useAuth } from "../auth";
import { ErrorBanner, Toggle } from "../components/ui";
import { t } from "../i18n/ko";
import { dateTime } from "../lib/format";
import { useLoad } from "../lib/hooks";

const ROLES = ["cs", "manager", "owner"] as const;

/** Staff accounts: add, change role, deactivate, reset password. Owners only. */
export function StaffPage() {
  const { can, staff: me } = useAuth();
  const list = useLoad(() => (can("owner") ? call(api.GET("/studio-api/staff")) : Promise.resolve({ items: [] as Staff[] })), []);
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState<string>();

  if (!can("owner")) {
    return (
      <div className="page">
        <header className="page-head">
          <h1>{t.staff.title}</h1>
        </header>
        <p className="muted">{t.staff.ownerOnly}</p>
      </div>
    );
  }

  async function patch(s: Staff, body: StaffPatch, done?: string) {
    setError(undefined);
    setNotice(undefined);
    try {
      const next = await call(api.PATCH("/studio-api/staff/{id}", { params: { path: { id: s.id } }, body }));
      list.setData({ items: (list.data?.items ?? []).map((x) => (x.id === next.id ? next : x)) });
      if (done) setNotice(done);
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <div className="page">
      <header className="page-head">
        <h1>{t.staff.title}</h1>
      </header>
      <AddStaff
        onAdded={(s) => {
          list.setData({ items: [...(list.data?.items ?? []), s] });
          setNotice(t.staff.added);
        }}
      />
      <p className="muted small">{t.staff.roleHint}</p>
      <ErrorBanner text={error ?? list.error} />
      {notice && <p className="ok">{notice}</p>}
      <div className="card table-card">
        <table className="table" data-testid="staff">
          <thead>
            <tr>
              <th>{t.staff.name}</th>
              <th>{t.staff.email}</th>
              <th>{t.staff.role}</th>
              <th>{t.staff.status}</th>
              <th>{t.staff.created}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(list.data?.items ?? []).map((s) => (
              <tr key={s.id} className={s.active ? "" : "dim"}>
                <td className="strong">
                  {s.name} {s.id === me?.id && <span className="tag">{t.staff.you}</span>}
                </td>
                <td>{s.email}</td>
                <td>
                  <select
                    className="input-sm"
                    aria-label={`${s.name} ${t.staff.role}`}
                    value={s.role}
                    onChange={(e) => void patch(s, { role: e.target.value as Staff["role"] })}
                  >
                    {ROLES.map((r) => (
                      <option key={r} value={r}>
                        {t.roles[r]}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <Toggle
                    checked={s.active}
                    label={t.staff.active}
                    onChange={(v) => {
                      if (!v && !confirm(t.staff.deactivateConfirm)) return;
                      void patch(s, { active: v });
                    }}
                  />
                </td>
                <td className="muted">{dateTime(s.created_at)}</td>
                <td>
                  <button
                    className="btn btn-secondary btn-xs"
                    onClick={() => {
                      const pw = prompt(t.staff.newPassword);
                      if (pw) void patch(s, { password: pw }, t.staff.passwordChanged);
                    }}
                  >
                    {t.staff.resetPassword}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function AddStaff({ onAdded }: { onAdded: (s: Staff) => void }) {
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState<Staff["role"]>("cs");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      onAdded(await call(api.POST("/studio-api/staff", { body: { email: email.trim(), name: name.trim(), role, password } })));
      setEmail("");
      setName("");
      setPassword("");
      setRole("cs");
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card add-staff" onSubmit={submit} aria-label={t.staff.add}>
      <label className="field">
        <span>{t.staff.name}</span>
        <input required maxLength={100} value={name} onChange={(e) => setName(e.target.value)} />
      </label>
      <label className="field grow">
        <span>{t.staff.email}</span>
        <input required type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
      </label>
      <label className="field">
        <span>{t.staff.role}</span>
        <select value={role} onChange={(e) => setRole(e.target.value as Staff["role"])}>
          {ROLES.map((r) => (
            <option key={r} value={r}>
              {t.roles[r]}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>
          {t.staff.password} <small className="muted">{t.staff.passwordHint}</small>
        </span>
        <input required type="password" minLength={8} maxLength={72} autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </label>
      <button className="btn btn-primary" disabled={busy}>
        {t.staff.add}
      </button>
      {error && <p className="error full-row">{error}</p>}
    </form>
  );
}
