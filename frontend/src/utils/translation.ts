import type { OCRBlock } from "./selection";

export interface ParsedTranslationOutput {
  hasIndexedEntries: boolean;
  indexed: Record<number, string>;
  lines: string[];
}

export type TranslationDirection = "to-zh" | "to-en";

// detectDirectionForText decides the translation direction based on the
// dominant script in the OCR text: Chinese-dominated text is translated to
// English, otherwise to Chinese.
export function detectDirectionForText(text: string): TranslationDirection {
  const sample = text.trim();
  if (!sample) {
    return "to-zh";
  }

  const characters = Array.from(sample);
  const total = Math.max(1, characters.length);
  let hanCount = 0;
  let latinCount = 0;

  for (const char of characters) {
    if (/\s/.test(char)) {
      continue;
    }
    if (/[\p{Script=Han}]/u.test(char)) {
      hanCount += 1;
    } else if (/[A-Za-z]/.test(char)) {
      latinCount += 1;
    }
  }

  if (hanCount / total > 0.35) {
    return "to-en";
  }
  if (latinCount / total > 0.35) {
    return "to-zh";
  }
  return "to-zh";
}

export function parseTranslationOutput(rawText: string): ParsedTranslationOutput {
  const indexed: Record<number, string> = {};
  const lines: string[] = [];

  for (const rawLine of rawText.replace(/\r/g, "").split("\n")) {
    const cleaned = cleanTranslationLine(rawLine);
    if (!cleaned) {
      continue;
    }

    const indexedLine = parseIndexedLine(cleaned);
    if (indexedLine) {
      indexed[indexedLine.index] = indexedLine.text;
      lines.push(indexedLine.text);
      continue;
    }

    lines.push(cleaned);
  }

  return {
    hasIndexedEntries: Object.keys(indexed).length > 0,
    indexed,
    lines
  };
}

export function translationForOCRBlock(
  blockIndex: number,
  block: OCRBlock,
  parsed: ParsedTranslationOutput,
  blockCount: number,
  direction: TranslationDirection = "to-zh"
): string {
  const candidate = candidateForBlock(blockIndex, parsed, blockCount);
  if (!isTrustedReplacement(block.text, candidate, direction)) {
    return "";
  }
  return candidate;
}

export function isLikelyDuplicateTranslation(leftText: string, rightText: string): boolean {
  const left = normalizeDuplicateText(leftText);
  const right = normalizeDuplicateText(rightText);
  if (left.length < 18 || right.length < 18) {
    return false;
  }

  const leftListIndex = leadingListIndex(leftText);
  const rightListIndex = leadingListIndex(rightText);
  if (leftListIndex && rightListIndex && leftListIndex !== rightListIndex) {
    return false;
  }

  if (left === right) {
    return true;
  }

  const shorter = left.length <= right.length ? left : right;
  const longer = left.length <= right.length ? right : left;
  const lengthRatio = shorter.length / Math.max(1, longer.length);
  if (lengthRatio >= 0.45 && longer.includes(shorter)) {
    return true;
  }

  const prefixLength = commonPrefixLength(left, right);
  const characterSimilarity = setSimilarity(characters(left), characters(right));
  if (prefixLength >= 8 && characterSimilarity >= 0.36) {
    return true;
  }

  return diceSimilarity(ngrams(left, 2), ngrams(right, 2)) >= 0.46;
}

function candidateForBlock(blockIndex: number, parsed: ParsedTranslationOutput, blockCount: number): string {
  if (parsed.hasIndexedEntries) {
    return parsed.indexed[blockIndex + 1] ?? "";
  }

  if (parsed.lines.length === blockCount || blockCount === 1) {
    return parsed.lines[blockIndex] ?? "";
  }

  return "";
}

function cleanTranslationLine(line: string): string {
  let cleaned = line.trim();
  if (!cleaned || cleaned === "```") {
    return "";
  }

  cleaned = cleaned.replace(/\bOCR_TEXT_(BEGIN|END)\b[:：]?\s*/gi, "").trim();
  if (!cleaned || /^[-_*`:\s]*(BEGIN|END)[-_*`:\s]*$/i.test(cleaned)) {
    return "";
  }

  if (/^[-_*`:\s]*OCR[-_\s]*TEXT[-_\s]*(BEGIN|END)[-_*`:\s]*$/i.test(cleaned)) {
    return "";
  }

  return cleaned.replace(/^["'“”‘’]+|["'“”‘’]+$/g, "").trim();
}

function parseIndexedLine(line: string): { index: number; text: string } | null {
  const match = line.match(/^(?:\[(\d+)\]|(\d+)[.)、:：-])\s*(.*)$/);
  if (!match) {
    return null;
  }

  const index = Number(match[1] ?? match[2]);
  const text = cleanTranslationLine(match[3] ?? "");
  if (!Number.isInteger(index) || index <= 0 || !text) {
    return null;
  }

  return { index, text };
}

function isTrustedReplacement(sourceText: string, translatedText: string, direction: TranslationDirection): boolean {
  const source = normalizeComparableText(sourceText);
  const translated = normalizeComparableText(translatedText);
  if (!source || !translated) {
    return false;
  }

  if (/OCR[-_\s]*TEXT/i.test(translatedText)) {
    return false;
  }

  if (!hasNaturalLanguage(translatedText)) {
    return false;
  }

  if (!isTargetLanguageText(translatedText, direction)) {
    return false;
  }

  if (source === translated) {
    return true;
  }

  return true;
}

function normalizeComparableText(text: string): string {
  return text
    .trim()
    .replace(/\s+/g, " ")
    .replace(/^["'“”‘’]+|["'“”‘’]+$/g, "")
    .toLowerCase();
}

function hasNaturalLanguage(text: string): boolean {
  return /[\p{Script=Han}A-Za-z]/u.test(text);
}

function isTargetLanguageText(text: string, direction: TranslationDirection): boolean {
  const hasHan = /[\p{Script=Han}]/u.test(text);
  const hasLatin = /[A-Za-z]/.test(text);
  if (direction === "to-zh") {
    return hasHan;
  }
  return hasLatin && !hasHan;
}

function normalizeDuplicateText(text: string): string {
  return text
    .trim()
    .toLowerCase()
    .replace(/https?:\/\/\S+/g, "")
    .replace(/[^\p{Script=Han}a-z0-9]+/gu, "");
}

function leadingListIndex(text: string): string | null {
  const match = text.trim().match(/^(\d+)[.)、．]\s+/);
  return match?.[1] ?? null;
}

function commonPrefixLength(left: string, right: string): number {
  const limit = Math.min(left.length, right.length);
  let index = 0;
  while (index < limit && left[index] === right[index]) {
    index += 1;
  }
  return index;
}

function characters(text: string): string[] {
  return Array.from(text);
}

function ngrams(text: string, size: number): string[] {
  const chars = characters(text);
  if (chars.length <= size) {
    return chars.length > 0 ? [chars.join("")] : [];
  }

  const result: string[] = [];
  for (let index = 0; index <= chars.length - size; index += 1) {
    result.push(chars.slice(index, index + size).join(""));
  }
  return result;
}

function setSimilarity(leftItems: string[], rightItems: string[]): number {
  const left = new Set(leftItems);
  const right = new Set(rightItems);
  if (left.size === 0 || right.size === 0) {
    return 0;
  }

  let intersection = 0;
  for (const item of left) {
    if (right.has(item)) {
      intersection += 1;
    }
  }
  return intersection / Math.min(left.size, right.size);
}

function diceSimilarity(leftItems: string[], rightItems: string[]): number {
  const left = new Set(leftItems);
  const right = new Set(rightItems);
  if (left.size === 0 || right.size === 0) {
    return 0;
  }

  let intersection = 0;
  for (const item of left) {
    if (right.has(item)) {
      intersection += 1;
    }
  }
  return (2 * intersection) / (left.size + right.size);
}
