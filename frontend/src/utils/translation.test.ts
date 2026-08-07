import { describe, expect, it } from "vitest";
import {
  detectDirectionForText,
  isLikelyDuplicateTranslation,
  parseTranslationOutput,
  translationForOCRBlock
} from "./translation";
import type { OCRBlock } from "./selection";

const block = (text: string): OCRBlock => ({ text, x: 0, y: 0, width: 0.2, height: 0.1 });

describe("detectDirectionForText", () => {
  it("translates Chinese-dominated text to English", () => {
    expect(detectDirectionForText("\u5982\u679c\u5c1a\u672a\u5b89\u88c5 Wails\uff0c\u4e5f\u53ef\u4ee5\u7528\u6a21\u62df\u622a\u56fe")).toBe("to-en");
  });

  it("translates Latin-dominated text to Chinese", () => {
    expect(detectDirectionForText("Neutral\nNegative\nPositive")).toBe("to-zh");
  });

  it("defaults to Chinese for empty text", () => {
    expect(detectDirectionForText("")).toBe("to-zh");
    expect(detectDirectionForText("   ")).toBe("to-zh");
  });

  it("defaults to Chinese for numeric-only text", () => {
    expect(detectDirectionForText("30\n42\n100")).toBe("to-zh");
  });
});

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

  it("keeps unchanged lines that are already in the target language", () => {
    const parsed = parseTranslationOutput(
      "[1] \u5982\u679c\u5c1a\u672a\u5b89\u88c5 Wails\uff0c\u4f60\u53ef\u4ee5\u9884\u89c8\u524d\u7aef\n[2] If Wails is not installed yet"
    );

    expect(
      translationForOCRBlock(
        0,
        block("\u5982\u679c\u5c1a\u672a\u5b89\u88c5 Wails\uff0c\u4f60\u53ef\u4ee5\u9884\u89c8\u524d\u7aef"),
        parsed,
        2,
        "to-zh"
      )
    ).toBe("\u5982\u679c\u5c1a\u672a\u5b89\u88c5 Wails\uff0c\u4f60\u53ef\u4ee5\u9884\u89c8\u524d\u7aef");
    expect(translationForOCRBlock(1, block("If Wails is not installed yet"), parsed, 2, "to-zh")).toBe("");
    expect(
      translationForOCRBlock(
        0,
        block("\u5982\u679c\u5c1a\u672a\u5b89\u88c5 Wails\uff0c\u4f60\u53ef\u4ee5\u9884\u89c8\u524d\u7aef"),
        parsed,
        2,
        "to-en"
      )
    ).toBe("");
    expect(translationForOCRBlock(1, block("If Wails is not installed yet"), parsed, 2, "to-en")).toBe(
      "If Wails is not installed yet"
    );
  });
});

describe("isLikelyDuplicateTranslation", () => {
  it("detects adjacent paraphrases of the same translated paragraph", () => {
    expect(
      isLikelyDuplicateTranslation(
        "\u6211\u5e0c\u671b\u80fd\u591f\u5ba1\u8ba1\u6d41\u7a0b\u4e0e\u7ed3\u679c\uff0c\u7528\u4e8e\u8c03\u8bd5\u53ca\u5b9a\u671f\u9a8c\u8bc1\u6a21\u578b\u8f93\u51fa\u3002\u56e0\u6b64\u7ed3\u679c\u8bb0\u5f55\u5728 Google Sheet \u4e2d\uff0c\u53ef\u67e5\u770b\u63d0\u53ca\u7279\u5b9a\u6a21\u578b\u7684\u8bc4\u8bba ID \u53ca\u6a21\u578b\u5224\u5b9a\u7684\u60c5\u611f\u503e\u5411\u3002",
        "\u6211\u5e0c\u671b\u80fd\u591f\u5ba1\u8ba1\u6d41\u7a0b\u548c\u7ed3\u679c\uff0c\u7528\u4e8e\u8c03\u8bd5\u4ee5\u53ca\u5076\u5c14\u5bf9\u6a21\u578b\u8f93\u51fa\u8fdb\u884c\u5408\u7406\u6027\u68c0\u67e5\u3002\u56e0\u6b64\uff0c\u7ed3\u679c\u4f1a\u8bb0\u5f55\u5230 Google \u8868\u683c\u4e2d\uff0c\u4f60\u53ef\u4ee5\u770b\u5230\u63d0\u53ca\u7279\u5b9a\u6a21\u578b\u7684\u8bc4\u8bba ID \u4ee5\u53ca\u6a21\u578b\u5224\u65ad\u51fa\u7684\u60c5\u611f\u503e\u5411\u3002"
      )
    ).toBe(true);
  });

  it("does not collapse distinct numbered process steps", () => {
    expect(
      isLikelyDuplicateTranslation(
        "2. \u63d0\u793a\u4e00\u4e2a LLM \u7b5b\u9009\u51fa\u6807\u9898\u6d89\u53ca LLM \u6216\u7f16\u7801\u7684\u5e16\u5b50\u3002",
        "3. \u5bf9\u6bcf\u7bc7\u5e16\u5b50\uff0c\u5c06\u6807\u9898\u548c\u8bc4\u8bba\u53d1\u9001\u81f3 Gemini \u8fdb\u884c\u8bc4\u4f30\u3002"
      )
    ).toBe(false);
  });
});
