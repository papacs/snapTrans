import type { Rect, TranslationPalette } from "./selection";
import {
  OVERLAY_FONT_FAMILY,
  type OverlayBlock,
  type MeasureText,
} from "./overlay-layout";

export interface StyledOverlayBlock extends OverlayBlock {
  palette: TranslationPalette;
  foreground: string;
}

export function canvasTextMeasurer(
  context: CanvasRenderingContext2D,
): MeasureText {
  const widths = new Map<string, number>();
  return (text, size) => {
    const key = size + ":" + text;
    const cached = widths.get(key);
    if (cached !== undefined) return cached;
    context.font = "400 " + size + "px " + OVERLAY_FONT_FAMILY;
    const width = context.measureText(text).width;
    if (widths.size > 4096) widths.clear();
    widths.set(key, width);
    return width;
  };
}
export function paintTranslationOverlay(
  context: CanvasRenderingContext2D,
  blocks: StyledOverlayBlock[],
  scaleX: number,
  scaleY: number,
): void {
  context.save();
  context.scale(scaleX, scaleY);
  for (const block of blocks) {
    const rect = block.bounds;
    context.save();
    context.beginPath();
    context.rect(rect.x, rect.y, rect.width, block.height);
    context.clip();
    context.fillStyle = block.palette.backgroundColor;
    context.fillRect(rect.x, rect.y, rect.width, block.height);
    context.font = "400 " + block.fontSize + "px " + OVERLAY_FONT_FAMILY;
    context.fillStyle = block.foreground;
    context.textAlign = block.kind === "label" ? "center" : "left";
    context.textBaseline = "middle";
    const top =
      rect.y +
      (block.kind === "label"
        ? Math.max(
            0,
            (block.height - block.lines.length * block.lineHeight) / 2,
          )
        : 0);
    block.lines.forEach((line, index) => {
      context.fillText(
        line,
        rect.x +
          (block.kind === "label"
            ? rect.width / 2
            : 1 + (index > 0 ? block.indent : 0)),
        top + block.lineHeight * (index + 0.5),
      );
    });
    context.restore();
  }
  context.restore();
}

// Some OCR boxes include a filled emoji/check icon without recognizing it as
// text. Preserve a compact, saturated leading marker instead of painting it out.
export function leadingMarkerInset(
  canvas: HTMLCanvasElement | null,
  rect: Rect,
): number {
  if (!canvas || rect.width < rect.height * 2) return 0;
  const bounds = canvas.getBoundingClientRect();
  if (!bounds.width || !bounds.height) return 0;
  const sx = canvas.width / bounds.width,
    sy = canvas.height / bounds.height;
  const x = Math.max(0, Math.floor((rect.x - bounds.left) * sx)),
    y = Math.max(0, Math.floor((rect.y - bounds.top) * sy));
  const width = Math.min(
    canvas.width - x,
    Math.ceil(Math.min(rect.width, rect.height * 1.4) * sx),
  );
  const height = Math.min(canvas.height - y, Math.ceil(rect.height * sy));
  if (width < 1 || height < 1) return 0;
  const context = canvas.getContext("2d");
  if (!context) return 0;
  try {
    const data = context.getImageData(x, y, width, height);
    return markerInsetFromPixels(data) / sx;
  } catch {
    return 0;
  }
}
export function markerInsetFromPixels(
  image: Pick<ImageData, "data" | "width" | "height">,
): number {
  const { data, width, height } = image;
  if (
    !Number.isFinite(width) ||
    !Number.isFinite(height) ||
    data.length < width * height * 4
  )
    return 0;
  let left = width,
    right = -1,
    top = height,
    bottom = -1,
    count = 0;
  for (let y = 0; y < height; y++)
    for (let x = 0; x < width; x++) {
      const i = (y * width + x) * 4;
      const r = data[i]!,
        g = data[i + 1]!,
        b = data[i + 2]!;
      if (data[i + 3]! < 200 || Math.max(r, g, b) - Math.min(r, g, b) < 55)
        continue;
      left = Math.min(left, x);
      right = Math.max(right, x);
      top = Math.min(top, y);
      bottom = Math.max(bottom, y);
      count++;
    }
  const w = right - left + 1,
    h = bottom - top + 1;
  if (
    w < height * 0.45 ||
    h < height * 0.5 ||
    w / h < 0.65 ||
    w / h > 1.35 ||
    count / (w * h) < 0.55 ||
    left > height * 0.35
  )
    return 0;
  return Math.min(width, right + 4);
}
