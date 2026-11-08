import { useRef, useState, type PointerEvent } from "react";

export type Rect = { x: number; y: number; w: number; h: number };

/**
 * Shows an image and lets the user drag a crop rectangle. The rectangle is
 * reported in the image's own pixels (what hg-server crops with).
 */
export function CropImage({
  src,
  naturalWidth,
  naturalHeight,
  crop,
  onCrop,
}: {
  src: string;
  naturalWidth: number;
  naturalHeight: number;
  crop?: Rect;
  onCrop: (r?: Rect) => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [drag, setDrag] = useState<{ x0: number; y0: number; x1: number; y1: number }>();

  function point(e: PointerEvent) {
    const r = box.current!.getBoundingClientRect();
    return {
      x: Math.min(Math.max(e.clientX - r.left, 0), r.width),
      y: Math.min(Math.max(e.clientY - r.top, 0), r.height),
      scale: naturalWidth / r.width,
    };
  }

  function finish(e: PointerEvent) {
    if (!drag) return;
    const { scale } = point(e);
    const x = Math.round(Math.min(drag.x0, drag.x1) * scale);
    const y = Math.round(Math.min(drag.y0, drag.y1) * scale);
    const w = Math.round(Math.abs(drag.x1 - drag.x0) * scale);
    const h = Math.round(Math.abs(drag.y1 - drag.y0) * scale);
    setDrag(undefined);
    if (w >= 16 && h >= 16) {
      onCrop({ x, y, w: Math.min(w, naturalWidth - x), h: Math.min(h, naturalHeight - y) });
    }
  }

  // Shown rectangle as fractions of the box.
  let shown: { l: number; t: number; w: number; h: number } | undefined;
  if (drag && box.current) {
    const r = box.current.getBoundingClientRect();
    shown = {
      l: Math.min(drag.x0, drag.x1) / r.width,
      t: Math.min(drag.y0, drag.y1) / r.height,
      w: Math.abs(drag.x1 - drag.x0) / r.width,
      h: Math.abs(drag.y1 - drag.y0) / r.height,
    };
  } else if (crop) {
    shown = { l: crop.x / naturalWidth, t: crop.y / naturalHeight, w: crop.w / naturalWidth, h: crop.h / naturalHeight };
  }

  return (
    <div
      ref={box}
      className="crop"
      style={{ aspectRatio: `${naturalWidth} / ${naturalHeight}` }}
      onPointerDown={(e) => {
        e.currentTarget.setPointerCapture(e.pointerId);
        const p = point(e);
        setDrag({ x0: p.x, y0: p.y, x1: p.x, y1: p.y });
      }}
      onPointerMove={(e) => {
        if (!drag) return;
        const p = point(e);
        setDrag({ ...drag, x1: p.x, y1: p.y });
      }}
      onPointerUp={finish}
      data-testid="crop"
    >
      <img src={src} alt="" draggable={false} />
      {shown && (
        <div
          className="crop-rect"
          style={{
            left: `${shown.l * 100}%`,
            top: `${shown.t * 100}%`,
            width: `${shown.w * 100}%`,
            height: `${shown.h * 100}%`,
          }}
        />
      )}
    </div>
  );
}
