import { Link } from "react-router";
import type { ItemView, Product } from "../api/client";
import { t } from "../i18n/ko";
import { won } from "../lib/format";
import { NumberBadge, Photo, Swatch } from "./ui";

/** Color swatches of a product's first option that has colors. */
export function ProductSwatches({ product, size = 12 }: { product: Product; size?: number }) {
  const opt = product.options.find((o) => o.values.some((v) => v.hex1));
  if (!opt) return null;
  return (
    <span className="sws">
      {opt.values.slice(0, 4).map((v) => (
        <Swatch key={v.id} hex1={v.hex1} hex2={v.hex2} size={size} />
      ))}
    </span>
  );
}

/** A numbered item card on the live page (board 2). */
export function ItemCard({ item, onOpen, tall = false }: { item: ItemView; onOpen: (item: ItemView) => void; tall?: boolean }) {
  const p = item.product;
  return (
    <button type="button" className={"card" + (p.sold_out ? " sold" : "")} onClick={() => onOpen(item)} data-testid={`item-${item.number}`}>
      <span className={"card-img" + (tall ? " tall" : "")}>
        <Photo src={p.photos[0]} size="100%" radius={12} alt={p.title} />
        <span className="pos">
          <NumberBadge n={item.number} />
        </span>
        {item.is_new && !p.sold_out && <span className="newb pos-r">{t.live.newBadge}</span>}
        {p.sold_out && (
          <span className="soldov">
            <span>{t.live.soldOut}</span>
          </span>
        )}
      </span>
      <span className="cname">{p.title}</span>
      <span className="crow">
        <span className="price">{won(p.price_won)}</span>
        <ProductSwatches product={p} />
      </span>
    </button>
  );
}

/** A product card for lists outside a stream (home). */
export function ProductCard({ product }: { product: Product }) {
  return (
    <Link className={"card" + (product.sold_out ? " sold" : "")} to={`/products/${product.id}`}>
      <span className="card-img">
        <Photo src={product.photos[0]} size="100%" radius={12} alt={product.title} />
        {product.sold_out && (
          <span className="soldov">
            <span>{t.live.soldOut}</span>
          </span>
        )}
      </span>
      <span className="cname">{product.title}</span>
      <span className="crow">
        <span className="price">{won(product.price_won)}</span>
        <ProductSwatches product={product} />
      </span>
    </Link>
  );
}
