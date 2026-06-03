import { describe, expect, it } from "vitest";
import { parseTranslationOutput, translationForOCRBlock } from "./translation";
import type { OCRBlock } from "./selection";

const block = (text: string): OCRBlock => ({ text, x: 0, y: 0, width: 0.2, height: 0.1 });

describe("parseTranslationOutput", () => {
  it("strips leaked OCR delimiters and preserves numbered translation mapping", () => {
    const parsed = parseTranslationOutput("OCR_TEXT_BEGIN\n[1] \u5341\u5927\u70ed\u95e8\u6a21\u578b\n[3] \u4e2d\u6027\nOCR_TEXT_END");

    expect(parsed.lines).toEqual(["\u5341\u5927\u70ed\u95e8\u6a21\u578b", "\u4e2d\u6027"]);
    expect(parsed.indexed[1]).toBe("\u5341\u5927\u70ed\u95e8\u6a21\u578b");
    expect(parsed.indexed[3]).toBe("\u4e2d\u6027");
  });

  it("does not trust missing, numeric, delimiter, or unchanged replacements", () => {
    const parsed = parseTranslationOutput("[1] \u5341\u5927\u70ed\u95e8\u6a21\u578b\n[2] 30\n[3] OCR_TEXT_BEGIN\n[4] DeepSeek V4 Pro");

    expect(translationForOCRBlock(0, block("Top 10 Model Popularity"), parsed, 4)).toBe("\u5341\u5927\u70ed\u95e8\u6a21\u578b");
    expect(translationForOCRBlock(1, block("30"), parsed, 4)).toBe("");
    expect(translationForOCRBlock(2, block("Neutral"), parsed, 4)).toBe("");
    expect(translationForOCRBlock(3, block("DeepSeek V4 Pro"), parsed, 4)).toBe("");
  });

  it("falls back to positional lines only when the line count matches the OCR block count", () => {
    const parsed = parseTranslationOutput("\u4e2d\u6027\n\u8d1f\u9762\n\u6b63\u9762");

    expect(translationForOCRBlock(1, block("Negative"), parsed, 3)).toBe("\u8d1f\u9762");
    expect(translationForOCRBlock(1, block("Negative"), parsed, 4)).toBe("");
  });
});
