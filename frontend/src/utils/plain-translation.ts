import { wrapOverlayText, type MeasureText } from "./overlay-layout";

// Vision input has no OCR coordinates. Use a complete text card for both
// sizing the result and exporting it, rather than copying the untouched crop.
export function layoutPlainTranslation(text: string, width: number, measure?: MeasureText) {
  const padding = 12, fontSize = 16, lineHeight = 24;
  const lines = text.split(/\r?\n/).flatMap(paragraph =>
    paragraph.trim() ? wrapOverlayText(paragraph, fontSize, Math.max(1, width - padding * 2), measure) : [""]);
  return { width, height: Math.max(48, lines.length * lineHeight + padding * 2), padding, fontSize, lineHeight, lines };
}
