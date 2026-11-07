import type { Product } from "../api/client";
import { t } from "../i18n/ko";

/** The product's first photo, or an empty square. */
export function Thumb({ product, size = 44 }: { product: Pick<Product, "media">; size?: number }) {
  const photo = product.media.find((m) => m.kind === "photo");
  return photo ? (
    <img className="thumb" src={photo.url} width={size} height={size} alt="" />
  ) : (
    <span className="thumb thumb-empty" style={{ width: size, height: size }} />
  );
}

/** "판매 / 재고 2 / 5" over all variants; until-sold-out stock is unlimited. */
export function stockText(p: Product): string {
  let unlimited = false;
  let stock = 0;
  let sold = 0;
  for (const v of p.variants) {
    sold += v.sold_qty;
    if (v.stock_mode === "until_sold_out") unlimited = true;
    else stock += v.stock_qty;
  }
  return `${t.console.soldStock} ${sold} / ${unlimited ? t.console.unlimited : stock}`;
}

/** Same numbers without the label: "2 / 5". */
export function soldOfStock(p: Product): string {
  return stockText(p).replace(`${t.console.soldStock} `, "");
}

export function isSoldOut(p: Product): boolean {
  return p.variants.every((v) => !v.buyable);
}

/** Product status: 작성 중, 판매 중, 숨김, 보관. */
export function ProductStatusTag({ status }: { status: string }) {
  return <span className={`tag tag-p-${status}`}>{t.productStatus[status] ?? status}</span>;
}

/** What is still missing before details_complete (hg-server's rule). */
export function missingDetails(p: Pick<Product, "title" | "description">): string[] {
  const out: string[] = [];
  if (!p.title || p.title.startsWith("Live #")) out.push(t.products.missingTitle);
  if (!p.description) out.push(t.products.missingDescription);
  return out;
}
