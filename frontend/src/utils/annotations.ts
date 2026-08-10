import type { Point, Rect, Size } from "./selection";

export type AnnotationTool = "rectangle" | "ellipse" | "arrow" | "pen" | "mosaic" | "text" | "sticker";

export type Annotation =
  | {
      tool: "rectangle" | "ellipse" | "arrow";
      start: Point;
      end: Point;
      color: string;
      width: number;
    }
  | {
      tool: "pen" | "mosaic";
      points: Point[];
      color: string;
      width: number;
    }
  | {
      tool: "text" | "sticker";
      point: Point;
      text: string;
      color: string;
      fontSize: number;
    };

export function toolbarPosition(
  selection: Rect,
  viewport: Size,
  toolbar: Size,
  gap = 12
): Point {
  const padding = 8;
  const maxX = Math.max(padding, viewport.width - toolbar.width - padding);
  const x = clamp(selection.x + selection.width - toolbar.width, padding, maxX);
  const below = selection.y + selection.height + gap;
  const above = selection.y - toolbar.height - gap;
  const maxY = Math.max(padding, viewport.height - toolbar.height - padding);
  const y = below + toolbar.height <= viewport.height - padding
    ? below
    : above >= padding
      ? above
      : clamp(below, padding, maxY);

  return { x: Math.round(x), y: Math.round(y) };
}

export function renderAnnotations(
  context: CanvasRenderingContext2D,
  annotations: Annotation[],
  pixelatedSource?: CanvasImageSource
): void {
  for (const annotation of annotations) {
    renderAnnotation(context, annotation, pixelatedSource);
  }
}

export function renderAnnotation(
  context: CanvasRenderingContext2D,
  annotation: Annotation,
  pixelatedSource?: CanvasImageSource
): void {
  context.save();
  context.lineCap = "round";
  context.lineJoin = "round";

  switch (annotation.tool) {
    case "rectangle":
      applyStroke(context, annotation.color, annotation.width);
      context.strokeRect(
        annotation.start.x,
        annotation.start.y,
        annotation.end.x - annotation.start.x,
        annotation.end.y - annotation.start.y
      );
      break;
    case "ellipse": {
      applyStroke(context, annotation.color, annotation.width);
      const centerX = (annotation.start.x + annotation.end.x) / 2;
      const centerY = (annotation.start.y + annotation.end.y) / 2;
      const radiusX = Math.abs(annotation.end.x - annotation.start.x) / 2;
      const radiusY = Math.abs(annotation.end.y - annotation.start.y) / 2;
      context.beginPath();
      context.ellipse(centerX, centerY, radiusX, radiusY, 0, 0, Math.PI * 2);
      context.stroke();
      break;
    }
    case "arrow":
      drawArrow(context, annotation.start, annotation.end, annotation.color, annotation.width);
      break;
    case "pen":
      applyStroke(context, annotation.color, annotation.width);
      drawPolyline(context, annotation.points);
      break;
    case "mosaic":
      if (pixelatedSource) {
        drawMosaic(context, annotation.points, annotation.width, pixelatedSource);
      }
      break;
    case "text":
    case "sticker":
      context.fillStyle = annotation.color;
      context.font = `${annotation.fontSize}px "Segoe UI", "Microsoft YaHei", sans-serif`;
      context.textBaseline = "top";
      context.fillText(annotation.text, annotation.point.x, annotation.point.y);
      break;
  }

  context.restore();
}

export function createPixelatedCanvas(source: HTMLCanvasElement, blockSize: number): HTMLCanvasElement {
  const pixelated = document.createElement("canvas");
  pixelated.width = source.width;
  pixelated.height = source.height;
  const context = pixelated.getContext("2d");
  if (!context) {
    return pixelated;
  }

  const small = document.createElement("canvas");
  small.width = Math.max(1, Math.ceil(source.width / blockSize));
  small.height = Math.max(1, Math.ceil(source.height / blockSize));
  const smallContext = small.getContext("2d");
  if (!smallContext) {
    return pixelated;
  }

  smallContext.imageSmoothingEnabled = true;
  smallContext.drawImage(source, 0, 0, small.width, small.height);
  context.imageSmoothingEnabled = false;
  context.drawImage(small, 0, 0, small.width, small.height, 0, 0, source.width, source.height);
  return pixelated;
}

function applyStroke(context: CanvasRenderingContext2D, color: string, width: number): void {
  context.strokeStyle = color;
  context.lineWidth = width;
}

function drawPolyline(context: CanvasRenderingContext2D, points: Point[]): void {
  if (points.length === 0) {
    return;
  }
  context.beginPath();
  context.moveTo(points[0].x, points[0].y);
  for (const point of points.slice(1)) {
    context.lineTo(point.x, point.y);
  }
  if (points.length === 1) {
    context.lineTo(points[0].x + 0.01, points[0].y + 0.01);
  }
  context.stroke();
}

function drawArrow(
  context: CanvasRenderingContext2D,
  start: Point,
  end: Point,
  color: string,
  width: number
): void {
  const deltaX = end.x - start.x;
  const deltaY = end.y - start.y;
  const length = Math.hypot(deltaX, deltaY);
  if (length < 0.01) {
    return;
  }

  const directionX = deltaX / length;
  const directionY = deltaY / length;
  const normalX = -directionY;
  const normalY = directionX;
  const headLength = Math.min(
    Math.max(width * 8, 24),
    Math.max(width * 2.5, length * 0.45)
  );
  const headHalfWidth = Math.min(
    headLength * 0.62,
    Math.max(width * 2.8, headLength * 0.48)
  );
  const baseCenter = {
    x: end.x - directionX * headLength,
    y: end.y - directionY * headLength
  };

  applyStroke(context, color, width * 1.35);
  context.beginPath();
  context.moveTo(start.x, start.y);
  context.lineTo(
    baseCenter.x + directionX * width * 1.5,
    baseCenter.y + directionY * width * 1.5
  );
  context.stroke();

  context.fillStyle = color;
  context.beginPath();
  context.moveTo(end.x, end.y);
  context.lineTo(
    baseCenter.x + normalX * headHalfWidth,
    baseCenter.y + normalY * headHalfWidth
  );
  context.lineTo(
    baseCenter.x - normalX * headHalfWidth,
    baseCenter.y - normalY * headHalfWidth
  );
  context.closePath();
  context.fill();
}

function drawMosaic(
  context: CanvasRenderingContext2D,
  points: Point[],
  width: number,
  pixelatedSource: CanvasImageSource
): void {
  if (points.length === 0) {
    return;
  }
  const pattern = context.createPattern(pixelatedSource, "no-repeat");
  if (!pattern) {
    return;
  }
  context.beginPath();
  context.lineCap = "round";
  context.lineJoin = "round";
  context.lineWidth = width;
  context.moveTo(points[0].x, points[0].y);
  for (const point of points.slice(1)) {
    context.lineTo(point.x, point.y);
  }
  if (points.length === 1) {
    context.lineTo(points[0].x + 0.01, points[0].y + 0.01);
  }
  context.strokeStyle = pattern;
  context.stroke();
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
