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
  const padding = viewport.width < 520 ? 16 : 24;
  const availableWidth = Math.max(220, viewport.width - padding * 2);
  const preferredWidth = Math.max(selection.width, viewport.width < 520 ? 320 : 360);
  const width = Math.min(preferredWidth, availableWidth);

  const availableHeight = Math.max(180, viewport.height - padding * 2);
  const preferredHeight = Math.min(Math.max(selection.height, 172), 460);
  const height = Math.min(preferredHeight, availableHeight);

  const left = clamp(selection.x, padding, viewport.width - width - padding);
  const below = selection.y + selection.height + 12;
  const above = selection.y - height - 12;
  const hasRoomBelow = below + height <= viewport.height - padding;
  const y = hasRoomBelow ? below : above;
  const top = clamp(y, padding, viewport.height - height - padding);

  return {
    x: Math.round(left),
    y: Math.round(top),
    width: Math.round(width),
    height: Math.round(height)
  };
}

export function cropCanvasToDataUrl(canvas: HTMLCanvasElement, imageRect: Rect): string {
  const target = document.createElement("canvas");
  target.width = Math.max(1, Math.round(imageRect.width));
  target.height = Math.max(1, Math.round(imageRect.height));

  const context = target.getContext("2d");
  if (!context) {
    throw new Error("Canvas 2D context is unavailable");
  }

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

export function isUsableSelection(rect: Rect, minimumSize = 8): boolean {
  return rect.width >= minimumSize && rect.height >= minimumSize;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
