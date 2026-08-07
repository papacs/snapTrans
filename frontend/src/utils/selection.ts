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

export interface DisplayInfo {
  x: number;
  y: number;
  width: number;
  height: number;
  scale: number;
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

export function clampPointToBounds(point: Point, bounds: Size): Point {
  return {
    x: clamp(point.x, 0, Math.max(0, bounds.width)),
    y: clamp(point.y, 0, Math.max(0, bounds.height))
  };
}

export function selectionBadgePosition(
  selection: Rect,
  viewport: Size,
  badge: Size = { width: 88, height: 24 }
): Point {
  const padding = 8;
  const gap = 8;
  const x = clamp(
    selection.x,
    padding,
    Math.max(padding, viewport.width - badge.width - padding)
  );
  const above = selection.y - badge.height - gap;
  const below = selection.y + selection.height + gap;
  const y = above >= padding
    ? above
    : clamp(below, padding, Math.max(padding, viewport.height - badge.height - padding));

  return { x: Math.round(x), y: Math.round(y) };
}

export function mapCssRectToImageRect(
  rect: Rect,
  cssSize: Size,
  imageSize: Size,
  displays: DisplayInfo[] = []
): Rect {
  if (cssSize.width <= 0 || cssSize.height <= 0) {
    return { x: 0, y: 0, width: 0, height: 0 };
  }

  if (!displays || displays.length === 0) {
    return ratioMapCssRectToImageRect(rect, cssSize, imageSize);
  }

  const unionLogical = unionLogicalSize(displays);
  const scaleX = unionLogical.width / cssSize.width;
  const scaleY = unionLogical.height / cssSize.height;

  const corners: Point[] = [
    { x: rect.x, y: rect.y },
    { x: rect.x + rect.width, y: rect.y },
    { x: rect.x, y: rect.y + rect.height },
    { x: rect.x + rect.width, y: rect.y + rect.height }
  ];

  const mapped = corners.map((corner) => {
    const logical = {
      x: corner.x * scaleX,
      y: corner.y * scaleY
    };
    const display = displayForLogicalPoint(logical, displays) ?? nearestDisplay(logical, displays)!;
    return {
      x: logical.x * display.scale,
      y: logical.y * display.scale
    };
  });

  const minX = Math.min(...mapped.map((point) => point.x));
  const minY = Math.min(...mapped.map((point) => point.y));
  const maxX = Math.max(...mapped.map((point) => point.x));
  const maxY = Math.max(...mapped.map((point) => point.y));

  return {
    x: clamp(Math.round(minX), 0, imageSize.width),
    y: clamp(Math.round(minY), 0, imageSize.height),
    width: clamp(Math.round(maxX - minX), 0, imageSize.width),
    height: clamp(Math.round(maxY - minY), 0, imageSize.height)
  };
}

function ratioMapCssRectToImageRect(rect: Rect, cssSize: Size, imageSize: Size): Rect {
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

function unionLogicalSize(displays: DisplayInfo[]): Size {
  let maxRight = 0;
  let maxBottom = 0;
  for (const display of displays) {
    maxRight = Math.max(maxRight, display.x + display.width);
    maxBottom = Math.max(maxBottom, display.y + display.height);
  }
  return { width: Math.max(1, maxRight), height: Math.max(1, maxBottom) };
}

function displayForLogicalPoint(point: Point, displays: DisplayInfo[]): DisplayInfo | null {
  return displays.find(
    (display) =>
      point.x >= display.x &&
      point.x <= display.x + display.width &&
      point.y >= display.y &&
      point.y <= display.y + display.height
  ) ?? null;
}

function nearestDisplay(point: Point, displays: DisplayInfo[]): DisplayInfo | null {
  if (displays.length === 0) {
    return null;
  }
  const centerX = point.x;
  const centerY = point.y;
  return displays.reduce((best, display) => {
    const bestCenter = {
      x: best.x + best.width / 2,
      y: best.y + best.height / 2
    };
    const candidateCenter = {
      x: display.x + display.width / 2,
      y: display.y + display.height / 2
    };
    const bestDistance = (bestCenter.x - centerX) ** 2 + (bestCenter.y - centerY) ** 2;
    const candidateDistance = (candidateCenter.x - centerX) ** 2 + (candidateCenter.y - centerY) ** 2;
    return candidateDistance < bestDistance ? display : best;
  });
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

export function shouldUseFlowTranslationLayout(blocks: OCRBlock[]): boolean {
  if (blocks.length === 0) {
    return false;
  }

  const textBlocks = blocks.filter((block) => hasNaturalLanguage(block.text));
  if (textBlocks.length === 0) {
    return false;
  }

  const longBlocks = textBlocks.filter((block) => normalizedTextLength(block.text) >= 32);
  const wideTextBlocks = textBlocks.filter(
    (block) => block.width >= 0.42 && normalizedTextLength(block.text) >= 18
  );

  return wideTextBlocks.length >= 1 || longBlocks.length >= 2 || (textBlocks.length >= 6 && longBlocks.length >= 1);
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

export function wrapTranslationText(text: string, fontSize: number, width: number): string[] {
  const unitsPerLine = Math.max(1, width / Math.max(1, fontSize * 0.62));
  const lines: string[] = [];
  let current = "";
  let currentUnits = 0;

  for (const char of Array.from(text.trim())) {
    const unit = characterUnit(char);
    if (current && currentUnits + unit > unitsPerLine) {
      lines.push(current);
      current = char.trimStart();
      currentUnits = characterUnit(current);
      continue;
    }

    current += char;
    currentUnits += unit;
  }

  if (current) {
    lines.push(current);
  }

  return lines.length > 0 ? lines : [text];
}

export function sampleCanvasLuminance(canvas: HTMLCanvasElement | null, cssRect: Rect): number | null {
  return sampleCanvasColor(canvas, cssRect)?.luminance ?? null;
}

export function sampleCanvasColor(canvas: HTMLCanvasElement | null, cssRect: Rect): SampledColor | null {
  const sample = sampleCanvasImageData(canvas, cssRect);
  return sample ? colorFromImageData(sample.data) : null;
}

export function sampleCanvasForegroundColor(canvas: HTMLCanvasElement | null, cssRect: Rect): SampledColor | null {
  const sample = sampleCanvasImageData(canvas, cssRect);
  if (!sample) {
    return null;
  }

  const background = colorFromImageData(sample.data);
  if (!background) {
    return null;
  }

  let best: SampledColor | null = null;
  let bestDistance = 0;
  for (let index = 0; index < sample.data.length; index += 4) {
    const alpha = sample.data[index + 3] / 255;
    if (alpha <= 0.05) {
      continue;
    }

    const red = sample.data[index];
    const green = sample.data[index + 1];
    const blue = sample.data[index + 2];
    const luminance = luminanceForColor(red, green, blue);
    const distance = colorDistance({ red, green, blue }, background);
    if (distance < 55 && Math.abs(luminance - background.luminance) < 0.18) {
      continue;
    }

    if (distance > bestDistance) {
      bestDistance = distance;
      best = { red, green, blue, luminance };
    }
  }

  return best;
}

function sampleCanvasImageData(canvas: HTMLCanvasElement | null, cssRect: Rect): ImageData | null {
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
    return context.getImageData(
      imageRect.x,
      imageRect.y,
      Math.max(1, imageRect.width),
      Math.max(1, imageRect.height)
    );
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
  const reds: number[] = [];
  const greens: number[] = [];
  const blues: number[] = [];
  const luminances: number[] = [];
  for (let index = 0; index < data.length; index += 4) {
    const alpha = data[index + 3] / 255;
    if (alpha <= 0.05) {
      continue;
    }

    const red = data[index] / 255;
    const green = data[index + 1] / 255;
    const blue = data[index + 2] / 255;
    reds.push(data[index]);
    greens.push(data[index + 1]);
    blues.push(data[index + 2]);
    luminances.push(0.2126 * red + 0.7152 * green + 0.0722 * blue);
  }

  if (reds.length === 0) {
    return null;
  }

  return {
    red: median(reds),
    green: median(greens),
    blue: median(blues),
    luminance: median(luminances)
  };
}

function luminanceForColor(red: number, green: number, blue: number): number {
  return 0.2126 * (red / 255) + 0.7152 * (green / 255) + 0.0722 * (blue / 255);
}

function colorDistance(
  left: Pick<SampledColor, "red" | "green" | "blue">,
  right: Pick<SampledColor, "red" | "green" | "blue">
): number {
  const red = left.red - right.red;
  const green = left.green - right.green;
  const blue = left.blue - right.blue;
  return Math.sqrt(red * red + green * green + blue * blue);
}

function median(values: number[]): number {
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  if (sorted.length % 2 === 1) {
    return sorted[middle];
  }
  return (sorted[middle - 1] + sorted[middle]) / 2;
}

function characterUnit(char: string): number {
  if (/\s/.test(char)) {
    return 0.35;
  }
  if (/[\p{Script=Han}]/u.test(char)) {
    return 1;
  }
  if (/[A-Za-z0-9]/.test(char)) {
    return 0.62;
  }
  return 0.5;
}

function normalizedTextLength(text: string): number {
  return Array.from(text.trim().replace(/\s+/g, " ")).length;
}

function hasNaturalLanguage(text: string): boolean {
  return /[\p{Script=Han}A-Za-z]/u.test(text);
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
