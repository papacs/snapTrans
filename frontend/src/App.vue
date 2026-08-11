<script setup lang="ts">
import {
  Bot,
  Camera,
  Check,
  Copy,
  Cpu,
  Eye,
  EyeOff,
  FolderOpen,
  History,
  Image as ImageIcon,
  Keyboard,
  Languages,
  RefreshCw,
  Settings,
  Sparkles,
  X
} from "lucide-vue-next";
import MarkdownIt from "markdown-it";
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import ScreenshotEditor from "./components/ScreenshotEditor.vue";
import {
  clearHistory,
  copyImageDataUrl,
  copyText,
  defaultConfig,
  frontendReady,
  getEnvironmentStatus,
  getHistory,
  getVersion,
  hasWailsBackend,
  hideWindow,
  isAutoStartEnabled,
  loadConfig,
  onBackendEvent,
  openLogFolder,
  processImage,
  saveConfig,
  saveScreenshot,
  setAutoStart,
  showCaptureWindow,
  showSettingsWindow,
  testConnection,
  triggerCapture,
  type AppConfig,
  type CapturePayload,
  type EnvironmentStatus,
  type GenerationEvent,
  type HistoryEntry,
  type OCRBlockPayload,
  type OCRResultEvent,
  type TranslationDirection,
  type TranslationDirectionEvent,
  type TranslationTokenEvent,
  type WorkflowErrorPayload
} from "./services/backend";
import {
  clampPointToBounds,
  cropCanvasToDataUrlAsync,
  fitTranslationFontSize,
  fontSizeForTranslationBlock,
  isUsableSelection,
  mapCssRectToImageRect,
  mapOCRBlockToSelection,
  normalizeResultBox,
  normalizeRect,
  sampleCanvasColor,
  sampleCanvasForegroundColor,
  selectionBadgePosition,
  shouldUseFlowTranslationLayout,
  translationPaletteForColor,
  wrapTranslationText,
  type Point,
  type Rect,
  type SampledColor
} from "./utils/selection";
import {
  isLikelyDuplicateTranslation,
  parseTranslationOutput,
  translationForOCRBlock
} from "./utils/translation";
import { shortcutKeyFromKeyboardEvent } from "./utils/shortcut";
import {
  normalizeSettingsLocale,
  settingsMessages,
  type SettingsLocale
} from "./i18n/settings";

type Phase = "idle" | "loading" | "ready" | "drawing" | "editing" | "processing" | "streaming" | "done" | "error";
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
const captureLayerRef = ref<HTMLElement | null>(null);
const resultPanelRef = ref<HTMLElement | null>(null);
const phase = ref<Phase>("idle");
const capture = ref<CapturePayload | null>(null);
const dragStart = ref<Point | null>(null);
const selection = ref<Rect | null>(null);
const resultRect = ref<Rect | null>(null);
const ocrBlocks = ref<OCRBlockPayload[]>([]);
const viewport = reactive({ width: window.innerWidth, height: window.innerHeight });
const translationText = ref("");
const translationDirection = ref<TranslationDirection>("to-zh");
const manualDirection = ref(false);
const resolvedTranslationDirection = computed<"to-zh" | "to-en">(() =>
  translationDirection.value === "to-en" ? "to-en" : "to-zh"
);
const currentCropDataUrl = ref("");
const workflowGeneration = ref(0);
let captureLoadSequence = 0;
const errorMessage = ref("");
const settingsOpen = ref(false);
const settingsError = ref("");
const settingsSaving = ref(false);
const historyEntries = ref<HistoryEntry[]>([]);
const historyLoaded = ref(false);
const testStatus = ref<"idle" | "testing" | "ok" | "error">("idle");
const testMessage = ref("");
const shortcutRecording = ref<"translate" | "screenshot" | null>(null);
const screenshotNotice = ref("");
const envStatus = ref<EnvironmentStatus | null>(null);
const autoStartEnabled = ref(false);
const autoStartTouched = ref(false);
const appVersion = ref("");
const showAPIKey = ref(false);
const copied = ref(false);
const copiedImage = ref(false);
const isDesktop = hasWailsBackend();
const markdown = new MarkdownIt({ breaks: true, linkify: false, html: false });
const config = reactive<AppConfig>({ ...defaultConfig });
const settingsLocale = computed(() => normalizeSettingsLocale(config.uiLanguage));
const settingsText = computed(() => settingsMessages[settingsLocale.value]);

watch(
  settingsLocale,
  (locale) => {
    document.documentElement.lang = locale;
  },
  { immediate: true }
);

function setSettingsLocale(locale: SettingsLocale): void {
  config.uiLanguage = locale;
}

let pendingTranslationText = "";
let translationFrame: number | null = null;
let translationFrameUsesTimeout = false;

function commitPendingTranslationText(): void {
  if (!pendingTranslationText) {
    return;
  }
  translationText.value += pendingTranslationText;
  pendingTranslationText = "";
}

function queueTranslationText(token: string): void {
  pendingTranslationText += token;
  if (translationFrame !== null) {
    return;
  }

  if (typeof globalThis.requestAnimationFrame === "function") {
    translationFrameUsesTimeout = false;
    translationFrame = globalThis.requestAnimationFrame(() => {
      translationFrame = null;
      commitPendingTranslationText();
    });
    return;
  }

  translationFrameUsesTimeout = true;
  translationFrame = window.setTimeout(() => {
    translationFrame = null;
    commitPendingTranslationText();
  }, 16);
}

function flushTranslationText(): void {
  if (translationFrame !== null) {
    if (translationFrameUsesTimeout) {
      window.clearTimeout(translationFrame);
    } else if (typeof globalThis.cancelAnimationFrame === "function") {
      globalThis.cancelAnimationFrame(translationFrame);
    }
    translationFrame = null;
  }
  commitPendingTranslationText();
}

function resetTranslationText(): void {
  if (translationFrame !== null) {
    if (translationFrameUsesTimeout) {
      window.clearTimeout(translationFrame);
    } else if (typeof globalThis.cancelAnimationFrame === "function") {
      globalThis.cancelAnimationFrame(translationFrame);
    }
  }
  translationFrame = null;
  pendingTranslationText = "";
  translationText.value = "";
}

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
  let height =
    usesFlowTranslationLayout.value && cleanTranslationText.value
      ? buildAnchoredTranslationLayout(box).height
      : box.height;

  if (phase.value === "error") {
    const errorLines = wrapTranslationText(errorMessage.value, 14, Math.max(80, box.width - 24));
    height = Math.max(height, Math.min(196, Math.max(112, errorLines.length * 21 + 58)));
  }

  height = Math.min(height, Math.max(48, viewport.height - 16));
  const top = Math.max(8, Math.min(box.y, viewport.height - height - 8));

  return {
    left: `${box.x}px`,
    top: `${top}px`,
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

  const box = normalizeResultBox(rect, viewport);
  const preferredFontSize = Math.max(14, Math.min(22, Math.round(box.height * 0.36)));
  const availableRect = {
    x: 0,
    y: 0,
    width: Math.max(1, box.width - 20),
    height: Math.max(1, box.height - 16)
  };
  const fontSize = cleanTranslationText.value
    ? fitTranslationFontSize(cleanTranslationText.value, availableRect, preferredFontSize, 1.3)
    : preferredFontSize;
  return {
    fontSize: `${fontSize}px`,
    lineHeight: `${Math.round(fontSize * 1.3)}px`
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
const showsResultProgress = computed(
  () => phase.value === "processing" || (phase.value === "streaming" && !translationText.value)
);
const resultProgressLabel = computed(() => phase.value === "processing" ? "OCR..." : "Translating...");

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
      resolvedTranslationDirection.value
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

const showsTranslatedOverlay = computed(
  () => hasTranslatedOverlay.value && phase.value !== "error"
);

const preservesSourcePreview = computed(
  () =>
    showsTranslatedOverlay.value ||
    phase.value === "processing" ||
    (phase.value === "streaming" && !translationText.value)
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

const selectionBadgeStyle = computed(() => {
  const rect = selection.value;
  if (!rect) {
    return {};
  }

  const point = selectionBadgePosition(rect, viewport);
  return {
    left: `${point.x}px`,
    top: `${point.y}px`
  };
});

const unsubs: Array<() => void> = [];

onMounted(async () => {
  window.addEventListener("resize", updateViewport);
  window.addEventListener("keydown", onKeyDown);

  unsubs.push(
    onBackendEvent<CapturePayload>("capture-start", async (payload) => {
      await startCapture(payload);
    }),
    onBackendEvent<GenerationEvent>("ocr-start", (payload) => {
      if (!isCurrentGeneration(payload)) {
        return;
      }
      phase.value = "processing";
    }),
    onBackendEvent<OCRResultEvent>("ocr-result", (payload) => {
      if (!isCurrentGeneration(payload)) {
        return;
      }
      ocrBlocks.value = payload.blocks ?? [];
    }),
    onBackendEvent<TranslationDirectionEvent>("translation-direction", (payload) => {
      if (!isCurrentGeneration(payload) || manualDirection.value) {
        return;
      }
      translationDirection.value = payload.direction === "to-en" ? "to-en" : "to-zh";
    }),
    onBackendEvent<GenerationEvent>("translation-start", (payload) => {
      if (!isCurrentGeneration(payload)) {
        return;
      }
      phase.value = "streaming";
    }),
    onBackendEvent<TranslationTokenEvent>("translation-token", (payload) => {
      if (!isCurrentGeneration(payload)) {
        return;
      }
      queueTranslationText(payload.token);
      phase.value = "streaming";
    }),
    onBackendEvent<GenerationEvent>("translation-done", (payload) => {
      if (!isCurrentGeneration(payload)) {
        return;
      }
      flushTranslationText();
      phase.value = "done";
      if (config.autoCopy && cleanTranslationText.value) {
        void copyText(cleanTranslationText.value);
        copied.value = true;
        window.setTimeout(() => {
          copied.value = false;
        }, 1100);
      }
    }),
    onBackendEvent<WorkflowErrorPayload>("workflow-error", (payload) => {
      if (!isCurrentGeneration(payload)) {
        return;
      }
      flushTranslationText();
      if (payload.stage === "translation" && translationText.value.trim()) {
        errorMessage.value = "";
        phase.value = "done";
        return;
      }
      errorMessage.value = payload.message;
      phase.value = "error";
    }),
    onBackendEvent("settings-open", async () => {
      invalidatePendingCaptureLoad();
      capture.value = null;
      resultRect.value = null;
      selection.value = null;
      ocrBlocks.value = [];
      resetTranslationText();
      errorMessage.value = "";
      copied.value = false;
      copiedImage.value = false;
      screenshotNotice.value = "";
      workflowGeneration.value += 1;
      phase.value = "idle";
      settingsOpen.value = true;
      await nextTick();
      await showSettingsWindow();
      void loadHistory();
      void loadEnvironmentStatus();
      void loadAutoStart();
      void loadVersion();
    })
  );

  await frontendReady();
  Object.assign(config, await loadConfig());
});

onBeforeUnmount(() => {
  invalidatePendingCaptureLoad();
  resetTranslationText();
  window.removeEventListener("resize", updateViewport);
  window.removeEventListener("keydown", onKeyDown);
  detachCaptureDragListeners();
  for (const unsub of unsubs) {
    unsub();
  }
});

function updateViewport(): void {
  viewport.width = window.innerWidth;
  viewport.height = window.innerHeight;
}

function isCurrentGeneration(payload: { generation?: number }): boolean {
  return typeof payload.generation !== "number" || payload.generation === workflowGeneration.value;
}

function onKeyDown(event: KeyboardEvent): void {
  if (event.key === "Escape" && settingsOpen.value) {
    event.preventDefault();
    void closeSettings();
    return;
  }
  if (event.key === "Escape" && isCaptureActive()) {
    event.preventDefault();
    void cancelCapture();
  }
}

async function startCapture(payload: CapturePayload): Promise<void> {
  const sequence = ++captureLoadSequence;
  settingsOpen.value = false;
  capture.value = payload;
  dragStart.value = null;
  selection.value = null;
  resultRect.value = null;
  ocrBlocks.value = [];
  resetTranslationText();
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  screenshotNotice.value = "";
  manualDirection.value = false;
  workflowGeneration.value += 1;
  phase.value = "loading";

  await nextTick();
  if (sequence !== captureLoadSequence) {
    return;
  }

  const drawn = await drawCapture(payload, sequence);
  if (!drawn || sequence !== captureLoadSequence) {
    return;
  }

  phase.value = "ready";
  await nextTick();
  if (sequence !== captureLoadSequence) {
    return;
  }
  await showCaptureWindow();
}

async function drawCapture(payload: CapturePayload, sequence: number): Promise<boolean> {
  const image = new Image();
  await new Promise<void>((resolve, reject) => {
    image.onload = () => resolve();
    image.onerror = () => reject(new Error("Failed to load captured image"));
    image.src = payload.image;
  });

  if (sequence !== captureLoadSequence) {
    return false;
  }

  const canvas = canvasRef.value;
  if (!canvas) {
    return false;
  }

  canvas.width = image.naturalWidth || payload.width;
  canvas.height = image.naturalHeight || payload.height;
  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("Canvas 2D context is unavailable");
  }

  context.clearRect(0, 0, canvas.width, canvas.height);
  context.drawImage(image, 0, 0, canvas.width, canvas.height);
  return true;
}

function invalidatePendingCaptureLoad(): void {
  captureLoadSequence += 1;
}

function pointerPosition(event: MouseEvent): Point {
  const target = captureLayerRef.value ?? (event.currentTarget as HTMLElement);
  const rect = target.getBoundingClientRect();
  return clampPointToBounds(
    {
      x: event.clientX - rect.left,
      y: event.clientY - rect.top
    },
    { width: rect.width, height: rect.height }
  );
}

function onMouseDown(event: MouseEvent): void {
  if (event.button !== 0 || phase.value !== "ready") {
    return;
  }

  attachCaptureDragListeners();
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

  detachCaptureDragListeners();
  const rect = normalizeRect(dragStart.value, pointerPosition(event));
  selection.value = rect;
  dragStart.value = null;

  if (!isUsableSelection(rect)) {
    selection.value = null;
    phase.value = "ready";
    return;
  }

  if (capture.value?.mode === "screenshot") {
    phase.value = "editing";
    return;
  }

  await submitSelection(rect);
}

function attachCaptureDragListeners(): void {
  window.addEventListener("mousemove", onMouseMove);
  window.addEventListener("mouseup", onMouseUp);
}

function detachCaptureDragListeners(): void {
  window.removeEventListener("mousemove", onMouseMove);
  window.removeEventListener("mouseup", onMouseUp);
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
    { width: canvas.width, height: canvas.height },
    payload.displays
  );

  resultRect.value = rect;
  selection.value = null;
  ocrBlocks.value = [];
  resetTranslationText();
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  phase.value = "processing";

  try {
		// Let Vue paint the OCR indicator before PNG compression. toBlob keeps
		// the potentially expensive encoding work off the interaction task.
		await nextTick();
		const crop = await cropCanvasToDataUrlAsync(canvas, imageRect);
    currentCropDataUrl.value = crop;
    await processImage(crop, directionForRequest(), workflowGeneration.value);
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
  return phase.value === "loading" || phase.value === "ready" || phase.value === "drawing" || phase.value === "editing";
}

async function cancelCapture(): Promise<void> {
  invalidatePendingCaptureLoad();
  detachCaptureDragListeners();
  dragStart.value = null;
  selection.value = null;
  capture.value = null;
  resultRect.value = null;
  ocrBlocks.value = [];
  resetTranslationText();
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  currentCropDataUrl.value = "";
  screenshotNotice.value = "";
  workflowGeneration.value += 1;
  phase.value = "idle";
  await hideWindow();
}

async function completeScreenshot(dataUrl: string): Promise<void> {
  screenshotNotice.value = "正在复制...";
  try {
    await copyImageDataUrl(dataUrl);
    await cancelCapture();
  } catch (error) {
    screenshotNotice.value = error instanceof Error ? error.message : "复制截图失败";
  }
}

async function saveEditedScreenshot(dataUrl: string): Promise<void> {
  screenshotNotice.value = "正在保存...";
  try {
    const path = await saveScreenshot(dataUrl);
    screenshotNotice.value = path ? "截图已保存" : "";
  } catch (error) {
    screenshotNotice.value = error instanceof Error ? error.message : "保存截图失败";
  }
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

async function copyOcrText(): Promise<void> {
  const text = ocrBlocks.value.map((block) => block.text).join("\n").trim();
  if (!text) {
    return;
  }
  await copyText(text);
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

  manualDirection.value = true;
  translationDirection.value = translationDirection.value === "to-zh" ? "to-en" : "to-zh";
  ocrBlocks.value = [];
  resetTranslationText();
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  screenshotNotice.value = "";
  phase.value = "streaming";
  workflowGeneration.value += 1;

  try {
    await processImage(currentCropDataUrl.value, directionForRequest(), workflowGeneration.value);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "Failed to reverse translation direction";
    phase.value = "error";
  }
}

async function retryProcessing(): Promise<void> {
  if (!currentCropDataUrl.value || isResultBusy.value) {
    return;
  }

  errorMessage.value = "";
  ocrBlocks.value = [];
  resetTranslationText();
  copied.value = false;
  copiedImage.value = false;
  phase.value = "processing";
  workflowGeneration.value += 1;

  try {
    await processImage(currentCropDataUrl.value, directionForRequest(), workflowGeneration.value);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "Failed to retry";
    phase.value = "error";
  }
}

function directionForRequest(): TranslationDirection {
  if (config.autoDirection && !manualDirection.value) {
    return "auto";
  }
  return translationDirection.value;
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
    { width: sourceCanvas.width, height: sourceCanvas.height },
    capture.value?.displays
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
    const text = translationForOCRBlock(index, block, parsed, ocrBlocks.value.length, resolvedTranslationDirection.value);
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
      resolvedTranslationDirection.value
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
    ? Math.max(8, Math.round(baseFontSize * 0.48))
    : Math.max(8, Math.round(baseFontSize * 0.6));
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
  resetTranslationText();
  currentCropDataUrl.value = "";
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  workflowGeneration.value += 1;
  await hideWindow();
}

async function closeSettings(): Promise<void> {
  settingsOpen.value = false;
  shortcutRecording.value = null;
  testStatus.value = "idle";
  testMessage.value = "";
  if (isDesktop) {
    await hideWindow();
  }
}

function openSettings(): void {
  settingsError.value = "";
  settingsOpen.value = true;
  void loadHistory();
  void loadEnvironmentStatus();
  void loadAutoStart();
  void loadVersion();
}

async function loadVersion(): Promise<void> {
  try {
    appVersion.value = await getVersion();
  } catch {
    appVersion.value = "";
  }
}

async function loadAutoStart(): Promise<void> {
  try {
    autoStartEnabled.value = await isAutoStartEnabled();
  } catch {
    autoStartEnabled.value = false;
  }
}

async function toggleAutoStart(): Promise<void> {
  autoStartTouched.value = true;
  const desiredState = autoStartEnabled.value;
  try {
    await setAutoStart(desiredState);
  } catch (error) {
    autoStartEnabled.value = !desiredState;
    settingsError.value = error instanceof Error ? error.message : "Failed to update autostart";
  } finally {
    autoStartTouched.value = false;
  }
}

async function loadEnvironmentStatus(): Promise<void> {
  try {
    envStatus.value = await getEnvironmentStatus();
  } catch {
    envStatus.value = null;
  }
}

async function loadHistory(): Promise<void> {
  try {
    const entries = await getHistory();
    historyEntries.value = Array.isArray(entries) ? entries : [];
  } catch {
    historyEntries.value = [];
  }
  historyLoaded.value = true;
}

async function clearAllHistory(): Promise<void> {
  try {
    await clearHistory();
    historyEntries.value = [];
  } catch {
    // ignore
  }
}

async function runConnectionTest(): Promise<void> {
  testStatus.value = "testing";
  testMessage.value = "";
  try {
    await testConnection();
    testStatus.value = "ok";
    testMessage.value = settingsText.value.connectionSuccessful;
  } catch (error) {
    testStatus.value = "error";
    testMessage.value = error instanceof Error ? error.message : settingsText.value.connectionFailed;
  }
}

function startShortcutRecording(kind: "translate" | "screenshot" = "translate"): void {
  shortcutRecording.value = kind;
}

function cancelShortcutRecording(): void {
  shortcutRecording.value = null;
}

function onShortcutRecorderKeydown(event: KeyboardEvent): void {
  if (!shortcutRecording.value) {
    return;
  }

  event.preventDefault();
  event.stopPropagation();

  if (event.key === "Escape") {
    cancelShortcutRecording();
    return;
  }

  const shortcut = shortcutKeyFromKeyboardEvent(event);
  if (shortcut) {
    if (shortcutRecording.value === "screenshot") {
      config.screenshotShortcutKey = shortcut;
    } else {
      config.shortcutKey = shortcut;
    }
    shortcutRecording.value = null;
  }
}

async function saveSettings(): Promise<void> {
  settingsError.value = "";
  settingsSaving.value = true;
  try {
    await saveConfig({ ...config });
    settingsSaving.value = false;
    await closeSettings();
  } catch (error) {
    settingsSaving.value = false;
    settingsError.value = error instanceof Error ? error.message : "Failed to save settings";
  }
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
      @click="openSettings"
    >
      <Settings class="h-5 w-5" aria-hidden="true" />
    </button>

    <section
      v-if="capture"
      ref="captureLayerRef"
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
        v-if="selection && phase !== 'editing'"
        class="pointer-events-none absolute border-2 border-emerald-300 bg-emerald-200/5 shadow-[0_0_0_9999px_rgba(2,6,23,0.18)] outline outline-1 outline-white/90"
        :style="selectionStyle"
      />
      <div
        v-if="selection && phase === 'drawing' && selection.width > 0 && selection.height > 0"
        class="pointer-events-none absolute z-20 rounded-md bg-slate-950/80 px-2 py-1 font-mono text-xs font-medium tabular-nums text-white shadow-lg backdrop-blur-sm"
        :style="selectionBadgeStyle"
        data-testid="selection-size"
      >
        {{ Math.round(selection.width) }} × {{ Math.round(selection.height) }}
      </div>
    </section>

    <ScreenshotEditor
      v-if="phase === 'editing' && capture && selection && canvasRef"
      :source-canvas="canvasRef"
      :capture="capture"
      :rect="selection"
      @cancel="cancelCapture"
      @complete="completeScreenshot"
      @save="saveEditedScreenshot"
    />

    <div
      v-if="phase === 'editing' && screenshotNotice"
      class="pointer-events-none absolute left-1/2 top-5 z-[70] -translate-x-1/2 rounded-lg bg-slate-950/88 px-3 py-2 text-sm font-medium text-white shadow-lg backdrop-blur"
      data-testid="screenshot-notice"
    >
      {{ screenshotNotice }}
    </div>

    <section
      v-if="resultRect"
      ref="resultPanelRef"
      data-testid="result-panel"
      class="absolute z-20 overflow-visible transition-[height,top,box-shadow,background-color] duration-100 ease-out"
      :class="
        preservesSourcePreview
          ? 'rounded-md border-2 border-emerald-400 shadow-[0_0_0_1px_rgba(255,255,255,0.70),0_8px_28px_rgba(16,185,129,0.18)]'
          : 'rounded-md border border-white/70 bg-white/92 p-2 shadow-[0_10px_36px_rgba(15,23,42,0.26)] ring-1 ring-slate-900/5 backdrop-blur-[2px] dark:border-slate-700/70 dark:bg-zinc-950/92'
      "
      :style="resultStyle"
    >
      <div
        v-if="showsResultProgress"
        data-testid="translation-progress"
        class="pointer-events-none absolute inset-0 z-30 flex items-center justify-center"
      >
        <div
          data-testid="translation-progress-card"
          class="flex min-h-24 min-w-32 flex-col items-center justify-center rounded-xl bg-slate-950/85 px-5 py-4 text-white shadow-[0_12px_36px_rgba(15,23,42,0.34)] backdrop-blur-sm dark:bg-zinc-100/90 dark:text-zinc-950"
        >
          <span class="h-8 w-8 animate-spin rounded-full border-[3px] border-emerald-400 border-t-transparent" />
          <span class="mt-3 text-sm font-medium">{{ resultProgressLabel }}</span>
        </div>
      </div>

      <template v-if="hasFlowTranslationLayout && phase !== 'error'">
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

      <template v-else-if="hasOCRBlockLayout && phase !== 'error'">
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
        <div
          v-if="phase === 'error'"
          data-testid="result-error"
          class="flex h-full flex-col gap-2 overflow-y-auto overflow-x-hidden break-words whitespace-pre-wrap text-sm leading-6 text-rose-700 dark:text-rose-300"
        >
          <div class="min-h-0 flex-1 [overflow-wrap:anywhere]">{{ errorMessage }}</div>
          <div class="flex gap-2">
            <button
              class="inline-flex h-8 items-center gap-1.5 rounded-md border border-slate-300 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:bg-zinc-900 dark:text-slate-200 dark:hover:bg-zinc-800"
              type="button"
              title="Retry translation"
              aria-label="Retry translation"
              :disabled="!currentCropDataUrl"
              @click="retryProcessing"
            >
              <RefreshCw class="h-3.5 w-3.5" aria-hidden="true" />
              Retry
            </button>
            <button
              class="inline-flex h-8 items-center gap-1.5 rounded-md border border-slate-300 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:bg-zinc-900 dark:text-slate-200 dark:hover:bg-zinc-800"
              type="button"
              title="Copy original OCR text"
              aria-label="Copy OCR text"
              :disabled="!ocrBlocks.length"
              @click="copyOcrText"
            >
              <Copy class="h-3.5 w-3.5" aria-hidden="true" />
              Copy OCR
            </button>
          </div>
        </div>

        <div
          v-else
          data-testid="translation-result"
          class="markdown-body h-full overflow-hidden break-words pr-1 text-slate-950 dark:text-slate-100"
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
      data-testid="settings-shell"
      class="absolute inset-0 z-40 overflow-hidden bg-[radial-gradient(circle_at_90%_0%,rgba(16,185,129,0.08),transparent_32%),linear-gradient(180deg,#f8fafc,#f1f5f9)] text-slate-900 dark:bg-[radial-gradient(circle_at_90%_0%,rgba(16,185,129,0.10),transparent_32%),linear-gradient(180deg,#09090b,#18181b)] dark:text-slate-100"
    >
      <form
        class="flex h-full w-full flex-col overflow-hidden"
        @submit.prevent="saveSettings"
        @keydown="onShortcutRecorderKeydown"
      >
        <header
          data-testid="settings-drag-region"
          class="flex shrink-0 cursor-move select-none items-center justify-between border-b border-slate-200/90 bg-white/95 px-6 py-4 shadow-[0_1px_0_rgba(15,23,42,0.02)] backdrop-blur-xl dark:border-zinc-800 dark:bg-zinc-950/95"
          style="--wails-draggable: drag"
        >
          <div class="flex min-w-0 items-center gap-3">
            <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-[0_6px_18px_rgba(5,150,105,0.24)]">
              <Settings class="h-4 w-4" aria-hidden="true" />
            </div>
            <div class="min-w-0">
              <div class="flex items-baseline gap-2">
                <h1 class="text-base font-semibold tracking-tight">{{ settingsText.title }}</h1>
                <span v-if="appVersion" class="text-[11px] font-medium text-slate-400">v{{ appVersion }}</span>
              </div>
              <p class="truncate text-xs text-slate-500 dark:text-slate-400">{{ settingsText.subtitle }}</p>
            </div>
          </div>
          <div
            class="flex items-center gap-2"
            style="--wails-draggable: no-drag"
          >
            <div class="flex h-8 items-center rounded-lg bg-slate-100 p-0.5 ring-1 ring-slate-200/80 dark:bg-zinc-900 dark:ring-zinc-800">
              <button
                data-testid="locale-zh"
                class="h-7 rounded-md px-2.5 text-[11px] font-semibold transition"
                :class="settingsLocale === 'zh-CN' ? 'bg-white text-emerald-700 shadow-sm dark:bg-zinc-800 dark:text-emerald-300' : 'text-slate-500 hover:text-slate-800 dark:text-slate-400 dark:hover:text-slate-100'"
                style="--wails-draggable: no-drag"
                type="button"
                @click="setSettingsLocale('zh-CN')"
              >
                {{ settingsText.chinese }}
              </button>
              <button
                data-testid="locale-en"
                class="h-7 rounded-md px-2.5 text-[11px] font-semibold transition"
                :class="settingsLocale === 'en' ? 'bg-white text-emerald-700 shadow-sm dark:bg-zinc-800 dark:text-emerald-300' : 'text-slate-500 hover:text-slate-800 dark:text-slate-400 dark:hover:text-slate-100'"
                style="--wails-draggable: no-drag"
                type="button"
                @click="setSettingsLocale('en')"
              >
                {{ settingsText.english }}
              </button>
            </div>
            <button
              class="inline-flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 transition hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-zinc-800 dark:hover:text-white"
              style="--wails-draggable: no-drag"
              type="button"
              :title="settingsText.close"
              aria-label="Close"
              @click="closeSettings"
            >
              <X class="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </header>

        <div data-testid="settings-scroll" class="settings-scrollbar min-h-0 flex-1 overflow-y-auto px-6 py-5">
          <div class="mx-auto max-w-3xl space-y-4">

            <section data-testid="settings-section" class="settings-card">
              <div class="settings-section-header">
                <div class="settings-section-icon bg-violet-50 text-violet-600 dark:bg-violet-950 dark:text-violet-300">
                  <Bot class="h-4 w-4" aria-hidden="true" />
                </div>
                <div>
                  <h2 class="settings-section-title">{{ settingsText.aiService }}</h2>
                  <p class="settings-section-description">{{ settingsText.aiServiceDescription }}</p>
                </div>
              </div>

        <div v-if="envStatus" class="mb-4 grid gap-2 sm:grid-cols-2">
          <div
            class="settings-status"
            :class="envStatus.ocrReady
              ? 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
              : 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-300'"
          >
            <Check v-if="envStatus.ocrReady" class="h-3.5 w-3.5" aria-hidden="true" />
            <X v-else class="h-3.5 w-3.5" aria-hidden="true" />
            <span class="truncate">
              {{ settingsText.ocr }}: {{ envStatus.ocrReady ? settingsText.ready : settingsText.notFound }}
              <span v-if="!envStatus.ocrReady" class="opacity-80">- {{ envStatus.ocrDetail }}</span>
            </span>
          </div>
          <div
            class="settings-status"
            :class="envStatus.apiKeyConfigured
              ? 'border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
              : 'border-amber-300 bg-amber-50 text-amber-700 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-300'"
          >
            <Check v-if="envStatus.apiKeyConfigured" class="h-3.5 w-3.5" aria-hidden="true" />
            <X v-else class="h-3.5 w-3.5" aria-hidden="true" />
            <span>
              {{ settingsText.apiKey }}: {{ envStatus.apiKeyConfigured ? settingsText.configured : settingsText.missingAPIKey }}
            </span>
          </div>
        </div>

        <label class="settings-field">
          <span class="settings-label">{{ settingsText.apiKey }}</span>
          <span class="relative block">
            <input
              v-model="config.apiKey"
              class="settings-input pr-11"
              :type="showAPIKey ? 'text' : 'password'"
              autocomplete="off"
              :placeholder="settingsText.apiKeyPlaceholder"
            />
            <button
              class="absolute inset-y-0 right-1 inline-flex w-9 items-center justify-center rounded-md text-slate-400 transition hover:text-slate-700 dark:hover:text-slate-200"
              type="button"
              :title="showAPIKey ? settingsText.hideAPIKey : settingsText.showAPIKey"
              :aria-label="showAPIKey ? settingsText.hideAPIKey : settingsText.showAPIKey"
              @click="showAPIKey = !showAPIKey"
            >
              <EyeOff v-if="showAPIKey" class="h-4 w-4" aria-hidden="true" />
              <Eye v-else class="h-4 w-4" aria-hidden="true" />
            </button>
          </span>
        </label>

        <div class="mt-4 grid gap-4 sm:grid-cols-2">
        <label class="settings-field">
          <span class="settings-label">{{ settingsText.baseURL }}</span>
          <input
            v-model="config.baseURL"
            class="settings-input"
            type="url"
            placeholder="https://your-litellm-host/v1"
          />
          <span class="settings-help">{{ settingsText.baseURLHelp }}</span>
        </label>

        <label class="settings-field">
          <span class="settings-label">{{ settingsText.model }}</span>
          <input
            v-model="config.model"
            class="settings-input"
            type="text"
            placeholder="gemini/gemini-3.5-flash-lite"
          />
          <span class="settings-help">{{ settingsText.modelHelp }}</span>
        </label>
        </div>

        <div class="mt-4 flex min-h-9 flex-wrap items-center gap-3 border-t border-slate-100 pt-4 dark:border-zinc-800">
          <button
            class="settings-secondary-button"
            type="button"
            title="Test the API key, base URL, and model"
            :disabled="testStatus === 'testing'"
            @click="runConnectionTest"
          >
            <span v-if="testStatus === 'testing'" class="h-3.5 w-3.5 animate-spin rounded-full border-2 border-emerald-500 border-t-transparent" />
            <Sparkles v-else class="h-3.5 w-3.5" aria-hidden="true" />
            {{ testStatus === "testing" ? settingsText.testing : settingsText.testConnection }}
          </button>
          <span
            v-if="testStatus !== 'idle'"
            class="min-w-0 flex-1 truncate text-xs font-medium"
            :class="testStatus === 'ok' ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'"
          >
            {{ testMessage }}
          </span>
        </div>
            </section>

            <section data-testid="settings-section" class="settings-card">
              <div class="settings-section-header">
                <div class="settings-section-icon bg-blue-50 text-blue-600 dark:bg-blue-950 dark:text-blue-300">
                  <Cpu class="h-4 w-4" aria-hidden="true" />
                </div>
                <div>
                  <h2 class="settings-section-title">{{ settingsText.captureOCR }}</h2>
                  <p class="settings-section-description">{{ settingsText.captureOCRDescription }}</p>
                </div>
              </div>

        <div class="grid gap-4 sm:grid-cols-2">
        <label class="settings-field">
          <span class="settings-label">{{ settingsText.captureShortcut }}</span>
          <div class="flex gap-2">
            <span class="relative min-w-0 flex-1">
              <Keyboard class="pointer-events-none absolute left-3 top-3 h-4 w-4 text-slate-400" aria-hidden="true" />
              <input
                v-model="config.shortcutKey"
                class="settings-input pl-9"
                type="text"
                :placeholder="shortcutRecording === 'translate' ? settingsText.pressShortcut : 'Alt+Q'"
                :readonly="shortcutRecording === 'translate'"
              />
            </span>
            <button
              class="settings-secondary-button h-10 shrink-0"
              type="button"
              :title="shortcutRecording === 'translate' ? 'Press a key combination or Esc to cancel' : 'Record shortcut'"
              @click="startShortcutRecording('translate')"
            >
              {{ shortcutRecording === "translate" ? settingsText.recording : settingsText.record }}
            </button>
          </div>
          <span class="settings-help" :class="shortcutRecording === 'translate' ? 'text-emerald-600 dark:text-emerald-400' : ''">
            {{ shortcutRecording === "translate" ? settingsText.pressShortcut : settingsText.shortcutHelp }}
          </span>
        </label>

        <label class="settings-field">
          <span class="settings-label">{{ settingsText.screenshotShortcut }}</span>
          <div class="flex gap-2">
            <span class="relative min-w-0 flex-1">
              <Keyboard class="pointer-events-none absolute left-3 top-3 h-4 w-4 text-slate-400" aria-hidden="true" />
              <input
                v-model="config.screenshotShortcutKey"
                class="settings-input pl-9"
                type="text"
                :placeholder="shortcutRecording === 'screenshot' ? settingsText.pressShortcut : 'Alt+W'"
                :readonly="shortcutRecording === 'screenshot'"
              />
            </span>
            <button
              class="settings-secondary-button h-10 shrink-0"
              type="button"
              :title="shortcutRecording === 'screenshot' ? 'Press a key combination or Esc to cancel' : 'Record shortcut'"
              @click="startShortcutRecording('screenshot')"
            >
              {{ shortcutRecording === "screenshot" ? settingsText.recording : settingsText.record }}
            </button>
          </div>
          <span class="settings-help" :class="shortcutRecording === 'screenshot' ? 'text-emerald-600 dark:text-emerald-400' : ''">
            {{ shortcutRecording === "screenshot" ? settingsText.pressShortcut : settingsText.screenshotShortcutHelp }}
          </span>
        </label>

        <label class="settings-field">
          <span class="settings-label">{{ settingsText.rapidOCRPath }}</span>
          <input
            v-model="config.rapidOCRPath"
            class="settings-input"
            type="text"
            placeholder="RapidOCR-json_v0.2.0"
          />
          <span class="settings-help">{{ settingsText.rapidOCRPathHelp }}</span>
        </label>
        </div>

        <div class="mt-4 grid gap-2 sm:grid-cols-2">
        <label class="settings-toggle">
          <span class="min-w-0 pr-3">
            <span class="settings-toggle-title">{{ settingsText.startWithWindows }}</span>
            <span class="settings-toggle-description">{{ settingsText.startWithWindowsDescription }}</span>
          </span>
          <input
            v-model="autoStartEnabled"
            class="peer sr-only"
            type="checkbox"
            :disabled="autoStartTouched"
            @change="toggleAutoStart"
          />
          <span class="settings-switch" aria-hidden="true" />
        </label>

        <label class="settings-toggle">
          <span class="min-w-0 pr-3">
            <span class="settings-toggle-title">{{ settingsText.keepOCRReady }}</span>
            <span class="settings-toggle-description">{{ settingsText.keepOCRReadyDescription }}</span>
          </span>
          <input
            v-model="config.persistentOCR"
            class="peer sr-only"
            type="checkbox"
          />
          <span class="settings-switch" aria-hidden="true" />
        </label>
        </div>
            </section>

            <section data-testid="settings-section" class="settings-card">
              <div class="settings-section-header">
                <div class="settings-section-icon bg-emerald-50 text-emerald-600 dark:bg-emerald-950 dark:text-emerald-300">
                  <Languages class="h-4 w-4" aria-hidden="true" />
                </div>
                <div>
                  <h2 class="settings-section-title">{{ settingsText.translation }}</h2>
                  <p class="settings-section-description">{{ settingsText.translationDescription }}</p>
                </div>
              </div>

        <div class="grid gap-2 sm:grid-cols-2">
        <label class="settings-toggle">
          <span class="min-w-0 pr-3">
            <span class="settings-toggle-title">{{ settingsText.detectDirection }}</span>
            <span class="settings-toggle-description">{{ settingsText.detectDirectionDescription }}</span>
          </span>
          <input v-model="config.autoDirection" class="peer sr-only" type="checkbox" />
          <span class="settings-switch" aria-hidden="true" />
        </label>

        <label class="settings-toggle">
          <span class="min-w-0 pr-3">
            <span class="settings-toggle-title">{{ settingsText.copyAutomatically }}</span>
            <span class="settings-toggle-description">{{ settingsText.copyAutomaticallyDescription }}</span>
          </span>
          <input
            v-model="config.autoCopy"
            class="peer sr-only"
            type="checkbox"
          />
          <span class="settings-switch" aria-hidden="true" />
        </label>
        </div>

        <div class="mt-4 grid gap-4 sm:grid-cols-2">
        <label class="settings-field">
          <span class="settings-label">{{ settingsText.customInstructions }}</span>
          <textarea
            v-model="config.systemPrompt"
            class="settings-textarea"
            :placeholder="settingsText.customInstructionsPlaceholder"
          />
        </label>

        <label class="settings-field">
          <span class="settings-label">{{ settingsText.glossary }}</span>
          <textarea
            v-model="config.glossary"
            class="settings-textarea"
            :placeholder="settingsText.glossaryPlaceholder"
          />
          <span class="settings-help">{{ settingsText.glossaryHelp }}</span>
        </label>
        </div>
            </section>

            <section data-testid="settings-section" class="settings-card">
        <div class="settings-section-header mb-3">
          <div class="settings-section-icon bg-amber-50 text-amber-600 dark:bg-amber-950 dark:text-amber-300">
            <History class="h-4 w-4" aria-hidden="true" />
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="settings-section-title">{{ settingsText.recentTranslations }}</h2>
            <p class="settings-section-description">{{ settingsText.recentTranslationsDescription }}</p>
          </div>
          <button
            class="settings-tertiary-button shrink-0"
            type="button"
            :title="settingsText.clearHistory"
            aria-label="Clear history"
            :disabled="historyEntries.length === 0"
            @click="clearAllHistory"
          >
            {{ settingsText.clear }}
          </button>
        </div>
        <div v-if="historyEntries.length === 0" class="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-4 py-5 text-center text-xs text-slate-500 dark:border-zinc-800 dark:bg-zinc-900/50 dark:text-slate-400">
          {{ historyLoaded ? settingsText.noRecentTranslations : settingsText.loadingHistory }}
        </div>
        <div v-else class="max-h-44 divide-y divide-slate-100 overflow-y-auto rounded-lg border border-slate-200 dark:divide-zinc-800 dark:border-zinc-800">
          <div
            v-for="entry in historyEntries"
            :key="entry.id"
            class="group flex items-center gap-3 px-3 py-2.5 transition hover:bg-slate-50 dark:hover:bg-zinc-900"
          >
            <div class="min-w-0 flex-1 text-xs">
              <div class="truncate text-slate-400 dark:text-slate-500">{{ entry.source }}</div>
              <div class="mt-0.5 truncate font-medium text-slate-700 dark:text-slate-200">{{ entry.translation }}</div>
            </div>
            <button
              class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-slate-400 transition hover:bg-white hover:text-slate-700 hover:shadow-sm dark:hover:bg-zinc-800 dark:hover:text-slate-100"
              type="button"
              :title="settingsText.copyHistoryEntry"
              aria-label="Copy history entry"
              @click="copyText(entry.translation)"
            >
              <Copy class="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          </div>
        </div>
            </section>
          </div>
        </div>

        <footer
          data-testid="settings-footer"
          class="shrink-0 border-t border-slate-200 bg-white px-6 py-3.5 dark:border-zinc-800 dark:bg-zinc-950"
        >
          <div v-if="settingsError" class="mx-auto mb-3 max-w-3xl rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-300">
            {{ settingsError }}
          </div>
        <div class="mx-auto flex max-w-3xl items-center justify-between gap-3">
          <button
            v-if="isDesktop"
            class="settings-tertiary-button"
            type="button"
            title="Open the local log folder"
            aria-label="Open log folder"
            @click="openLogFolder"
          >
            <FolderOpen class="h-3.5 w-3.5" aria-hidden="true" />
            {{ settingsText.openLogs }}
          </button>
          <span v-else class="text-[11px] text-slate-400">{{ settingsText.unsavedHint }}</span>
          <div class="flex items-center gap-2">
            <button
              class="settings-secondary-button h-9"
              type="button"
              @click="closeSettings"
            >
              {{ settingsText.cancel }}
            </button>
            <button
              class="inline-flex h-9 min-w-24 items-center justify-center rounded-lg bg-emerald-600 px-4 text-sm font-semibold text-white shadow-sm transition hover:bg-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500/30 disabled:cursor-not-allowed disabled:opacity-60"
              type="submit"
              :disabled="settingsSaving"
            >
              {{ settingsSaving ? settingsText.saving : settingsText.saveChanges }}
            </button>
          </div>
        </div>
        </footer>
      </form>
    </section>
  </main>
</template>
