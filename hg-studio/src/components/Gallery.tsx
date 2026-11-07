import { t } from "../i18n/ko";
import { Icon } from "./Icon";

/** Photos and videos in a grid; photos open full size in a new tab. */
export function Gallery({
  media,
  onDelete,
  empty = t.orders.noInspection,
  firstLabel,
}: {
  media: { id: string; url: string; content_type: string }[];
  onDelete?: (id: string) => void;
  empty?: string;
  firstLabel?: string;
}) {
  if (media.length === 0) return <p className="muted small">{empty}</p>;
  return (
    <ul className="gallery">
      {media.map((m, i) => (
        <li key={m.id}>
          {i === 0 && firstLabel && <span className="gallery-tag">{firstLabel}</span>}
          {m.content_type.startsWith("video/") ? (
            <video src={m.url} controls preload="metadata" aria-label={t.orders.video} />
          ) : (
            <a href={m.url} target="_blank" rel="noreferrer">
              <img src={m.url} alt="" />
            </a>
          )}
          {onDelete && (
            <button className="icon-btn gallery-del" aria-label={t.products.deleteMedia} onClick={() => onDelete(m.id)}>
              <Icon name="close" size={16} />
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}
