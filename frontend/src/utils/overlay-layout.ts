import {
  fontSizeForTranslationBlock,
  mapOCRBlockToSelection,
  type OCRBlock,
  type Rect,
  type Size,
} from "./selection";
import {
  translationForOCRBlock,
  type ParsedTranslationOutput,
  type TranslationDirection,
} from "./translation";

export const OVERLAY_FONT_FAMILY =
  '"Microsoft YaHei UI", "Microsoft YaHei", "Segoe UI", sans-serif';
export type MeasureText = (text: string, fontSize: number) => number;
export interface SourceRegion {
  key: string;
  indices: number[];
  bounds: Rect;
  availableHeight: number;
  fontSize: number;
  kind: "prose" | "label";
}
export interface OverlayBlock extends SourceRegion {
  text: string;
  lines: string[];
  lineHeight: number;
  indent: number;
  height: number;
  truncated: boolean;
}
export interface OverlayLayout {
  blocks: OverlayBlock[];
  truncated: boolean;
}

// Keep social identity and OCR-confused counters in the original screenshot.
export function preserveSourceIdentity(text: string): boolean {
  return (
    (/@[A-Za-z0-9_]+/.test(text) && text.length < 90) ||
    /^[^\p{Script=Han}A-Za-z]*[\d,.]+\s*[kKmM万亿]?\s*$/u.test(text.trim()) ||
    /^[QqlIt\d.,\s]{0,7}\s+\d[\d,.\s万亿kKmM]*$/u.test(text.trim())
  );
}
function prose(text: string, rect: Rect): boolean {
  const value = text.trim();
  return (
    value.length >= 30 ||
    value.split(/\s+/).length >= 5 ||
    (value.length >= 18 &&
      value.split(/\s+/).length >= 3 &&
      rect.width / rect.height >= 5.5) ||
    (/[，。！？：；]/u.test(value) && value.length >= 12)
  );
}
function startsList(text: string): boolean {
  return /^\s*(?:\d+[.)、．]\s*|[•*✓✔✅☑\-]\s*)/u.test(text);
}
function median(values: number[]): number {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.floor(sorted.length / 2)] ?? 18;
}
function union(a: Rect, b: Rect): Rect {
  const x = Math.min(a.x, b.x),
    y = Math.min(a.y, b.y);
  return {
    x,
    y,
    width: Math.max(a.x + a.width, b.x + b.width) - x,
    height: Math.max(a.y + a.height, b.y + b.height) - y,
  };
}
function overlapsX(a: Rect, b: Rect): boolean {
  return Math.min(a.x + a.width, b.x + b.width) > Math.max(a.x, b.x) + 2;
}

// Geometry is computed only when OCR/capture changes, never from streamed text.
export function buildSourceRegions(
  blocks: OCRBlock[],
  size: Size,
  markerInsets: number[] = [],
  nativeSelection = false,
): SourceRegion[] {
  const entries = blocks.map((block, index) => {
    const mapped = mapOCRBlockToSelection(block, {
      width: size.width,
      height: size.height,
      x: 0,
      y: 0,
    });
    const inset = Math.min(
      markerInsets[index] ?? 0,
      Math.max(0, mapped.width - 1),
    );
    const right = Math.min(size.width, mapped.x + mapped.width),
      bottom = Math.min(size.height, mapped.y + mapped.height);
    const x = Math.max(0, mapped.x + inset),
      y = Math.max(0, mapped.y);
    return {
      block,
      index,
      rect: {
        x,
        y,
        width: Math.max(1, right - x),
        height: Math.max(1, bottom - y),
      },
      marker: inset > 0,
    };
  });
  const content = entries.filter((e) => nativeSelection || !preserveSourceIdentity(e.block.text));
  const typicalHeight = median(content.map((e) => e.rect.height));
  const regions: SourceRegion[] = [];
  const sorted = [...content].sort(
    (a, b) => a.rect.y - b.rect.y || a.rect.x - b.rect.x,
  );
  for (const entry of sorted) {
    const previous = regions.at(-1);
    const lastIndex = previous?.indices.at(-1);
    const last = lastIndex === undefined ? undefined : entries[lastIndex];
    const gap = last
      ? entry.rect.y - (last.rect.y + last.rect.height)
      : Infinity;
    const merge =
      previous &&
      last &&
      previous.kind === "prose" &&
      !entry.marker &&
      !startsList(entry.block.text) &&
      !/[.!?。！？:：;；]\s*$/u.test(last.block.text) &&
      (nativeSelection || !preserveSourceIdentity(entry.block.text)) &&
      gap >= -Math.min(last.rect.height, entry.rect.height) * 0.3 &&
      gap <= Math.max(last.rect.height, entry.rect.height) * 0.38 &&
      Math.abs(entry.rect.x - last.rect.x) <=
        Math.max(4, typicalHeight * 0.3) &&
      Math.max(last.rect.height, entry.rect.height) /
        Math.min(last.rect.height, entry.rect.height) <
        1.35 &&
      // Never merge around an intervening heading or a different column.
      !entries.some(
        (other) =>
          other.index !== lastIndex &&
          other.index !== entry.index &&
          other.rect.y > last.rect.y + last.rect.height * 0.4 &&
          other.rect.y < entry.rect.y &&
          overlapsX(other.rect, entry.rect),
      );
    if (merge) {
      previous.indices.push(entry.index);
      previous.bounds = union(previous.bounds, entry.rect);
      continue;
    }
    const kind = nativeSelection || prose(entry.block.text, entry.rect) ? "prose" : "label";
    const height = entry.rect.height;
    const fontHeight =
      height >= typicalHeight * 0.75 && height <= typicalHeight * 1.3
        ? typicalHeight
        : height;
    regions.push({
      key: String(entry.index),
      indices: [entry.index],
      bounds: entry.rect,
      availableHeight: height,
      fontSize: Math.max(10, Math.min(26, Math.round(fontHeight * 0.75))),
      kind,
    });
  }
  for (const region of regions) {
    const bottom = region.bounds.y + region.bounds.height;
    let nextTop = size.height;
    for (const entry of entries) {
      if (
        region.indices.includes(entry.index) ||
        !overlapsX(region.bounds, entry.rect)
      )
        continue;
      if (entry.rect.y >= bottom - 2) nextTop = Math.min(nextTop, entry.rect.y);
    }
    const extra =
      region.kind === "prose"
        ? Math.min(region.fontSize * 0.8, Math.max(0, nextTop - bottom) * 0.5)
        : 0;
    region.availableHeight = Math.max(
      1,
      Math.min(size.height - region.bounds.y, region.bounds.height + extra),
    );
  }
  return regions;
}

export const approximateTextWidth: MeasureText = (text, fontSize) =>
  Array.from(text).reduce(
    (sum, char) =>
      sum +
      fontSize *
        (/\s/.test(char)
          ? 0.3
          : /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}，。！？、：；（）]/u.test(
                char,
              )
            ? 1
            : /[A-Z]/.test(char)
              ? 0.65
              : 0.55),
    0,
  );

export function wrapOverlayText(
  text: string,
  fontSize: number,
  width: number,
  measure: MeasureText = approximateTextWidth,
  indent = 0,
): string[] {
  const lines: string[] = [];
  for (const paragraph of text.split("\n")) {
    let line = "";
    const tokens =
      paragraph.match(
        /[A-Za-z0-9]+(?:['’./:@_-][A-Za-z0-9]+)*[，。！？、：；）】》」』”’.,!?;:]*|\s+|[（【《「『“‘]*[^\s][，。！？、：；）】》」』”’.,!?;:]*/gu,
      ) ?? [];
    for (const token of tokens) {
      if (
        measure(line + token, fontSize) <=
        width - (lines.length > 0 ? indent : 0)
      ) {
        line += token;
        continue;
      }
      if (line.trim()) {
        lines.push(line.trimEnd());
        line = "";
      }
      if (!token.trim()) continue;
      // Only break a word when the word itself cannot fit the column.
      for (const char of Array.from(token)) {
        if (
          line &&
          measure(line + char, fontSize) >
            width - (lines.length > 0 ? indent : 0)
        ) {
          lines.push(line);
          line = "";
        }
        line += char;
      }
    }
    if (line.trim()) lines.push(line.trimEnd());
  }
  return lines;
}
function ellipsize(
  text: string,
  fontSize: number,
  width: number,
  measure: MeasureText,
): string {
  let value = text.trimEnd();
  while (value && measure(value + "…", fontSize) > width)
    value = Array.from(value).slice(0, -1).join("");
  return value + "…";
}
function joinTranslation(parts: string[]): string {
  let result = "";
  for (const part of parts) {
    if (!part.trim()) continue;
    if (
      result &&
      /[A-Za-z0-9.!?;:,)\]]$/.test(result) &&
      /^[A-Za-z0-9]/.test(part)
    )
      result += " ";
    result += part.trim();
  }
  return result;
}
export function layoutTranslations(
  regions: SourceRegion[],
  sources: OCRBlock[],
  parsed: ParsedTranslationOutput,
  direction: TranslationDirection,
  measure: MeasureText = approximateTextWidth,
): OverlayLayout {
  const blocks: OverlayBlock[] = [];
  for (const region of regions) {
    const parts = region.indices.map((index) =>
      translationForOCRBlock(
        index,
        sources[index]!,
        parsed,
        sources.length,
        direction,
      ),
    );
    const text = joinTranslation(parts);
    if (!text) continue; // Leave source visible until a usable translation arrives.
    const width = Math.max(1, region.bounds.width - 2);
    let fontSize =
      region.kind === "label"
        ? Math.min(
            region.fontSize,
            fontSizeForTranslationBlock(text, region.bounds),
          )
        : region.fontSize;
    fontSize = Math.max(Math.min(10, region.fontSize), fontSize);
    const prefix = text.match(/^(?:\d+[.)、．]|[•*\-])\s+/)?.[0] ?? "";
    let indent = prefix ? Math.min(width * 0.25, measure(prefix, fontSize)) : 0;
    let lineHeight = Math.round(fontSize * 1.25);
    let lines = wrapOverlayText(text, fontSize, width, measure, indent);
    // At most two pixels of fitting: do not turn overflowing text into tiny print.
    const minFont = Math.max(10, region.fontSize - 2);
    while (
      lines.length * lineHeight > region.availableHeight &&
      fontSize > minFont
    ) {
      fontSize--;
      lineHeight = Math.round(fontSize * 1.25);
      indent = prefix ? Math.min(width * 0.25, measure(prefix, fontSize)) : 0;
      lines = wrapOverlayText(text, fontSize, width, measure, indent);
    }
    const capacity = Math.max(
      1,
      Math.floor(region.availableHeight / lineHeight),
    );
    const truncated = lines.length > capacity;
    if (truncated) {
      lines = lines.slice(0, capacity);
      lines[lines.length - 1] = ellipsize(
        lines.at(-1)!,
        fontSize,
        width - (capacity > 1 ? indent : 0),
        measure,
      );
    }
    blocks.push({
      ...region,
      fontSize,
      text,
      lines,
      lineHeight,
      indent,
      height: Math.max(
        region.bounds.height,
        Math.min(region.availableHeight, lines.length * lineHeight),
      ),
      truncated,
    });
  }
  return { blocks, truncated: blocks.some((b) => b.truncated) };
}
