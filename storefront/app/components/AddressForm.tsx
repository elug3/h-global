import { useEffect, useRef, useState, type FormEvent } from "react";
import type { Shipping } from "../api/client";
import { t } from "../i18n/ko";
import { validPCCC } from "../lib/format";
import { Icon } from "./ui";

const SAVED = "hg-shipping";
const POSTCODE_JS = "https://t1.daumcdn.net/mapjsapi/bundle/postcode/prod/postcode.v2.js";

/** The address saved on this device for the next order, if any. */
export function savedShipping(): Shipping | null {
  try {
    const s = localStorage.getItem(SAVED);
    return s ? (JSON.parse(s) as Shipping) : null;
  } catch {
    return null;
  }
}

export function complete(s: Shipping | null | undefined): s is Shipping {
  return !!s && !!s.recipient && !!s.phone && !!s.postcode && !!s.address1 && validPCCC(s.pccc);
}

/** Address with Korean postcode search and the customs code (board 11). */
export function AddressForm({
  initial,
  busy,
  onSave,
  onCancel,
}: {
  initial: Shipping | null;
  busy: boolean;
  onSave: (s: Shipping, remember: boolean) => void;
  onCancel?: () => void;
}) {
  const [s, setS] = useState<Shipping>({
    recipient: initial?.recipient ?? "",
    phone: initial?.phone ?? "",
    postcode: initial?.postcode ?? "",
    address1: initial?.address1 ?? "",
    address2: initial?.address2 ?? "",
    country: "KR",
    pccc: initial?.pccc ?? "",
  });
  const [remember, setRemember] = useState(true);
  const [searching, setSearching] = useState(false);
  const set = (k: keyof Shipping, v: string) => setS((cur) => ({ ...cur, [k]: v }));
  const pccc = (s.pccc ?? "").toUpperCase().replace(/\s/g, "");
  const pcccOk = validPCCC(pccc);

  function submit(e: FormEvent) {
    e.preventDefault();
    if (!pcccOk) return;
    onSave({ ...s, recipient: s.recipient.trim(), phone: s.phone.trim(), pccc }, remember);
  }

  return (
    <form className="form" onSubmit={submit} aria-label={t.checkout.addressTitle}>
      <label className="fl">
        <span className="flb">{t.checkout.recipient}</span>
        <input className="field" autoComplete="name" required maxLength={50} value={s.recipient} onChange={(e) => set("recipient", e.target.value)} />
      </label>
      <label className="fl">
        <span className="flb">{t.checkout.phone}</span>
        <input className="field" type="tel" autoComplete="tel" required inputMode="tel" placeholder="010-0000-0000" value={s.phone} onChange={(e) => set("phone", e.target.value)} />
      </label>
      <div className="fl">
        <span className="flb">{t.checkout.address}</span>
        <div className="frow">
          <input className="field grow" aria-label={t.checkout.postcode} required inputMode="numeric" placeholder={t.checkout.postcode} value={s.postcode} onChange={(e) => set("postcode", e.target.value)} />
          <button type="button" className="btn btn-sec btn-sm h48" onClick={() => setSearching(true)}>
            {t.checkout.findPostcode}
          </button>
        </div>
        <input className="field" aria-label={t.checkout.address1} required placeholder={t.checkout.address1} value={s.address1} onChange={(e) => set("address1", e.target.value)} />
        <input className="field" aria-label={t.checkout.address2} placeholder={t.checkout.address2} value={s.address2 ?? ""} onChange={(e) => set("address2", e.target.value)} />
      </div>
      <label className="fl">
        <span className="flb">{t.checkout.pccc}</span>
        <span className={"field" + (pcccOk ? " ok" : pccc ? " bad" : "")}>
          <input
            aria-label={t.checkout.pccc}
            required
            maxLength={13}
            autoCapitalize="characters"
            placeholder={t.checkout.pcccPlaceholder}
            value={s.pccc ?? ""}
            onChange={(e) => set("pccc", e.target.value)}
          />
          {pcccOk && (
            <em>
              <Icon name="check" size={16} stroke={2.4} /> {t.checkout.pcccOk}
            </em>
          )}
        </span>
        <span className={"hint" + (pccc && !pcccOk ? " bad" : "")}>{pccc && !pcccOk ? t.checkout.pcccBad : t.checkout.pcccHint}</span>
      </label>
      <a className="help" href="https://unipass.customs.go.kr/csp/persIndex.do" target="_blank" rel="noreferrer">
        <Icon name="link" size={14} /> {t.checkout.pcccLink}
      </a>
      <label className="agree">
        <input type="checkbox" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
        <span className={"cb" + (remember ? " on" : "")} aria-hidden="true">
          {remember && <Icon name="check" size={14} stroke={2.6} />}
        </span>
        <span>{t.checkout.remember}</span>
      </label>
      <div className="frow">
        {onCancel && (
          <button type="button" className="btn btn-sec" onClick={onCancel}>
            {t.app.cancel}
          </button>
        )}
        <button className="btn btn-pri grow" disabled={busy || !pcccOk}>
          {t.checkout.saveAddress}
        </button>
      </div>
      {searching && (
        <PostcodeSearch
          onClose={() => setSearching(false)}
          onPick={(zip, addr) => {
            setS((cur) => ({ ...cur, postcode: zip, address1: addr }));
            setSearching(false);
          }}
        />
      )}
    </form>
  );
}

export function rememberShipping(s: Shipping) {
  try {
    localStorage.setItem(SAVED, JSON.stringify(s));
  } catch {
    /* private mode: nothing saved */
  }
}

type DaumPostcode = new (opts: {
  oncomplete: (d: { zonecode: string; roadAddress: string; jibunAddress: string; buildingName: string }) => void;
  width: string;
  height: string;
}) => { embed: (el: HTMLElement) => void };

/** Kakao (Daum) postcode search, free and without a key, in a bottom sheet. */
function PostcodeSearch({ onPick, onClose }: { onPick: (zip: string, address: string) => void; onClose: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  const pick = useRef(onPick);
  pick.current = onPick;
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    const w = window as unknown as { daum?: { Postcode: DaumPostcode } };
    const show = () => {
      if (!w.daum || !box.current) return setFailed(true);
      new w.daum.Postcode({
        width: "100%",
        height: "100%",
        oncomplete: (d) => pick.current(d.zonecode, (d.roadAddress || d.jibunAddress) + (d.buildingName ? ` (${d.buildingName})` : "")),
      }).embed(box.current);
    };
    if (w.daum) return show();
    const el = document.createElement("script");
    el.src = POSTCODE_JS;
    el.onload = show;
    el.onerror = () => setFailed(true);
    document.head.appendChild(el);
  }, []);
  return (
    <div className="sheet-wrap" role="dialog" aria-modal="true" aria-label={t.checkout.postcodeTitle}>
      <button type="button" className="dim" aria-label={t.app.close} onClick={onClose} />
      <div className="sheet tall">
        <div className="sh-head">
          <b>{t.checkout.postcodeTitle}</b>
          <button type="button" className="icon-link" aria-label={t.app.close} onClick={onClose}>
            <Icon name="close" size={20} />
          </button>
        </div>
        {failed ? <p className="hint">{t.checkout.postcodeFailed}</p> : <div ref={box} className="postcode" />}
      </div>
    </div>
  );
}
