import type { OCRBlock } from "./selection";

export interface ParsedTranslationOutput {
  hasIndexedEntries: boolean;
  indexed: Record<number, string>;
  lines: string[];
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
  blockCount: number
): string {
  const candidate = candidateForBlock(blockIndex, parsed, blockCount);
  if (!isTrustedReplacement(block.text, candidate)) {
    return "";
  }
  return candidate;
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

function isTrustedReplacement(sourceText: string, translatedText: string): boolean {
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

  if (source === translated) {
    return false;
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
