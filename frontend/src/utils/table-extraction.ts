import type { OCRBlockPayload } from "../services/backend";

export interface ExtractedTable {
  cells: string[][];
  rows: number;
  columns: number;
}

export interface TableSourceSize {
  width: number;
  height: number;
}

interface PositionedBlock extends OCRBlockPayload {
  text: string;
  centerY: number;
}

function median(values: number[]): number {
  if (!values.length) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2
    ? sorted[middle]!
    : (sorted[middle - 1]! + sorted[middle]!) / 2;
}

function joinCellText(current: string, next: string): string {
  const clean = next.replace(/\s*\n\s*/g, " ").trim();
  if (!current) return clean;
  const separator =
    /[\u3400-\u9fff]$/.test(current) && /^[\u3400-\u9fff]/.test(clean)
      ? ""
      : " ";
  return current + separator + clean;
}

function looksLikePaginationRow(blocks: PositionedBlock[]): boolean {
  const text = blocks.map((block) => block.text).join(" ");
  return /\d+\s*(?:条|items?|rows?)\s*\/\s*(?:页|page)/iu.test(text)
    || /(?:每页|per\s+page)\s*\d+/iu.test(text);
}

function horizontalTextScale(
  blocks: PositionedBlock[],
  typicalHeight: number,
  sourceSize?: TableSourceSize,
): number {
  if (
    sourceSize
    && Number.isFinite(sourceSize.width)
    && Number.isFinite(sourceSize.height)
    && sourceSize.width > 0
    && sourceSize.height > 0
  ) {
    // OCR x/width and y/height are normalized independently. Convert a
    // text-line height into horizontal units before using it as x tolerance.
    return typicalHeight * sourceSize.height / sourceSize.width;
  }

  return median(blocks.map((block) => {
    const characters = Array.from(block.text.replace(/\s+/g, "")).length;
    return block.width / Math.max(1, characters);
  }));
}

export function inferTable(
  blocks: OCRBlockPayload[],
  sourceSize?: TableSourceSize,
): ExtractedTable {
  const positioned: PositionedBlock[] = blocks
    .filter(
      (block) =>
        typeof block.text === "string" &&
        block.text.trim() &&
        [block.x, block.y, block.width, block.height].every(Number.isFinite) &&
        block.width > 0 &&
        block.height > 0,
    )
    .slice(0, 500)
    .map((block) => ({
      ...block,
      text: block.text.trim(),
      centerY: block.y + block.height / 2,
    }));
  if (!positioned.length) return { cells: [], rows: 0, columns: 0 };

  const typicalHeight = Math.max(0.008, median(positioned.map((b) => b.height)));
  const rowTolerance = Math.max(0.01, typicalHeight * 0.72);
  const rows: Array<{ centerY: number; height: number; blocks: PositionedBlock[] }> = [];
  for (const block of positioned.sort((a, b) => a.centerY - b.centerY || a.x - b.x)) {
    let row = rows
      .map((candidate) => ({
        candidate,
        distance: Math.abs(candidate.centerY - block.centerY),
      }))
      .filter(
        ({ candidate, distance }) =>
          distance <= Math.max(rowTolerance, (candidate.height + block.height) * 0.36),
      )
      .sort((a, b) => a.distance - b.distance)[0]?.candidate;
    if (!row) {
      row = { centerY: block.centerY, height: block.height, blocks: [] };
      rows.push(row);
    }
    row.blocks.push(block);
    row.centerY = row.blocks.reduce((sum, item) => sum + item.centerY, 0) / row.blocks.length;
    row.height = median(row.blocks.map((item) => item.height));
  }
  rows.sort((a, b) => a.centerY - b.centerY);

  while (rows.length > 1 && looksLikePaginationRow(rows[rows.length - 1]!.blocks)) {
    rows.pop();
  }

  const tableBlocks = rows.flatMap((row) => row.blocks);
  const columnTolerance = Math.min(
    0.06,
    Math.max(0.006, horizontalTextScale(tableBlocks, typicalHeight, sourceSize) * 1.5),
  );
  const anchors: Array<{ x: number; count: number }> = [];
  for (const block of [...tableBlocks].sort((a, b) => a.x - b.x)) {
    const nearest = anchors
      .map((anchor) => ({ anchor, distance: Math.abs(anchor.x - block.x) }))
      .filter(({ distance }) => distance <= columnTolerance)
      .sort((a, b) => a.distance - b.distance)[0]?.anchor;
    if (nearest) {
      nearest.x = (nearest.x * nearest.count + block.x) / (nearest.count + 1);
      nearest.count++;
    } else {
      anchors.push({ x: block.x, count: 1 });
    }
  }
  anchors.sort((a, b) => a.x - b.x);
  if (anchors.length > 30 || rows.length > 100)
    throw new Error("TABLE_TOO_LARGE");

  const cells = rows.map((row) => {
    const values = Array.from({ length: anchors.length }, () => "");
    for (const block of row.blocks.sort((a, b) => a.x - b.x)) {
      let index = 0;
      for (let i = 1; i < anchors.length; i++) {
        if (Math.abs(anchors[i]!.x - block.x) < Math.abs(anchors[index]!.x - block.x)) index = i;
      }
      values[index] = joinCellText(values[index]!, block.text);
    }
    return values;
  });
  return { cells, rows: cells.length, columns: anchors.length };
}

export function tableToTSV(cells: string[][]): string {
  return cells
    .map((row) => row.map((cell) => cell.replace(/[\t\r\n]+/g, " ").trim()).join("\t"))
    .join("\n");
}

function markdownCell(value: string): string {
  return value
    .replace(/\\/g, "\\\\")
    .replace(/\|/g, "\\|")
    .replace(/\r?\n/g, "<br>")
    .trim();
}

export function tableToMarkdown(cells: string[][]): string {
  if (!cells.length || !cells[0]?.length) return "";
  const columns = Math.max(...cells.map((row) => row.length));
  const normalized = cells.map((row) =>
    Array.from({ length: columns }, (_, index) => markdownCell(row[index] ?? "")),
  );
  const line = (row: string[]) => `| ${row.join(" | ")} |`;
  return [
    line(normalized[0]!),
    line(Array.from({ length: columns }, () => "---")),
    ...normalized.slice(1).map(line),
  ].join("\n");
}
