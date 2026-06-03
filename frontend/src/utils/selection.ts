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

export function isUsableSelection(rect: Rect, minimumSize = 8): boolean {
  return rect.width >= minimumSize && rect.height >= minimumSize;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
