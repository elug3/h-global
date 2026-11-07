import { useState } from "react";
import { api, call, errorText, type StudioItem } from "../../api/client";
import { t } from "../../i18n/ko";
import { won } from "../../lib/format";
import { stockText, Thumb } from "../../pages/ShowSetup";
import { Icon } from "../Icon";
import { ErrorBanner, NumberBadge } from "../ui";

/** Published items, newest number first, with pin, price and sold-out actions. */
export function PublishedList({ items, onChanged }: { items: StudioItem[]; onChanged: () => void }) {
  const [tab, setTab] = useState<"items" | "chat">("items");
  const [error, setError] = useState<string>();
  const sorted = [...items].sort((a, b) => b.number - a.number);

  async function run(p: Promise<unknown>) {
    setError(undefined);
    try {
      await p;
      onChanged();
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <section className="col-pub">
      <div className="tabs" role="tablist">
        <button role="tab" aria-selected={tab === "items"} className={tab === "items" ? "on" : ""} onClick={() => setTab("items")}>
          {t.console.publishedList} <em>{items.length}</em>
        </button>
        <button role="tab" aria-selected={tab === "chat"} className={tab === "chat" ? "on" : ""} onClick={() => setTab("chat")}>
          {t.console.chat}
        </button>
      </div>
      {tab === "chat" ? (
        <p className="muted empty">{t.console.chatSoon}</p>
      ) : (
        <>
          <ErrorBanner text={error} />
          {sorted.length === 0 && <p className="muted empty">{t.console.noPublished}</p>}
          <ul className="pub-list" data-testid="published">
            {sorted.map((it) => {
              const soldOut = it.product.variants.every((v) => !v.buyable);
              return (
                <li key={it.id} className={"pub-row" + (soldOut ? " sold" : "")}>
                  <Thumb product={it.product} size={40} />
                  <div className="pub-main">
                    <NumberBadge n={it.number} size="xs" />
                    <span className="pub-title">{it.product.title}</span>
                    <b>{won(it.product.price_won)}</b>
                    <span className="muted small">{soldOut ? t.console.soldOutLabel : stockText(it.product)}</span>
                  </div>
                  <div className="pub-actions">
                    <button
                      className={"icon-btn pin" + (it.pinned ? " on" : "")}
                      aria-pressed={it.pinned}
                      title={it.pinned ? t.console.unpin : t.console.pin}
                      aria-label={it.pinned ? t.console.unpin : t.console.pin}
                      onClick={() =>
                        void run(
                          call(
                            it.pinned
                              ? api.POST("/studio-api/items/{id}/unpin", { params: { path: { id: it.id } } })
                              : api.POST("/studio-api/items/{id}/pin", { params: { path: { id: it.id } } }),
                          ),
                        )
                      }
                    >
                      <Icon name="star" size={18} filled={it.pinned} />
                    </button>
                    <button
                      className="btn btn-secondary btn-xs"
                      onClick={() => {
                        const v = prompt(t.console.newPrice, String(it.product.price_won));
                        if (v === null) return;
                        const n = Number(v.replace(/\D/g, ""));
                        void run(
                          call(api.PATCH("/studio-api/products/{id}", { params: { path: { id: it.product_id } }, body: { price_won: n } })),
                        );
                      }}
                    >
                      {t.console.editPrice}
                    </button>
                    {!soldOut && (
                      <button
                        className="btn btn-secondary btn-xs"
                        onClick={() => {
                          if (confirm(t.console.soldOutConfirm)) {
                            void run(call(api.POST("/studio-api/items/{id}/sold-out", { params: { path: { id: it.id } } })));
                          }
                        }}
                      >
                        {t.console.soldOut}
                      </button>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        </>
      )}
    </section>
  );
}
