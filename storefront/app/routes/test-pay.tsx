import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { api, call, errorText } from "../api/client";
import { FlowHeader } from "../components/FlowHeader";
import { ErrorBanner } from "../components/ui";
import { t } from "../i18n/ko";

export function meta() {
  return [{ title: `${t.pay.testTitle} — ${t.app.name}` }, { name: "robots", content: "noindex" }];
}

/**
 * Stands in for the payment provider's card page while hg-server runs the
 * fake provider (plan D2: the provider is chosen after the business permit).
 * With a real provider, checkout sends the buyer to its page instead.
 */
export default function TestPay() {
  const { paymentId = "" } = useParams();
  const [params] = useSearchParams();
  const order = params.get("order") ?? "";
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [declined, setDeclined] = useState(false);
  const [error, setError] = useState<string>();

  async function run(kind: "approve" | "fail") {
    setBusy(true);
    setError(undefined);
    try {
      if (kind === "approve") {
        await call(api.POST("/api/dev/payments/{id}/approve", { params: { path: { id: paymentId } } }));
        navigate(`/orders/${order}/done`, { replace: true });
      } else {
        await call(api.POST("/api/dev/payments/{id}/fail", { params: { path: { id: paymentId } } }));
        setDeclined(true);
        setBusy(false);
      }
    } catch (e) {
      setError(errorText(e));
      setBusy(false);
    }
  }

  async function retry() {
    setBusy(true);
    try {
      const res = await call(api.POST("/api/orders/{id}/pay", { params: { path: { id: order } } }));
      setDeclined(false);
      navigate(`/pay/${res.payment.id}?order=${order}`, { replace: true });
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <FlowHeader title={t.pay.testTitle} />
      <main className="page narrow">
        <div className="form">
          <p className="note">{t.pay.testHint}</p>
          {declined ? (
            <>
              <p className="err">{t.pay.declined}</p>
              <button type="button" className="btn btn-pri full" disabled={busy} onClick={() => void retry()}>
                {t.pay.retry}
              </button>
            </>
          ) : (
            <>
              <button type="button" className="btn btn-pri full" disabled={busy} onClick={() => void run("approve")}>
                {t.pay.approve}
              </button>
              <button type="button" className="btn btn-sec full" disabled={busy} onClick={() => void run("fail")}>
                {t.pay.decline}
              </button>
            </>
          )}
          <ErrorBanner text={error} />
        </div>
      </main>
    </>
  );
}
