import type { HistoryEntry, OCRBlockPayload } from "../services/backend";
import type { Rect } from "./selection";
import { readableTranslation } from "./history";

// Keep paragraph boundaries and avoid introducing spaces between CJK glyphs.
export function mergeTextLines(text: string): string {
  return text
    .replace(/\r\n?/g, "\n")
    .split(/\n\s*\n/)
    .map((paragraph) =>
      paragraph
        .split("\n")
        .map((line) => line.trim())
        .reduce((joined, line) => {
          if (!joined) return line;
          if (!line) return joined;
          const separator =
            /[\u3400-\u9fff]$/.test(joined) && /^[\u3400-\u9fff]/.test(line)
              ? ""
              : " ";
          return joined + separator + line;
        }, ""),
    )
    .join("\n\n");
}
export interface RedactionCandidate {
  index: number;
  text: string;
  kind: string;
  rect: Rect;
}
export function detectSensitiveBlocks(
  blocks: OCRBlockPayload[],
  width: number,
  height: number,
): RedactionCandidate[] {
  if (width <= 0 || height <= 0) return [];
  return blocks.flatMap((block, index) => {
    const email = /[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/i.test(block.text);
    const phone =
      /(?:^|[^\d])(?:\+?86[-\s]?)?1[3-9](?:[-\s]?\d){9}(?!\d)|(?:^|[^\d])(?:\+\d{1,3}[-\s]?)?\(?\d{3}\)?[-\s]\d{3}[-\s]\d{4}(?!\d)/.test(
        block.text,
      );
    if (
      (!email && !phone) ||
      ![block.x, block.y, block.width, block.height].every(Number.isFinite) ||
      block.width <= 0 ||
      block.height <= 0
    )
      return [];
    const x = Math.max(0, Math.floor(block.x * width) - 3),
      y = Math.max(0, Math.floor(block.y * height) - 3);
    const right = Math.min(
        width,
        Math.ceil((block.x + block.width) * width) + 3,
      ),
      bottom = Math.min(
        height,
        Math.ceil((block.y + block.height) * height) + 3,
      );
    if (right <= x || bottom <= y) return [];
    return [
      {
        index,
        text: block.text,
        kind: email ? "email" : "phone",
        rect: { x, y, width: right - x, height: bottom - y },
      },
    ];
  });
}
export function filterLibrary(
  entries: HistoryEntry[],
  query: string,
  filter: "all" | "favorites" | "learning",
): HistoryEntry[] {
  const term = query.trim().toLocaleLowerCase();
  return entries.filter(
    (e) =>
      (filter !== "favorites" || e.favorite) &&
      (filter !== "learning" || e.kind === "learning") &&
      (!term ||
        [e.source, e.translation, e.example ?? ""]
          .join("\n")
          .toLocaleLowerCase()
          .includes(term)),
  );
}
export function libraryMarkdown(entries: HistoryEntry[]): string {
  // Blockquote every content line: source text cannot inject document headings/HTML.
  const quote = (value: string) =>
    value
      .replace(/[<>]/g, (c) => (c === "<" ? "&lt;" : "&gt;"))
      .split("\n")
      .map((line) => "> " + line)
      .join("\n");
  return (
    "# snapTrans\n\n" +
    entries
      .map(
        (e) =>
          "## " +
          (e.kind === "learning"
            ? "学习卡片 / Learning card"
            : "收藏 / Favorite") +
          "\n\n" +
          quote(e.source) +
          "\n\n" +
          quote(readableTranslation(e.translation)) +
          (e.example ? "\n\n" + quote(e.example) : ""),
      )
      .join("\n\n---\n\n")
  );
}
export function comparePixels(
  before: Uint8ClampedArray,
  after: Uint8ClampedArray,
  tolerance = 24,
): { pixels: Uint8ClampedArray; changed: number } {
  if (before.length !== after.length || after.length % 4 !== 0)
    throw new Error("Image dimensions do not match");
  const pixels = new Uint8ClampedArray(after);
  let changed = 0;
  for (let i = 0; i < after.length; i += 4) {
    const difference = Math.max(
      Math.abs(before[i]! - after[i]!),
      Math.abs(before[i + 1]! - after[i + 1]!),
      Math.abs(before[i + 2]! - after[i + 2]!),
      Math.abs(before[i + 3]! - after[i + 3]!),
    );
    if (difference > tolerance) {
      pixels[i] = 255;
      pixels[i + 1] = 48;
      pixels[i + 2] = 90;
      pixels[i + 3] = 255;
      changed++;
    }
  }
  return { pixels, changed };
}
export async function imageCanvas(url: string): Promise<HTMLCanvasElement> {
  const image = new Image();
  await new Promise<void>((resolve, reject) => {
    image.onload = () => resolve();
    image.onerror = () =>
      reject(new Error("无法读取图片 / Could not load image"));
    image.src = url;
  });
  if (
    !image.naturalWidth ||
    image.naturalWidth * image.naturalHeight > 16000000
  )
    throw new Error("图片请限制在 1600 万像素内 / Maximum 16 megapixels");
  const canvas = document.createElement("canvas");
  canvas.width = image.naturalWidth;
  canvas.height = image.naturalHeight;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Canvas unavailable");
  ctx.drawImage(image, 0, 0);
  return canvas;
}
let comparisonBaseline: HTMLCanvasElement | null = null;
let baselineGeneration = 0;
export function hasComparisonBaseline(): boolean {
  return comparisonBaseline !== null;
}
export function clearComparisonBaseline(): void {
  baselineGeneration++;
  comparisonBaseline = null;
}
export async function rememberComparison(url: string): Promise<void> {
  const generation = ++baselineGeneration;
  const canvas = await imageCanvas(url);
  if (generation === baselineGeneration) comparisonBaseline = canvas;
}
export async function compareScreenshot(
  url: string,
): Promise<{ image: string; percent: number }> {
  const before = comparisonBaseline;
  if (!before) throw new Error("请先记住基准截图 / Remember a baseline first");
  const after = await imageCanvas(url);
  if (before.width !== after.width || before.height !== after.height)
    throw new Error(
      "两张截图尺寸必须一致；请框选相同区域 / Select the same sized region",
    );
  const ctx = after.getContext("2d")!,
    baseCtx = before.getContext("2d")!;
  const result = comparePixels(
    baseCtx.getImageData(0, 0, before.width, before.height).data,
    ctx.getImageData(0, 0, after.width, after.height).data,
  );
  const output = ctx.createImageData(after.width, after.height);
  output.data.set(result.pixels);
  ctx.putImageData(output, 0, 0);
  return {
    image: after.toDataURL("image/png"),
    percent: (100 * result.changed) / (after.width * after.height),
  };
}
export async function redactPreview(
  url: string,
  rects: Rect[],
): Promise<string> {
  const canvas = await imageCanvas(url),
    ctx = canvas.getContext("2d")!;
  ctx.fillStyle = "#111827";
  for (const r of rects) ctx.fillRect(r.x, r.y, r.width, r.height);
  return canvas.toDataURL("image/png");
}
export async function shareCard(
  url: string,
  title: string,
  source: string,
  translation: string,
  bilingual: boolean,
): Promise<string> {
  const screenshot = await imageCanvas(url);
  const width = 960,
    padding = 48;
  const canvas = document.createElement("canvas");
  canvas.width = width;
  const ctx = canvas.getContext("2d")!;
  const lines = (text: string, size: number) => {
    ctx.font = `${size}px "Segoe UI", "Microsoft YaHei", sans-serif`;
    const rows: string[] = [];
    for (const paragraph of text.split("\n")) {
      let line = "";
      for (const char of paragraph) {
        if (ctx.measureText(line + char).width > width - padding * 2 - 48) {
          rows.push(line);
          line = char;
        } else line += char;
      }
      rows.push(line);
    }
    return rows;
  };
  if (title.length > 120 || source.length + translation.length > 12000)
    throw new Error("卡片文字过长，请缩短选区或标题 / Shorten the card text");
  const titleLines = lines(title || "snapTrans", 30);
  const sourceLines = bilingual ? lines(source, 22) : [];
  const translatedLines = bilingual ? lines(translation, 24) : [];
  const contentHeight = bilingual
    ? 64 + sourceLines.length * 34 + translatedLines.length * 36
    : Math.round(
        screenshot.height *
          Math.min(1, (width - padding * 2) / screenshot.width),
      );
  canvas.height =
    padding * 2 + titleLines.length * 42 + 32 + contentHeight + 48;
  if (canvas.height > 12000)
    throw new Error("卡片过长，请缩小选区 / Card too tall");
  const gradient = ctx.createLinearGradient(0, 0, width, canvas.height);
  gradient.addColorStop(0, "#e0e7ff");
  gradient.addColorStop(1, "#d1fae5");
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, width, canvas.height);
  ctx.fillStyle = "#0f172a";
  ctx.font = '600 30px "Segoe UI", "Microsoft YaHei", sans-serif';
  ctx.textBaseline = "top";
  titleLines.forEach((line, i) =>
    ctx.fillText(line, padding, padding + i * 42),
  );
  const top = padding + titleLines.length * 42 + 24;
  ctx.shadowColor = "#0f172a22";
  ctx.shadowBlur = 20;
  ctx.shadowOffsetY = 8;
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(padding, top, width - padding * 2, contentHeight + 24);
  ctx.shadowColor = "transparent";
  if (bilingual) {
    let y = top + 24;
    ctx.font = '22px "Segoe UI", "Microsoft YaHei", sans-serif';
    ctx.fillStyle = "#64748b";
    for (const line of sourceLines) {
      ctx.fillText(line, padding + 24, y);
      y += 34;
    }
    y += 28;
    ctx.font = '24px "Segoe UI", "Microsoft YaHei", sans-serif';
    ctx.fillStyle = "#0f172a";
    for (const line of translatedLines) {
      ctx.fillText(line, padding + 24, y);
      y += 36;
    }
  } else {
    const w =
      screenshot.width * Math.min(1, (width - padding * 2) / screenshot.width);
    ctx.drawImage(
      screenshot,
      padding + (width - padding * 2 - w) / 2,
      top + 12,
      w,
      contentHeight,
    );
  }
  ctx.fillStyle = "#64748b";
  ctx.font = "14px Segoe UI";
  ctx.fillText("snapTrans", padding, canvas.height - 32);
  return canvas.toDataURL("image/png");
}
