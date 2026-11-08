import { useEffect, useState } from "react";
import { api, call, type NewOption, type Suggestion } from "../api/client";
import { t } from "../i18n/ko";
import { Icon } from "./Icon";
import { Swatch } from "./ui";

// Option names are Korean, like the storefront; suggestions are per name.
export const COLOR = "색상";
export const SIZE = "사이즈";

export type OptionValue = { label: string; hex1?: string | null; hex2?: string | null };

/** Colors and sizes picked in the form → the options of a new product. */
export function toOptions(colors: OptionValue[], sizes: OptionValue[]): NewOption[] {
  const toNew = (v: OptionValue) => ({ label: v.label, hex1: v.hex1 ?? "", hex2: v.hex2 ?? "" });
  const options: NewOption[] = [];
  if (colors.length) options.push({ name: COLOR, values: colors.map(toNew) });
  if (sizes.length) options.push({ name: SIZE, values: sizes.map(toNew) });
  return options;
}

/**
 * Chips for one option. Recently used values come from hg-server (one
 * spelling, one swatch color per value); new values can be added.
 */
export function OptionPicker({
  name,
  label,
  values,
  onChange,
  withColor = false,
  refresh,
}: {
  name: string;
  label: string;
  values: OptionValue[];
  onChange: (v: OptionValue[]) => void;
  withColor?: boolean;
  refresh?: unknown; // reload suggestions when this changes (after each listing)
}) {
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [adding, setAdding] = useState("");
  const [hex, setHex] = useState("#111111");

  useEffect(() => {
    call(api.GET("/studio-api/catalog/suggestions", { params: { query: { option: name } } }))
      .then((r) => setSuggestions(r.items))
      .catch(() => setSuggestions([]));
  }, [name, refresh]);

  const has = (l: string) => values.some((v) => v.label.toLowerCase() === l.toLowerCase());
  const toggle = (v: OptionValue) =>
    onChange(has(v.label) ? values.filter((x) => x.label.toLowerCase() !== v.label.toLowerCase()) : [...values, v]);
  const chips: OptionValue[] = [...suggestions.map((s) => ({ label: s.label, hex1: s.hex1, hex2: s.hex2 }))];
  for (const v of values) if (!chips.some((c) => c.label.toLowerCase() === v.label.toLowerCase())) chips.push(v);

  function add() {
    const l = adding.trim();
    if (l && !has(l)) onChange([...values, { label: l, hex1: withColor ? hex : null }]);
    setAdding("");
  }

  return (
    <div className="field">
      <span>{label}</span>
      <div className="chips">
        {chips.map((c) => (
          <button
            type="button"
            key={c.label}
            className={"chip" + (has(c.label) ? " on" : "")}
            aria-pressed={has(c.label)}
            onClick={() => toggle(c)}
          >
            <Swatch hex1={c.hex1} hex2={c.hex2} size={12} />
            {c.label}
          </button>
        ))}
        <span className="chip-add">
          <input
            value={adding}
            maxLength={40}
            placeholder={withColor ? t.console.newColor : t.console.newSize}
            aria-label={withColor ? t.console.newColor : t.console.newSize}
            onChange={(e) => setAdding(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                add();
              }
            }}
          />
          {withColor && <input type="color" value={hex} aria-label="색상 코드" onChange={(e) => setHex(e.target.value)} />}
          <button type="button" className="btn btn-secondary btn-sm" disabled={!adding.trim()} onClick={add}>
            <Icon name="plus" size={14} />
            {t.console.addValue}
          </button>
        </span>
      </div>
    </div>
  );
}
