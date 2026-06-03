export interface Point {
  x: number;
  y: number;
}

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface OCRBlock {
  text: string;
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface TranslationPalette {
  backgroundColor: string;
  boxShadow: string;
  color: string;
  textShadow: string;
}

export interface SampledColor {
  red: number;
  green: number;
  blue: number;
  luminance: number;
}

export interface Size {
  width: number;
  height: number;
}

export function normalizeRect(start: Point, end: Point): Rect {
  const x = Math.min(start.x, end.x);
  const y = Math.min(start.y, end.y);
  return {
    x,
    y,
    width: Math.abs(end.x - start.x),
    height: Math.abs(end.y - start.y)
  };
}

export function mapCssRectToImageRect(rect: Rect, cssSize: Size, imageSize: Size): Rect {
  if (cssSize.width <= 0 || cssSize.height <= 0) {
    return { x: 0, y: 0, width: 0, height: 0 };
  }

  const scaleX = imageSize.width / cssSize.width;
  const scaleY = imageSize.height / cssSize.height;

  const x = clamp(Math.round(rect.x * scaleX), 0, imageSize.width);
  const y = clamp(Math.round(rect.y * scaleY), 0, imageSize.height);
  const right = clamp(Math.round((rect.x + rect.width) * scaleX), 0, imageSize.width);
  const bottom = clamp(Math.round((rect.y + rect.height) * scaleY), 0, imageSize.height);

  return {
    x,
    y,
    width: Math.max(0, right - x),
    height: Math.max(0, bottom - y)
  };
}

export function normalizeResultBox(selection: Rect, viewport: Size): Rect {
  const padding = 8;
  const availableWidth = Math.max(48, viewport.width - padding * 2);
  const availableHeight = Math.max(32, viewport.height - padding * 2);
  const width = Math.min(Math.max(selection.width, 48), availableWidth);
  const height = Math.min(Math.max(selection.height, 32), availableHeight);
  const left = clamp(selection.x, padding, viewport.width - width - padding);
  const top = clamp(selection.y, padding, viewport.height - height - padding);

  return {
    x: Math.round(left),
    y: Math.round(top),
    width: Math.round(width),
    height: Math.round(height)
  };
}

export function cropCanvasToDataUrl(canvas: HTMLCanvasElement, imageRect: Rect): string {
  const scale = ocrScaleForRect(imageRect);
  const target = document.createElement("canvas");
  target.width = Math.max(1, Math.round(imageRect.width * scale));
  target.height = Math.max(1, Math.round(imageRect.height * scale));

  const context = target.getContext("2d");
  if (!context) {
    throw new Error("Canvas 2D context is unavailable");
  }
  context.imageSmoothingEnabled = true;
  context.imageSmoothingQuality = "high";

  context.drawImage(
    canvas,
    imageRect.x,
    imageRect.y,
    imageRect.width,
    imageRect.height,
    0,
    0,
    target.width,
    target.height
  );

  return target.toDataURL("image/png");
}

export function ocrScaleForRect(imageRect: Rect): number {
  const shortSide = Math.min(imageRect.width, imageRect.height);
  if (shortSide <= 0) {
    return 1;
  }

  const minimumShortSide = 96;
  const maximumScale = 5;
  return Math.max(1, Math.min(maximumScale, Math.ceil(minimumShortSide / shortSide)));
}

export function mapOCRBlockToSelection(block: OCRBlock, selection: Rect): Rect {
  return {
    x: Math.round(selection.x + block.x * selection.width),
    y: Math.round(selection.y + block.y * selection.height),
    width: Math.max(1, Math.round(block.width * selection.width)),
    height: Math.max(1, Math.round(block.height * selection.height))
  };
}

export function fontSizeForOCRBlock(block: OCRBlock, selectionHeight: number): number {
  const blockHeight = Math.max(1, block.height * selectionHeight);
  return Math.max(11, Math.min(22, Math.round(blockHeight * 0.95)));
}

export function fontSizeForTranslationBlock(text: string, rect: Rect): number {
  const trimmed = text.trim();
  const charCount = Math.max(1, Array.from(trimmed).length);
  const lengthDamping = charCount > 6 ? 0.82 : 1;
  const heightLimit = rect.height * 0.84 * lengthDamping;
  const widthLimit = rect.width / Math.max(2, charCount * 1.15);
  const size = Math.min(heightLimit, widthLimit, 20);

  return Math.max(10, Math.round(size));
}

export function sampleCanvasLuminance(canvas: HTMLCanvasElement | null, cssRect: Rect): number | null {
  return sampleCanvasColor(canvas, cssRect)?.luminance ?? null;
}

export function sampleCanvasColor(canvas: HTMLCanvasElement | null, cssRect: Rect): SampledColor | null {
  if (!canvas || cssRect.width <= 0 || cssRect.height <= 0) {
    return null;
  }

  const bounds = canvas.getBoundingClientRect();
  const imageRect = mapCssRectToImageRect(
    {
      x: cssRect.x - bounds.left,
      y: cssRect.y - bounds.top,
      width: cssRect.width,
      height: cssRect.height
    },
    { width: bounds.width, height: bounds.height },
    { width: canvas.width, height: canvas.height }
  );
  if (imageRect.width <= 0 || imageRect.height <= 0) {
    return null;
  }

  const context = canvas.getContext("2d");
  if (!context) {
    return null;
  }

  try {
    const sample = context.getImageData(
      imageRect.x,
      imageRect.y,
      Math.max(1, imageRect.width),
      Math.max(1, imageRect.height)
    );
    return colorFromImageData(sample.data);
  } catch {
    return null;
  }
}

export function translationPaletteForLuminance(luminance: number | null): TranslationPalette {
  const safeLuminance = typeof luminance === "number" && Number.isFinite(luminance) ? luminance : 0.35;
  const value = Math.round(safeLuminance * 255);
  return translationPaletteForColor({
    red: value,
    green: value,
    blue: value,
    luminance: safeLuminance
  });
}

export function translationPaletteForColor(color: SampledColor | null): TranslationPalette {
  const sampled = color ?? { red: 37, green: 42, blue: 55, luminance: 0.22 };
  const red = clamp(Math.round(sampled.red), 0, 255);
  const green = clamp(Math.round(sampled.green), 0, 255);
  const blue = clamp(Math.round(sampled.blue), 0, 255);
  const safeLuminance =
    typeof sampled.luminance === "number" && Number.isFinite(sampled.luminance) ? sampled.luminance : 0.35;

  if (safeLuminance < 0.48) {
    return {
      backgroundColor: `rgba(${red}, ${green}, ${blue}, 0.98)`,
      boxShadow: "none",
      color: "#f8fafc",
      textShadow: "0 1px 1px rgba(0, 0, 0, 0.45)"
    };
  }

  return {
    backgroundColor: `rgba(${red}, ${green}, ${blue}, 0.98)`,
    boxShadow: "none",
    color: "#0f172a",
    textShadow: "none"
  };
}

export function isUsableSelection(rect: Rect, minimumSize = 8): boolean {
  return rect.width >= minimumSize && rect.height >= minimumSize;
}

function colorFromImageData(data: Uint8ClampedArray): SampledColor | null {
  let redTotal = 0;
  let greenTotal = 0;
  let blueTotal = 0;
  let total = 0;
  let count = 0;
  for (let index = 0; index < data.length; index += 4) {
    const alpha = data[index + 3] / 255;
    if (alpha <= 0.05) {
      continue;
    }

    const red = data[index] / 255;
    const green = data[index + 1] / 255;
    const blue = data[index + 2] / 255;
    redTotal += data[index] * alpha;
    greenTotal += data[index + 1] * alpha;
    blueTotal += data[index + 2] * alpha;
    total += (0.2126 * red + 0.7152 * green + 0.0722 * blue) * alpha;
    count += alpha;
  }

  if (count <= 0) {
    return null;
  }

  return {
    red: redTotal / count,
    green: greenTotal / count,
    blue: blueTotal / count,
    luminance: total / count
  };
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
