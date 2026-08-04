<script setup lang="ts">
import { Camera, Check, Copy, Image as ImageIcon, Languages, Settings, X } from "lucide-vue-next";
import MarkdownIt from "markdown-it";
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import {
  copyImageDataUrl,
  copyText,
  defaultConfig,
  hasWailsBackend,
  hideWindow,
  loadConfig,
  onBackendEvent,
  processImage,
  saveConfig,
  showCaptureWindow,
  triggerCapture,
  type AppConfig,
  type CapturePayload,
  type OCRResultPayload,
  type TranslationDirection,
  type WorkflowErrorPayload
} from "./services/backend";
import {
  cropCanvasToDataUrl,
  fontSizeForTranslationBlock,
  isUsableSelection,
  mapCssRectToImageRect,
  mapOCRBlockToSelection,
  normalizeResultBox,
  normalizeRect,
  sampleCanvasColor,
  sampleCanvasForegroundColor,
  shouldUseFlowTranslationLayout,
  translationPaletteForColor,
  wrapTranslationText,
  type Point,
  type Rect,
  type SampledColor
} from "./utils/selection";
import { isLikelyDuplicateTranslation, parseTranslationOutput, translationForOCRBlock } from "./utils/translation";

type Phase = "idle" | "loading" | "ready" | "drawing" | "processing" | "streaming" | "done" | "error";
type AnchoredTranslationLayoutBlock = {
  key: string;
  testId: "translation-line" | "translation-cover";
  text: string;
  mapped: Rect;
  top: number;
  width: number;
  height: number;
  fontSize: number;
  lineHeight: number;
  textLayout: Record<string, string>;
  lines: string[];
};
type AnchoredTranslationLayout = {
  blocks: AnchoredTranslationLayoutBlock[];
  height: number;
};

const ANCHORED_TRANSLATION_BLOCK_GAP = 2;

const canvasRef = ref<HTMLCanvasElement | null>(null);
const resultPanelRef = ref<HTMLElement | null>(null);
const phase = ref<Phase>("idle");
const capture = ref<CapturePayload | null>(null);
const dragStart = ref<Point | null>(null);
const selection = ref<Rect | null>(null);
const resultRect = ref<Rect | null>(null);
const ocrBlocks = ref<OCRResultPayload["blocks"]>([]);
const viewport = reactive({ width: window.innerWidth, height: window.innerHeight });
const translationText = ref("");
const translationDirection = ref<TranslationDirection>("to-zh");
const currentCropDataUrl = ref("");
const errorMessage = ref("");
const settingsOpen = ref(false);
const copied = ref(false);
const copiedImage = ref(false);
const isDesktop = hasWailsBackend();
const markdown = new MarkdownIt({ breaks: true, linkify: false });
const config = reactive<AppConfig>({ ...defaultConfig });

const parsedTranslation = computed(() => parseTranslationOutput(translationText.value));

const cleanTranslationText = computed(() => parsedTranslation.value.lines.join("\n").trim());

const renderedTranslation = computed(() => {
  if (cleanTranslationText.value.length === 0) {
    return "";
  }
  return markdown.render(cleanTranslationText.value);
});

const resultStyle = computed(() => {
  const rect = resultRect.value;
  if (!rect) {
    return {};
  }

  const box = normalizeResultBox(rect, viewport);
  const height =
    usesFlowTranslationLayout.value && cleanTranslationText.value
      ? buildAnchoredTranslationLayout(box).height
      : box.height;

  return {
    left: `${box.x}px`,
    top: `${box.y}px`,
    width: `${box.width}px`,
    height: `${height}px`,
    minHeight: `${height}px`
  };
});

const resultTextStyle = computed(() => {
  const rect = resultRect.value;
  if (!rect) {
    return {};
  }

  const fontSize = Math.max(14, Math.min(22, Math.round(rect.height * 0.36)));
  return {
    fontSize: `${fontSize}px`,
    lineHeight: `${Math.round(fontSize * 1.45)}px`
  };
});

const resultActionsStyle = computed(() => {
  const rect = resultRect.value;
  if (!rect) {
    return {};
  }

  const box = normalizeResultBox(rect, viewport);
  const vertical =
    box.y + box.height + 52 > viewport.height - 8
      ? { bottom: "calc(100% + 8px)" }
      : { top: "calc(100% + 8px)" };
  const horizontal = box.x + 204 > viewport.width - 8 ? { right: "0px" } : { left: "0px" };

  return { ...vertical, ...horizontal };
});

const isResultBusy = computed(() => phase.value === "processing" || phase.value === "streaming");

const reverseDirectionTitle = computed(() =>
  translationDirection.value === "to-zh" ? "Reverse: translate Chinese to English" : "Reverse: translate English to Chinese"
);

const inlineOCRBlocks = computed(() => {
  const rect = resultRect.value;
  if (!rect) {
    return [];
  }

  const box = normalizeResultBox(rect, viewport);
  const localSelection = { x: 0, y: 0, width: box.width, height: box.height };
  return ocrBlocks.value.flatMap((block, index) => {
    const mapped = mapOCRBlockToSelection(block, localSelection);
    const text = translationForOCRBlock(
      index,
      block,
      parsedTranslation.value,
      ocrBlocks.value.length,
      translationDirection.value
    );
    if (!text) {
      return [];
    }

    const fontSize = fontSizeForTranslationBlock(text || block.text, mapped);
    const sampleRect = {
      x: box.x + mapped.x,
      y: box.y + mapped.y,
      width: mapped.width,
      height: mapped.height
    };
    const palette = translationPaletteForColor(sampleCanvasColor(canvasRef.value, sampleRect));
    const foregroundColor = sampleCanvasForegroundColor(canvasRef.value, sampleRect);

    return {
      key: `${index}-${block.text}-${mapped.x}-${mapped.y}`,
      text,
      style: {
        left: `${mapped.x}px`,
        top: `${mapped.y}px`,
        width: `${mapped.width}px`,
        height: `${mapped.height}px`,
        fontSize: `${fontSize}px`,
        lineHeight: `${Math.round(fontSize * 1.12)}px`,
        fontWeight: "500",
        overflow: "visible" as const,
        overflowWrap: "anywhere" as const,
        textAlign: "center" as const,
        whiteSpace: "normal" as const,
        wordBreak: "break-word" as const,
        ...palette,
        color: cssColorForSampledColor(foregroundColor) ?? palette.color
      }
    };
  });
});

const anchoredTranslationBlocks = computed(() => {
  const rect = resultRect.value;
  if (!rect || !usesFlowTranslationLayout.value) {
    return [];
  }

  const box = normalizeResultBox(rect, viewport);
  return buildAnchoredTranslationLayout(box).blocks.map((block) => {
    const sampleRect = {
      x: box.x + block.mapped.x,
      y: box.y + block.mapped.y,
      width: block.width,
      height: block.mapped.height
    };
    const palette = translationPaletteForColor(sampleCanvasColor(canvasRef.value, sampleRect));
    const foregroundColor = sampleCanvasForegroundColor(canvasRef.value, sampleRect);

    return {
      key: block.key,
      testId: block.testId,
      text: block.text,
      style: {
        left: `${block.mapped.x}px`,
        top: `${block.top}px`,
        width: `${block.width}px`,
        minHeight: `${block.height}px`,
        fontSize: `${block.fontSize}px`,
        lineHeight: `${block.lineHeight}px`,
        fontWeight: block.mapped.height >= 34 ? "700" : "500",
        overflow: "visible" as const,
        overflowWrap: "anywhere" as const,
        padding: "1px 2px",
        boxSizing: "border-box" as const,
        textAlign: "left" as const,
        whiteSpace: "normal" as const,
        wordBreak: "break-word" as const,
        ...palette,
        color: block.text ? cssColorForSampledColor(foregroundColor) ?? palette.color : "transparent",
        textShadow: block.text ? palette.textShadow : "none",
        ...block.textLayout
      }
    };
  });
});

const hasOCRBlockLayout = computed(
  () =>
    !usesFlowTranslationLayout.value &&
    ocrBlocks.value.length > 0 &&
    (phase.value === "streaming" || phase.value === "processing" || inlineOCRBlocks.value.length > 0)
);

const usesFlowTranslationLayout = computed(
  () => ocrBlocks.value.length > 0 && shouldUseFlowTranslationLayout(ocrBlocks.value)
);

const hasFlowTranslationLayout = computed(
  () =>
    usesFlowTranslationLayout.value &&
    ocrBlocks.value.length > 0 &&
    (phase.value === "processing" || phase.value === "streaming" || phase.value === "done")
);

const hasTranslatedOverlay = computed(
  () => hasOCRBlockLayout.value || hasFlowTranslationLayout.value
);

const selectionStyle = computed(() => {
  const rect = selection.value;
  if (!rect) {
    return {};
  }

  return {
    left: `${rect.x}px`,
    top: `${rect.y}px`,
    width: `${rect.width}px`,
    height: `${rect.height}px`
  };
});

const unsubs: Array<() => void> = [];

onMounted(async () => {
  window.addEventListener("resize", updateViewport);
  window.addEventListener("keydown", onKeyDown);
  Object.assign(config, await loadConfig());

  unsubs.push(
    onBackendEvent<CapturePayload>("capture-start", async (payload) => {
      await startCapture(payload);
    }),
    onBackendEvent("ocr-start", () => {
      phase.value = "processing";
    }),
    onBackendEvent<OCRResultPayload>("ocr-result", (payload) => {
      ocrBlocks.value = payload.blocks ?? [];
    }),
    onBackendEvent("translation-start", () => {
      phase.value = "streaming";
    }),
    onBackendEvent<string>("translation-token", (token) => {
      translationText.value += token;
      phase.value = "streaming";
    }),
    onBackendEvent("translation-done", () => {
      phase.value = "done";
    }),
    onBackendEvent<WorkflowErrorPayload>("workflow-error", (payload) => {
      if (payload.stage === "translation" && translationText.value.trim()) {
        errorMessage.value = "";
        phase.value = "done";
        return;
      }
      errorMessage.value = payload.message;
      phase.value = "error";
    }),
    onBackendEvent("settings-open", () => {
      capture.value = null;
      resultRect.value = null;
      selection.value = null;
      ocrBlocks.value = [];
      translationText.value = "";
      errorMessage.value = "";
      copied.value = false;
      copiedImage.value = false;
      phase.value = "idle";
      settingsOpen.value = true;
    })
  );
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", updateViewport);
  window.removeEventListener("keydown", onKeyDown);
  for (const unsub of unsubs) {
    unsub();
  }
});

function updateViewport(): void {
  viewport.width = window.innerWidth;
  viewport.height = window.innerHeight;
}

function onKeyDown(event: KeyboardEvent): void {
  if (event.key === "Escape" && isCaptureActive()) {
    event.preventDefault();
    void cancelCapture();
  }
}

async function startCapture(payload: CapturePayload): Promise<void> {
  capture.value = payload;
  dragStart.value = null;
  selection.value = null;
  resultRect.value = null;
  ocrBlocks.value = [];
  translationText.value = "";
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  phase.value = "loading";

  await nextTick();
  await drawCapture(payload);
  phase.value = "ready";
  await nextTick();
  await showCaptureWindow();
}

async function drawCapture(payload: CapturePayload): Promise<void> {
  const canvas = canvasRef.value;
  if (!canvas) {
    return;
  }

  const image = new Image();
  image.src = payload.image;

  await new Promise<void>((resolve, reject) => {
    image.onload = () => resolve();
    image.onerror = () => reject(new Error("Failed to load captured image"));
  });

  canvas.width = image.naturalWidth || payload.width;
  canvas.height = image.naturalHeight || payload.height;
  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("Canvas 2D context is unavailable");
  }

  context.clearRect(0, 0, canvas.width, canvas.height);
  context.drawImage(image, 0, 0, canvas.width, canvas.height);
}

function pointerPosition(event: MouseEvent): Point {
  const target = event.currentTarget as HTMLElement;
  const rect = target.getBoundingClientRect();
  return {
    x: event.clientX - rect.left,
    y: event.clientY - rect.top
  };
}

function onMouseDown(event: MouseEvent): void {
  if (phase.value !== "ready") {
    return;
  }

  const point = pointerPosition(event);
  dragStart.value = point;
  selection.value = normalizeRect(point, point);
  phase.value = "drawing";
}

function onMouseMove(event: MouseEvent): void {
  if (phase.value !== "drawing" || !dragStart.value) {
    return;
  }

  selection.value = normalizeRect(dragStart.value, pointerPosition(event));
}

async function onMouseUp(event: MouseEvent): Promise<void> {
  if (phase.value !== "drawing" || !dragStart.value) {
    return;
  }

  const rect = normalizeRect(dragStart.value, pointerPosition(event));
  selection.value = rect;
  dragStart.value = null;

  if (!isUsableSelection(rect)) {
    selection.value = null;
    phase.value = "ready";
    return;
  }

  await submitSelection(rect);
}

async function submitSelection(rect: Rect): Promise<void> {
  const canvas = canvasRef.value;
  const payload = capture.value;
  if (!canvas || !payload) {
    return;
  }

  const bounds = canvas.getBoundingClientRect();
  const imageRect = mapCssRectToImageRect(
    rect,
    { width: bounds.width, height: bounds.height },
    { width: canvas.width, height: canvas.height }
  );

  resultRect.value = rect;
  selection.value = null;
  ocrBlocks.value = [];
  translationText.value = "";
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  phase.value = "processing";

  try {
    const crop = cropCanvasToDataUrl(canvas, imageRect);
    currentCropDataUrl.value = crop;
    await processImage(crop, translationDirection.value);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "Failed to process selection";
    phase.value = "error";
  }
}

function onContextMenu(event: MouseEvent): void {
  if (!isCaptureActive()) {
    return;
  }

  event.preventDefault();
  void cancelCapture();
}

function onMainMouseDown(event: MouseEvent): void {
  if (event.button !== 0 || !resultRect.value || settingsOpen.value || isCaptureActive()) {
    return;
  }

  const target = event.target;
  if (target instanceof Node && resultPanelRef.value?.contains(target)) {
    return;
  }

  void restore();
}

function onMainContextMenu(event: MouseEvent): void {
  if (!resultRect.value || settingsOpen.value || isCaptureActive()) {
    return;
  }

  event.preventDefault();
  void restore();
}

function isCaptureActive(): boolean {
  return phase.value === "loading" || phase.value === "ready" || phase.value === "drawing";
}

async function cancelCapture(): Promise<void> {
  dragStart.value = null;
  selection.value = null;
  capture.value = null;
  resultRect.value = null;
  ocrBlocks.value = [];
  translationText.value = "";
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  currentCropDataUrl.value = "";
  phase.value = "idle";
  await hideWindow();
}

async function copyResult(): Promise<void> {
  if (!cleanTranslationText.value) {
    return;
  }

  await copyText(cleanTranslationText.value);
  copied.value = true;
  window.setTimeout(() => {
    copied.value = false;
  }, 1100);
}

async function copyTranslatedImage(): Promise<void> {
  const dataUrl = renderTranslatedSelectionImage();
  if (!dataUrl) {
    return;
  }

  await copyImageDataUrl(dataUrl);
  copiedImage.value = true;
  window.setTimeout(() => {
    copiedImage.value = false;
  }, 1100);
}

async function reverseTranslationDirection(): Promise<void> {
  if (!currentCropDataUrl.value || isResultBusy.value) {
    return;
  }

  translationDirection.value = translationDirection.value === "to-zh" ? "to-en" : "to-zh";
  ocrBlocks.value = [];
  translationText.value = "";
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  phase.value = "streaming";

  try {
    await processImage(currentCropDataUrl.value, translationDirection.value);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "Failed to reverse translation direction";
    phase.value = "error";
  }
}

function renderTranslatedSelectionImage(): string | null {
  const sourceCanvas = canvasRef.value;
  const rect = resultRect.value;
  if (!sourceCanvas || !rect) {
    return null;
  }

  const bounds = sourceCanvas.getBoundingClientRect();
  const imageRect = mapCssRectToImageRect(
    rect,
    { width: bounds.width, height: bounds.height },
    { width: sourceCanvas.width, height: sourceCanvas.height }
  );
  if (imageRect.width <= 0 || imageRect.height <= 0) {
    return null;
  }

  const usesAnchoredExport = shouldUseFlowTranslationLayout(ocrBlocks.value) && cleanTranslationText.value.length > 0;
  const exportLayout = usesAnchoredExport
    ? buildAnchoredTranslationLayout({ x: 0, y: 0, width: rect.width, height: rect.height })
    : null;
  const scaleX = imageRect.width / rect.width;
  const scaleY = imageRect.height / rect.height;

  const target = document.createElement("canvas");
  target.width = imageRect.width;
  target.height = exportLayout ? Math.max(imageRect.height, Math.ceil(exportLayout.height * scaleY)) : imageRect.height;
  const context = target.getContext("2d");
  if (!context) {
    return null;
  }

  context.drawImage(
    sourceCanvas,
    imageRect.x,
    imageRect.y,
    imageRect.width,
    imageRect.height,
    0,
    0,
    target.width,
    imageRect.height
  );

  if (exportLayout) {
    if (target.height > imageRect.height) {
      const palette = translationPaletteForColor(sampleCanvasColor(sourceCanvas, rect));
      context.fillStyle = palette.backgroundColor;
      context.fillRect(0, imageRect.height, target.width, target.height - imageRect.height);
    }

    exportLayout.blocks.forEach((block) => {
      const sampleRect = {
        x: rect.x + block.mapped.x,
        y: rect.y + block.mapped.y,
        width: block.width,
        height: block.mapped.height
      };
      const palette = translationPaletteForColor(sampleCanvasColor(sourceCanvas, sampleRect));
      const foregroundColor = sampleCanvasForegroundColor(sourceCanvas, sampleRect);
      const x = Math.round(block.mapped.x * scaleX);
      const y = Math.round(block.top * scaleY);
      const width = Math.max(1, Math.round(block.width * scaleX));
      const fontSize = Math.max(8, Math.round(block.fontSize * scaleY));
      const lineHeight = Math.max(1, Math.round(block.lineHeight * scaleY));
      const indent = Number.parseInt(block.textLayout.paddingLeft ?? "0", 10) || 0;
      const scaledIndent = Math.round(indent * scaleX);
      const paintHeight = Math.max(Math.round(block.height * scaleY), block.lines.length * lineHeight + 2);

      context.fillStyle = palette.backgroundColor;
      context.fillRect(x, y, width, paintHeight);
      if (!block.text) {
        return;
      }
      context.fillStyle = cssColorForSampledColor(foregroundColor) ?? palette.color;
      context.font = `${block.mapped.height >= 34 ? "700" : "500"} ${fontSize}px "Segoe UI", "Microsoft YaHei", sans-serif`;
      context.textAlign = "left";
      context.textBaseline = "top";
      block.lines.forEach((line, lineIndex) => {
        const lineX = x + 2 + (lineIndex === 0 ? 0 : scaledIndent);
        context.fillText(line, lineX, y + 1 + lineHeight * lineIndex, Math.max(1, width - 4 - (lineX - x)));
      });
    });

    return target.toDataURL("image/png");
  }

  const localSelection = { x: 0, y: 0, width: rect.width, height: rect.height };
  const parsed = parsedTranslation.value;
  ocrBlocks.value.forEach((block, index) => {
    const text = translationForOCRBlock(index, block, parsed, ocrBlocks.value.length, translationDirection.value);
    if (!text) {
      return;
    }

    const mapped = mapOCRBlockToSelection(block, localSelection);
    const sampleRect = {
      x: rect.x + mapped.x,
      y: rect.y + mapped.y,
      width: mapped.width,
      height: mapped.height
    };
    const palette = translationPaletteForColor(sampleCanvasColor(sourceCanvas, sampleRect));
    const foregroundColor = sampleCanvasForegroundColor(sourceCanvas, sampleRect);
    const x = Math.round(mapped.x * scaleX);
    const y = Math.round(mapped.y * scaleY);
    const width = Math.max(1, Math.round(mapped.width * scaleX));
    const height = Math.max(1, Math.round(mapped.height * scaleY));
    const fontSize = Math.max(8, Math.round(fontSizeForTranslationBlock(text, mapped) * scaleY));
    const lineHeight = Math.round(fontSize * 1.12);
    const lines = wrapTranslationText(text, fontSize, Math.max(1, width - 2));
    const textHeight = lines.length * lineHeight;
    const paintHeight = Math.max(height, textHeight);
    const paintY = Math.round(y - Math.max(0, paintHeight - height) / 2);

    context.fillStyle = palette.backgroundColor;
    context.fillRect(x, paintY, width, paintHeight);
    context.fillStyle = cssColorForSampledColor(foregroundColor) ?? palette.color;
    context.font = `500 ${fontSize}px "Segoe UI", "Microsoft YaHei", sans-serif`;
    context.textAlign = "center";
    context.textBaseline = "middle";
    lines.forEach((line, lineIndex) => {
      const lineY = paintY + paintHeight / 2 - textHeight / 2 + lineHeight * (lineIndex + 0.5);
      context.fillText(line, x + width / 2, lineY, Math.max(1, width - 2));
    });
  });

  return target.toDataURL("image/png");
}

function fontSizeForAnchoredTranslationBlock(rect: Rect): number {
  return Math.max(12, Math.min(26, Math.round(rect.height * 0.85)));
}

function buildAnchoredTranslationLayout(box: Rect): AnchoredTranslationLayout {
  const localSelection = { x: 0, y: 0, width: box.width, height: box.height };
  const mappedBlocks = ocrBlocks.value.map((block, index) => ({
    block,
    index,
    mapped: mapOCRBlockToSelection(block, localSelection)
  }));
  let nextTop = 0;
  let previousVisibleText = "";
  let previousVisibleSourceLanguage = "";

  const blocks = mappedBlocks.map((entry, entryIndex) => {
    let text = translationForOCRBlock(
      entry.index,
      entry.block,
      parsedTranslation.value,
      ocrBlocks.value.length,
      translationDirection.value
    );
    const sourceLanguage = sourceLanguageForDuplicateGuard(entry.block.text);
    if (
      text &&
      previousVisibleText &&
      previousVisibleSourceLanguage === sourceLanguage &&
      isLikelyDuplicateTranslation(previousVisibleText, text)
    ) {
      text = "";
    }
    const top = Math.max(entry.mapped.y, nextTop);
    const width = widthForAnchoredTranslationBlock(entry.mapped, box.width);
    const nextOriginalTop = mappedBlocks[entryIndex + 1]?.mapped.y ?? box.height;
    const metrics = fitAnchoredTranslationBlock(text, entry.mapped, width, top, nextOriginalTop, box.height);
    const testId: AnchoredTranslationLayoutBlock["testId"] = text ? "translation-line" : "translation-cover";

    nextTop = top + metrics.height + ANCHORED_TRANSLATION_BLOCK_GAP;
    if (text) {
      previousVisibleText = text;
      previousVisibleSourceLanguage = sourceLanguage;
    }

    return {
      key: `${entry.index}-${entry.block.text}-${entry.mapped.x}-${entry.mapped.y}`,
      testId,
      text,
      mapped: entry.mapped,
      top,
      width,
      height: metrics.height,
      fontSize: metrics.fontSize,
      lineHeight: metrics.lineHeight,
      textLayout: metrics.textLayout,
      lines: metrics.lines
    };
  });

  const height = blocks.reduce((maximum, block) => Math.max(maximum, block.top + block.height), box.height);
  return { blocks, height: Math.ceil(height) };
}

function fitAnchoredTranslationBlock(
  text: string,
  mapped: Rect,
  width: number,
  top: number,
  nextOriginalTop: number,
  selectionHeight: number
) {
  const baseFontSize = fontSizeForAnchoredTranslationBlock(mapped);
  if (!text) {
    return measureAnchoredTranslationBlock(text, mapped, width, baseFontSize);
  }

  const hasHangingIndent = shouldUseHangingIndent(text);
  const minimumFontSize = hasHangingIndent
    ? Math.max(12, Math.round(baseFontSize * 0.56))
    : Math.max(13, Math.round(baseFontSize * 0.72));
  const targetBottom = Math.max(top + mapped.height, Math.min(selectionHeight, nextOriginalTop));
  const targetHeight = Math.max(mapped.height, targetBottom - top - ANCHORED_TRANSLATION_BLOCK_GAP);
  let selected = measureAnchoredTranslationBlock(text, mapped, width, baseFontSize);
  const hasTightFollower = nextOriginalTop < selectionHeight && nextOriginalTop - top <= mapped.height * 1.8;
  const compactFontSize = Math.max(minimumFontSize, Math.round(baseFontSize * 0.75));
  const shouldReduceWidthPressure =
    hasHangingIndent && hasTightFollower && anchoredTextWidthPressure(text, width, baseFontSize) > 0.82;
  const shouldCompactDenseListRow = hasHangingIndent && hasTightFollower;
  if (selected.height <= targetHeight && !shouldReduceWidthPressure && !shouldCompactDenseListRow) {
    return selected;
  }

  for (let fontSize = baseFontSize - 1; fontSize >= minimumFontSize; fontSize -= 1) {
    const candidate = measureAnchoredTranslationBlock(text, mapped, width, fontSize);
    selected = candidate;
    const widthPressureResolved =
      !shouldReduceWidthPressure || anchoredTextWidthPressure(text, width, fontSize) <= 0.82;
    const compactFontResolved = !shouldCompactDenseListRow || fontSize <= compactFontSize;
    if (candidate.height <= targetHeight && widthPressureResolved && compactFontResolved) {
      return candidate;
    }
  }

  return selected;
}

function measureAnchoredTranslationBlock(text: string, mapped: Rect, width: number, fontSize: number) {
  const lineHeight = Math.round(fontSize * 1.24);
  const textLayout = text ? textLayoutForAnchoredTranslation(text, fontSize) : {};
  const indent = Number.parseInt(textLayout.paddingLeft ?? "0", 10) || 0;
  const lines = text ? wrapTranslationText(text, fontSize, Math.max(1, width - 4 - indent)) : [];
  const textHeight = text ? lines.length * lineHeight + 2 : lineHeight;
  return {
    fontSize,
    lineHeight,
    textLayout,
    lines,
    height: Math.max(mapped.height, textHeight)
  };
}

function widthForAnchoredTranslationBlock(rect: Rect, selectionWidth: number): number {
  return Math.max(rect.width, selectionWidth - rect.x);
}

function textLayoutForAnchoredTranslation(text: string, fontSize: number): Record<string, string> {
  if (!shouldUseHangingIndent(text)) {
    return {};
  }

  const indent = Math.round(fontSize * 1.8);
  return {
    paddingLeft: `${indent}px`,
    textIndent: `-${indent}px`
  };
}

function shouldUseHangingIndent(text: string): boolean {
  return /^\s*(?:\d+[.)、．]|[•*-])\s+/.test(text);
}

function anchoredTextWidthPressure(text: string, width: number, fontSize: number): number {
  const textLayout = textLayoutForAnchoredTranslation(text, fontSize);
  const indent = Number.parseInt(textLayout.paddingLeft ?? "0", 10) || 0;
  const availableWidth = Math.max(1, width - 4 - indent);
  const unitsPerLine = availableWidth / Math.max(1, fontSize * 0.62);
  return anchoredTextUnits(text) / Math.max(1, unitsPerLine);
}

function anchoredTextUnits(text: string): number {
  return Array.from(text.trim()).reduce((total, char) => total + anchoredCharacterUnit(char), 0);
}

function anchoredCharacterUnit(char: string): number {
  if (/\s/.test(char)) {
    return 0.35;
  }
  if (/[\p{Script=Han}]/u.test(char)) {
    return 1;
  }
  if (/[A-Za-z0-9]/.test(char)) {
    return 0.62;
  }
  return 0.5;
}

function sourceLanguageForDuplicateGuard(text: string): string {
  const hasHan = /[\p{Script=Han}]/u.test(text);
  const hasLatin = /[A-Za-z]/.test(text);
  if (hasHan && hasLatin) {
    return "mixed";
  }
  if (hasHan) {
    return "han";
  }
  if (hasLatin) {
    return "latin";
  }
  return "other";
}

function cssColorForSampledColor(color: SampledColor | null): string | null {
  if (!color) {
    return null;
  }

  return `rgb(${Math.round(color.red)}, ${Math.round(color.green)}, ${Math.round(color.blue)})`;
}

async function restore(): Promise<void> {
  phase.value = "idle";
  capture.value = null;
  selection.value = null;
  resultRect.value = null;
  ocrBlocks.value = [];
  translationText.value = "";
  currentCropDataUrl.value = "";
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  await hideWindow();
}

async function closeSettings(): Promise<void> {
  settingsOpen.value = false;
  if (isDesktop) {
    await hideWindow();
  }
}

async function saveSettings(): Promise<void> {
  await saveConfig({ ...config });
  await closeSettings();
}
</script>

<template>
  <main
    class="relative h-full w-full overflow-hidden bg-transparent text-slate-950 dark:text-slate-50"
    @mousedown="onMainMouseDown"
    @contextmenu="onMainContextMenu"
  >
    <section
      v-if="!isDesktop && phase === 'idle'"
      class="absolute inset-0 flex items-center justify-center bg-[radial-gradient(circle_at_20%_20%,rgba(16,185,129,0.20),transparent_30%),radial-gradient(circle_at_80%_30%,rgba(37,99,235,0.18),transparent_34%),linear-gradient(135deg,#f8fafc,#eef2ff_45%,#ecfdf5)] p-5"
    >
      <div class="w-full max-w-sm rounded-lg border border-white/70 bg-white/85 p-5 shadow-floating backdrop-blur-md">
        <div class="mb-5 flex items-center justify-between">
          <div>
            <div class="text-xl font-semibold tracking-normal text-slate-950">snapTrans</div>
            <div class="mt-1 text-sm text-slate-500">Instant capture translation</div>
          </div>
          <button
            class="inline-flex h-11 w-11 items-center justify-center rounded-md bg-slate-950 text-white shadow-sm transition hover:bg-slate-800"
            type="button"
            title="Capture"
            aria-label="Capture"
            @click="triggerCapture"
          >
            <Camera class="h-5 w-5" aria-hidden="true" />
          </button>
        </div>
        <div class="grid grid-cols-3 gap-2">
          <div class="h-12 rounded-md bg-emerald-500/15" />
          <div class="h-12 rounded-md bg-blue-500/15" />
          <div class="h-12 rounded-md bg-slate-900/10" />
        </div>
      </div>
    </section>

    <button
      v-if="!isDesktop && phase === 'idle'"
      class="absolute right-4 top-4 z-30 hidden h-10 w-10 items-center justify-center rounded-md border border-white/40 bg-white/90 text-slate-900 shadow-floating backdrop-blur transition hover:bg-white"
      type="button"
      title="Capture"
      aria-label="Capture"
      @click="triggerCapture"
    >
      <Camera class="h-5 w-5" aria-hidden="true" />
    </button>

    <button
      v-if="phase === 'idle' && !settingsOpen"
      class="absolute right-4 top-16 z-30 inline-flex h-10 w-10 items-center justify-center rounded-md border border-white/40 bg-white/80 text-slate-900 shadow-floating backdrop-blur transition hover:bg-white dark:border-slate-600/70 dark:bg-slate-900/80 dark:text-slate-100"
      type="button"
      title="Settings"
      aria-label="Settings"
      @click="settingsOpen = true"
    >
      <Settings class="h-5 w-5" aria-hidden="true" />
    </button>

    <section
      v-if="capture"
      class="absolute inset-0 z-10 select-none transition-opacity duration-75"
      :class="[
        phase === 'ready' || phase === 'drawing' ? 'cursor-crosshair' : 'cursor-default',
        phase === 'loading' ? 'pointer-events-none opacity-0' : 'opacity-100'
      ]"
      @mousedown.left="onMouseDown"
      @mousemove="onMouseMove"
      @mouseup.left="onMouseUp"
      @contextmenu="onContextMenu"
    >
      <canvas ref="canvasRef" class="h-full w-full object-fill" />
      <div
        v-if="selection"
        class="pointer-events-none absolute border-2 border-emerald-300 bg-emerald-200/5 shadow-[0_0_0_9999px_rgba(2,6,23,0.18)] outline outline-1 outline-white/90"
        :style="selectionStyle"
      />
    </section>

    <section
      v-if="resultRect"
      ref="resultPanelRef"
      class="absolute z-20 overflow-visible transition"
      :class="
        hasTranslatedOverlay
          ? 'rounded-md border-2 border-emerald-400 shadow-[0_0_0_1px_rgba(255,255,255,0.70),0_8px_28px_rgba(16,185,129,0.18)]'
          : 'rounded-md border border-white/70 bg-white/92 p-2 shadow-[0_10px_36px_rgba(15,23,42,0.26)] ring-1 ring-slate-900/5 backdrop-blur-[2px] dark:border-slate-700/70 dark:bg-zinc-950/92'
      "
      :style="resultStyle"
    >
      <template v-if="hasFlowTranslationLayout && phase !== 'error'">
        <div
          v-if="phase === 'processing' || (phase === 'streaming' && !translationText)"
          class="absolute left-0 top-0 inline-flex h-7 items-center gap-2 rounded bg-white/90 px-2 text-xs text-slate-700 shadow-sm backdrop-blur dark:bg-zinc-950/90 dark:text-slate-200"
        >
          <span class="h-3 w-3 animate-spin rounded-full border-2 border-emerald-500 border-t-transparent" />
          <span>{{ phase === "processing" ? "OCR..." : "Translating..." }}</span>
        </div>
        <template v-else>
          <div
            v-for="block in anchoredTranslationBlocks"
            :key="block.key"
            class="absolute"
            :style="block.style"
            :data-testid="block.testId"
          >
            {{ block.text }}
          </div>
        </template>
      </template>

      <template v-else-if="hasOCRBlockLayout && phase !== 'error'">
        <div
          v-if="phase === 'streaming' && !translationText"
          class="absolute left-0 top-0 inline-flex h-7 items-center gap-2 rounded bg-white/90 px-2 text-xs text-slate-700 shadow-sm backdrop-blur dark:bg-zinc-950/90 dark:text-slate-200"
        >
          <span class="h-3 w-3 animate-spin rounded-full border-2 border-emerald-500 border-t-transparent" />
          <span>Translating...</span>
        </div>
        <div
          v-for="block in inlineOCRBlocks"
          :key="block.key"
          class="absolute flex items-center justify-center text-center font-medium"
          :style="block.style"
          data-testid="ocr-block"
        >
          {{ block.text }}
        </div>
      </template>

      <template v-else>
        <div v-if="phase === 'processing'" class="flex h-full items-center gap-3 text-sm text-slate-700 dark:text-slate-200">
          <span class="h-4 w-4 animate-spin rounded-full border-2 border-emerald-500 border-t-transparent" />
          <span>OCR...</span>
        </div>

        <div
          v-else-if="phase === 'streaming' && !translationText"
          class="flex h-full items-center gap-3 text-sm text-slate-700 dark:text-slate-200"
        >
          <span class="h-4 w-4 animate-spin rounded-full border-2 border-emerald-500 border-t-transparent" />
          <span>Translating...</span>
        </div>

        <div v-else-if="phase === 'error'" class="h-full overflow-auto text-sm leading-6 text-rose-700 dark:text-rose-300">
          {{ errorMessage }}
        </div>

        <div
          v-else
          class="markdown-body h-full overflow-auto pr-1 text-slate-950 dark:text-slate-100"
          :style="resultTextStyle"
          v-html="renderedTranslation"
        />
      </template>

      <div
        class="absolute flex h-10 items-center gap-2 rounded-md border border-white/70 bg-white/95 px-2 shadow-[0_10px_32px_rgba(15,23,42,0.22)] backdrop-blur dark:border-slate-700/70 dark:bg-zinc-950/95"
        :style="resultActionsStyle"
      >
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md bg-slate-900 text-white transition hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-slate-100 dark:text-slate-950 dark:hover:bg-white"
          type="button"
          title="Copy translated text"
          aria-label="Copy translated text"
          :disabled="!cleanTranslationText"
          @click="copyResult"
        >
          <Check v-if="copied" class="h-4 w-4" aria-hidden="true" />
          <Copy v-else class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-300 bg-white text-slate-800 transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:bg-zinc-900 dark:text-slate-100 dark:hover:bg-zinc-800"
          type="button"
          title="Copy translated screenshot"
          aria-label="Copy translated screenshot"
          :disabled="!cleanTranslationText"
          @click="copyTranslatedImage"
        >
          <Check v-if="copiedImage" class="h-4 w-4" aria-hidden="true" />
          <ImageIcon v-else class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-300 bg-white text-slate-800 transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:bg-zinc-900 dark:text-slate-100 dark:hover:bg-zinc-800"
          type="button"
          :title="reverseDirectionTitle"
          aria-label="Reverse translation direction"
          :disabled="!currentCropDataUrl || isResultBusy"
          @click="reverseTranslationDirection"
        >
          <Languages class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-300 bg-white text-slate-800 transition hover:bg-slate-50 dark:border-slate-700 dark:bg-zinc-900 dark:text-slate-100 dark:hover:bg-zinc-800"
          type="button"
          title="Close"
          aria-label="Close"
          @click="restore"
        >
          <X class="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
    </section>

    <section
      v-if="settingsOpen"
      class="absolute inset-0 z-40 flex items-center justify-center bg-slate-950/50 p-4 backdrop-blur-sm"
      @mousedown.self="closeSettings"
    >
      <form
        class="w-full max-w-md rounded-lg border border-white/40 bg-white p-5 shadow-floating dark:border-slate-700 dark:bg-zinc-950"
        @submit.prevent="saveSettings"
      >
        <div class="mb-5 flex items-center justify-between">
          <h1 class="text-base font-semibold">Settings</h1>
          <button
            class="inline-flex h-8 w-8 items-center justify-center rounded-md text-slate-500 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-zinc-800"
            type="button"
            title="Close"
            aria-label="Close"
            @click="closeSettings"
          >
            <X class="h-4 w-4" aria-hidden="true" />
          </button>
        </div>

        <label class="mb-3 block text-sm font-medium">
          <span class="mb-1 block">LLM API Key</span>
          <input
            v-model="config.apiKey"
            class="h-10 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none transition focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 dark:border-slate-700 dark:bg-zinc-900"
            type="password"
            autocomplete="off"
            placeholder="LiteLLM Virtual Key"
          />
        </label>

        <label class="mb-3 block text-sm font-medium">
          <span class="mb-1 block">LLM API Base URL</span>
          <input
            v-model="config.baseURL"
            class="h-10 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none transition focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 dark:border-slate-700 dark:bg-zinc-900"
            type="url"
            placeholder="https://your-litellm-host/v1"
          />
          <span class="mt-1 block text-xs font-normal text-slate-500">LiteLLM 地址通常以 /v1 结尾</span>
        </label>

        <label class="mb-3 block text-sm font-medium">
          <span class="mb-1 block">Model</span>
          <input
            v-model="config.model"
            class="h-10 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none transition focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 dark:border-slate-700 dark:bg-zinc-900"
            type="text"
            placeholder="gemini/gemini-3.5-flash-lite"
          />
        </label>
        <label class="mb-3 block text-sm font-medium">
          <span class="mb-1 block">Shortcut</span>
          <input
            v-model="config.shortcutKey"
            class="h-10 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none transition focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 dark:border-slate-700 dark:bg-zinc-900"
            type="text"
          />
        </label>

        <label class="mb-3 block text-sm font-medium">
          <span class="mb-1 block">RapidOCR Path</span>
          <input
            v-model="config.rapidOCRPath"
            class="h-10 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none transition focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 dark:border-slate-700 dark:bg-zinc-900"
            type="text"
          />
        </label>

        <div class="mt-5 flex justify-end gap-2">
          <button
            class="h-9 rounded-md border border-slate-300 bg-white px-3 text-sm font-medium text-slate-800 hover:bg-slate-50 dark:border-slate-700 dark:bg-zinc-900 dark:text-slate-100 dark:hover:bg-zinc-800"
            type="button"
            @click="closeSettings"
          >
            Cancel
          </button>
          <button
            class="h-9 rounded-md bg-emerald-600 px-3 text-sm font-medium text-white hover:bg-emerald-500"
            type="submit"
          >
            Save
          </button>
        </div>
      </form>
    </section>
  </main>
</template>
