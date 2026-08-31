<script setup lang="ts">
import {
  Bot,
  Camera,
  Check,
  ChevronDown,
  Copy,
  Cpu,
  FolderOpen,
  History,
  Image as ImageIcon,
  Keyboard,
  Languages,
  Minus,
  Moon,
  RefreshCw,
  Settings,
  Sparkles,
  Sun,
  Star,
  X
} from "lucide-vue-next";
import MarkdownIt from "markdown-it";
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import SettingsNavigation from "./components/SettingsNavigation.vue";
import { settingsSections, type SettingsSection } from "./utils/settings-navigation";
import ExtensionSettings from "./components/ExtensionSettings.vue";
import ExtensionWorkbench from "./components/ExtensionWorkbench.vue";
import LearningLibrary from "./components/LearningLibrary.vue";
import { normalizeFeatures } from "./utils/features";
import { clearComparisonBaseline, filterLibrary, libraryMarkdown } from "./utils/extensions";
import ScreenshotEditor from "./components/ScreenshotEditor.vue";
import {
  clearHistory,
  setHistoryFavorite,
  exportMarkdown,
  triggerScreenshot,
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
  minimizeWindow,
  onBackendEvent,
  openLogFolder,
  processImage,
  saveConfig,
  saveScreenshot,
  showCaptureWindow,
  showSettingsWindow,
  testConnection,
  translateRegion,
  translateSelection,
  triggerCapture,
  type AppConfig,
  type CapturePayload,
  type EnvironmentStatus,
  type GenerationEvent,
  type HistoryEntry,
  type TextBlockPayload,
  type TextRegionsEvent,
  type OCRResultEvent,
  type SelectionRegion,
  type TranslationDirection,
  type TranslationDirectionEvent,
  type TranslationTokenEvent,
  type WorkflowErrorPayload
} from "./services/backend";
import {
  clampPointToBounds,
  fitTranslationFontSize,
  isUsableSelection,
  mapCssRectToImageRect,
  normalizeResultBox,
  normalizeRect,
  sampleCanvasColor,
  sampleCanvasForegroundColor,
  selectionBadgePosition,
  translationPaletteForColor,
  wrapTranslationText,
  type Point,
  type Rect,
  type SampledColor
} from "./utils/selection";
import {
  parseTranslationOutput
} from "./utils/translation";
import { buildSourceRegions, layoutTranslations, OVERLAY_FONT_FAMILY } from "./utils/overlay-layout";
import { canvasTextMeasurer, leadingMarkerInset, paintTranslationOverlay } from "./utils/overlay-painter";
import { nativeSelectionBackdrop } from "./utils/selection-backdrop";
import { shortcutKeyFromKeyboardEvent } from "./utils/shortcut";
import {
  normalizeSettingsLocale,
  settingsMessages,
  type SettingsLocale
} from "./i18n/settings";

import { bilingualHistoryText, historyTimestamp, readableTranslation } from "./utils/history";

type Phase = "idle" | "loading" | "ready" | "drawing" | "editing" | "processing" | "streaming" | "done" | "partial" | "error";
const canvasRef = ref<HTMLCanvasElement | null>(null);
const captureLayerRef = ref<HTMLElement | null>(null);
const resultPanelRef = ref<HTMLElement | null>(null);
const phase = ref<Phase>("idle");
const capture = ref<CapturePayload | null>(null);
const dragStart = ref<Point | null>(null);
const selection = ref<Rect | null>(null);
const resultRect = ref<Rect | null>(null);
const textBlocks = ref<TextBlockPayload[]>([]);
const viewport = reactive({ width: window.innerWidth, height: window.innerHeight });
const translationText = ref("");
const translationDirection = ref<TranslationDirection>("to-zh");
const manualDirection = ref(false);
const resolvedTranslationDirection = computed<"to-zh" | "to-en">(() =>
  translationDirection.value === "to-en" ? "to-en" : "to-zh"
);
const currentCropDataUrl = ref("");
const lastImageRegion = ref<SelectionRegion | null>(null);
const hasResultSource = computed(() => Boolean(capture.value?.selectedText || lastImageRegion.value || currentCropDataUrl.value));
const workflowGeneration = ref(0);
let captureLoadSequence = 0;
const errorMessage = ref("");
const settingsOpen = ref(false);
const settingsSection = ref<SettingsSection>("ai");
const settingsScrollRef = ref<HTMLElement | null>(null);
function selectSettingsSection(section: SettingsSection): void {
  settingsSection.value = section;
  shortcutRecording.value = null;
  if (settingsScrollRef.value) settingsScrollRef.value.scrollTop = 0;
}
async function submitSettings(event: Event): Promise<void> {
  const form = event.currentTarget as HTMLFormElement;
  const invalid = form.querySelector<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(":invalid:not(fieldset)");
  if (invalid) {
    const panelID = invalid.closest('[role="tabpanel"]')?.id;
    const section = settingsSections.find(item => `settings-panel-${item.id}` === panelID);
    if (section) selectSettingsSection(section.id);
    await nextTick();
    invalid.focus();
    invalid.reportValidity();
    return;
  }
  await saveSettings();
}
const settingsError = ref("");
const settingsSaving = ref(false);
const sourceText = ref("");
const extensionOpen = ref(false);
const extensionImage = ref("");
const extensionTranslatedImage = ref("");
const extensionOrigin = ref({x:0,y:0});
const historyQuery = ref("");
const favoritesOnly = ref(false);
const favoritePending = ref("");
const historyEntries = ref<HistoryEntry[]>([]);
const historyLoaded = ref(false);
const testStatus = ref<"idle" | "testing" | "ok" | "error">("idle");
const testMessage = ref("");
const shortcutRecording = ref<"translate" | "screenshot" | null>(null);
const screenshotNotice = ref("");
const envStatus = ref<EnvironmentStatus | null>(null);
const autoStartEnabled = ref(false);
const autoStartLoading = ref(false);
const savedAutoStart = ref(false);
let connectionTestSequence = 0;
const copiedHistoryID = ref("");
const appVersion = ref("");
const copied = ref(false);
const copiedImage = ref(false);
const copyingGeneration = ref<number | null>(null);
const expandedTranslation = ref(false);
const isDesktop = hasWailsBackend();
const markdown = new MarkdownIt({ breaks: true, linkify: false, html: false });
const config = reactive<AppConfig>({ ...defaultConfig });
const savedConfig = reactive<AppConfig>({ ...defaultConfig });
const featureFlags = computed(() => normalizeFeatures(savedConfig.features));
const hasResultTools = computed(() => Object.entries(featureFlags.value).some(([key,value]) => value && !["historyTools","redaction","textExtraction"].includes(key)));
const visibleHistory = computed(() => filterLibrary(historyEntries.value.filter(e=>e.kind!=="learning"),featureFlags.value.historyTools?historyQuery.value:"",featureFlags.value.historyTools&&favoritesOnly.value?"favorites":"all"));
watch(() => featureFlags.value.imageCompare, enabled => {if(!enabled)clearComparisonBaseline();});
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

function toggleTheme(): void {
  config.theme = config.theme === "dark" ? "light" : "dark";
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

  if (textBlocks.value.length > 0 && phase.value !== "error") {
    return { left: rect.x+"px", top:rect.y+"px", width:rect.width+"px", height:rect.height+"px" };
  }
  const box = normalizeResultBox(rect, viewport);
  let height = box.height;

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
  const horizontal = box.x + 260 > viewport.width - 8 ? { right: "0px" } : { left: "0px" };

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

// Pixel colors do not change while text streams over the frozen capture.
const sampledPaletteCache = new Map<string, { palette: ReturnType<typeof translationPaletteForColor>; foregroundColor: SampledColor | null }>();
function sampleFrozenPalette(rect: Rect) {
  const canvas = canvasRef.value;
  const key = [captureLoadSequence, canvas?.width, canvas?.height, viewport.width, viewport.height, rect.x, rect.y, rect.width, rect.height].join(":");
  const cached = sampledPaletteCache.get(key);
  if (cached) return cached;
  const result = {
    palette: translationPaletteForColor(sampleCanvasColor(canvas, rect)),
    foregroundColor: sampleCanvasForegroundColor(canvas, rect)
  };
  if (canvas) {
    if (sampledPaletteCache.size >= 256) sampledPaletteCache.clear();
    sampledPaletteCache.set(key, result);
  }
  return result;
}

let measureOverlayText: ReturnType<typeof canvasTextMeasurer> | undefined;
function textMeasurer() {
  if (!measureOverlayText) {
    const context = document.createElement("canvas").getContext("2d");
    if (context && typeof context.measureText === "function") measureOverlayText = canvasTextMeasurer(context);
  }
  return measureOverlayText;
}
const sourceRegions = computed(() => {
  const rect = resultRect.value;
  if (!rect) return [];
  const insets = capture.value?.selectedText ? [] : textBlocks.value.map(block => leadingMarkerInset(canvasRef.value, {
    x: rect.x + block.x * rect.width, y: rect.y + block.y * rect.height,
    width: block.width * rect.width, height: block.height * rect.height
  }));
  return buildSourceRegions(textBlocks.value, rect, insets, Boolean(capture.value?.selectedText));
});
const overlayLayout = computed(() => layoutTranslations(sourceRegions.value, textBlocks.value, parsedTranslation.value, resolvedTranslationDirection.value, textMeasurer()));
const nativeBackdrops = computed(() => {
  const rect=resultRect.value;
  if(!rect || !capture.value?.selectedText) return [];
  return textBlocks.value.flatMap((block,index)=>{
    const mask=nativeSelectionBackdrop(canvasRef.value,{x:rect.x+block.x*rect.width,y:rect.y+block.y*rect.height,width:block.width*rect.width,height:block.height*rect.height},block.background);
    return mask?[{index,background:block.background!,rect:{...mask,x:mask.x-rect.x,y:mask.y-rect.y}}]:[];
  });
});
const visibleNativeBackdrops = computed(() => {
 const indices=new Set(overlayLayout.value.blocks.flatMap(block=>block.indices));
 return nativeBackdrops.value.filter(mask=>indices.has(mask.index));
});
const styledOverlayBlocks = computed(() => {
  const rect = resultRect.value;
  if (!rect) return [];
  return overlayLayout.value.blocks.map(block => {
    const { palette, foregroundColor } = sampleFrozenPalette({
      x: rect.x + block.bounds.x, y: rect.y + block.bounds.y,
      width: block.bounds.width, height: block.bounds.height
    });
    const nativeStyle = capture.value?.selectedText ? textBlocks.value[block.indices[0]!] : undefined;
    const validColor = (value?: string) => value && /^#[0-9a-f]{6}$/i.test(value) ? value : undefined;
    return { ...block, palette: { ...palette, backgroundColor: validColor(nativeStyle?.background) ?? palette.backgroundColor, textShadow: "none" }, foreground: validColor(nativeStyle?.foreground) ?? cssColorForSampledColor(foregroundColor) ?? palette.color };
  });
});
const visibleOverlayBlocks = computed(() => styledOverlayBlocks.value.map(block => ({
  ...block,
  style: {
    left: block.bounds.x + "px", top: block.bounds.y + "px",
    width: block.bounds.width + "px", height: block.height + "px",
    fontFamily: OVERLAY_FONT_FAMILY, fontSize: block.fontSize + "px", fontWeight: "400",
    lineHeight: block.lineHeight + "px", textAlign: block.kind === "label" ? "center" as const : "left" as const,
    display: "flex", flexDirection: "column" as const, justifyContent: block.kind === "label" ? "center" : "flex-start",
    whiteSpace: "pre" as const, overflow: "hidden", boxSizing: "border-box" as const,
    paddingLeft: block.kind === "prose" ? "1px" : "0", paddingRight: block.kind === "prose" ? "1px" : "0",
    ...block.palette, color: block.foreground
  }
})));
const hasTextBlockLayout = computed(() => textBlocks.value.length > 0);

const hasTranslatedOverlay = computed(
  () => hasTextBlockLayout.value
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
    onBackendEvent<TextRegionsEvent>("text-regions", payload => {
      if (!isCurrentGeneration(payload)) return;
      textBlocks.value = payload.blocks ?? [];
      sourceText.value = payload.text ?? "";
    }),
    onBackendEvent<OCRResultEvent>("ocr-result", (payload) => {
      if (!isCurrentGeneration(payload)) {
        return;
      }
      textBlocks.value = payload.blocks ?? [];
      sourceText.value = payload.text ?? "";
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
      if (savedConfig.autoCopy && cleanTranslationText.value) void copyResult(false);
    }),
    onBackendEvent<WorkflowErrorPayload>("workflow-error", (payload) => {
      if (!isCurrentGeneration(payload)) {
        return;
      }
      flushTranslationText();
      if (payload.stage === "translation" && translationText.value.trim()) {
        errorMessage.value = payload.message;
        phase.value = "partial";
        return;
      }
      errorMessage.value = payload.message;
      phase.value = "error";
    }),
    onBackendEvent("settings-open", async () => {
      extensionOpen.value = false;
      sourceText.value = "";
      invalidatePendingCaptureLoad();
      capture.value = null;
      resultRect.value = null;
      selection.value = null;
      textBlocks.value = [];
      resetTranslationText();
      errorMessage.value = "";
      copied.value = false;
      copiedImage.value = false;
  expandedTranslation.value = false;
      screenshotNotice.value = "";
      lastImageRegion.value = null;
      currentCropDataUrl.value = "";
      workflowGeneration.value += 1;
      phase.value = "idle";
      const wasOpen = settingsOpen.value;
      settingsOpen.value = true;
      await nextTick();
      await showSettingsWindow();
      void loadHistory();
      void loadEnvironmentStatus();
      if (!wasOpen) void loadAutoStart();
      void loadVersion();
    })
  );

  await frontendReady();
  Object.assign(savedConfig, await loadConfig());
  Object.assign(config, savedConfig);
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
  if (capture.value?.selectedText && resultRect.value) {
    void nextTick(() => { const rect = nativeSelectionRect(); if (rect && resultRect.value) resultRect.value = rect; });
  }
}

function isCurrentGeneration(payload: { generation?: number }): boolean {
  return typeof payload.generation !== "number" || payload.generation === workflowGeneration.value;
}

function onKeyDown(event: KeyboardEvent): void {
  if (document.querySelector("[data-extension-workbench]")) {
    if(event.key === "Escape") {event.preventDefault(); extensionOpen.value=false;}
    return;
  }
  if (event.key === "Escape" && settingsOpen.value) {
    event.preventDefault();
    void closeSettings();
    return;
  }
  if (event.key === "Escape" && (isCaptureActive() || resultRect.value)) {
    event.preventDefault();
    void cancelCapture();
  }
}

async function startCapture(payload: CapturePayload): Promise<void> {
  extensionOpen.value=false;sourceText.value="";
  const sequence = ++captureLoadSequence;
  sampledPaletteCache.clear();
  measureOverlayText = undefined;
  discardSettingsDraft();
  settingsOpen.value = false;
  capture.value = payload;
  dragStart.value = null;
  selection.value = null;
  resultRect.value = null;
  textBlocks.value = [];
  resetTranslationText();
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  expandedTranslation.value = false;
  screenshotNotice.value = "";
  manualDirection.value = false;
  lastImageRegion.value = null;
  currentCropDataUrl.value = "";
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

  phase.value = payload.selectedText ? "loading" : "ready";
  await nextTick();
  if (sequence !== captureLoadSequence) return;
  await showCaptureWindow();
  if (sequence !== captureLoadSequence) return;
  if (payload.selectedText) {
    await nextTick();
    await submitNativeSelection(sequence);
  }
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
  const context = canvas.getContext("2d", { willReadFrequently: true });
  if (!context) {
    throw new Error("Canvas 2D context is unavailable");
  }

  context.clearRect(0, 0, canvas.width, canvas.height);
  context.drawImage(image, 0, 0, canvas.width, canvas.height);
  return true;
}

function nativeSelectionRect(): Rect | null {
  const native = capture.value?.selectedText;
  const bounds = canvasRef.value?.getBoundingClientRect();
  if (!native || !bounds || bounds.width <= 0 || bounds.height <= 0) return null;
  return { x: native.x * bounds.width, y: native.y * bounds.height, width: native.width * bounds.width, height: native.height * bounds.height };
}

async function submitNativeSelection(sequence: number): Promise<void> {
  const native = capture.value?.selectedText;
  const rect = nativeSelectionRect();
  if (!native || !rect || sequence !== captureLoadSequence) return;
  resultRect.value = rect;
  textBlocks.value = native.blocks;
  phase.value = "streaming";
  const generation = workflowGeneration.value;
  try {
    await translateSelection(native.id, directionForRequest(), generation);
  } catch (error) {
    if (generation !== workflowGeneration.value) return;
    errorMessage.value = error instanceof Error ? error.message : "Unable to translate selected text";
    phase.value = "error";
  }
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
  textBlocks.value = [];
  resetTranslationText();
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  expandedTranslation.value = false;
  phase.value = "processing";

  try {
    // Let Vue paint the OCR indicator before the backend crop runs, keeping
    // the interaction task free. The backend crops the captured frame with
    // the same DPI mapping, so no PNG crosses the Wails bridge.
    await nextTick();
    if (imageRect.width <= 0 || imageRect.height <= 0) {
      throw new Error("Selection is empty");
    }
    lastImageRegion.value = imageRect;
    await translateRegion(imageRect, directionForRequest(), workflowGeneration.value);
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
  extensionOpen.value=false;sourceText.value="";
  invalidatePendingCaptureLoad();
  detachCaptureDragListeners();
  dragStart.value = null;
  selection.value = null;
  capture.value = null;
  resultRect.value = null;
  textBlocks.value = [];
  resetTranslationText();
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  expandedTranslation.value = false;
  currentCropDataUrl.value = "";
  lastImageRegion.value = null;
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

// A completed copy may belong to a selection the user has already left.
async function performResultCopy(copy: () => Promise<void>, closeOnSuccess: boolean): Promise<void> {
  const generation = workflowGeneration.value;
  if (copyingGeneration.value === generation) return;
  copyingGeneration.value = generation;
  try {
    await copy();
  } catch {
    if (generation === workflowGeneration.value) screenshotNotice.value = settingsText.value.copyFailed;
    return;
  } finally {
    if (copyingGeneration.value === generation) copyingGeneration.value = null;
  }
  if (generation !== workflowGeneration.value) return;
  screenshotNotice.value = "";
  if (closeOnSuccess) {
    await restore();
    return;
  }
  // Automatic copying must not dismiss a result before it can be read.
  copied.value = true;
  window.setTimeout(() => {
    if (generation === workflowGeneration.value) copied.value = false;
  }, 1100);
}

async function copyResult(closeOnSuccess = true): Promise<void> {
  const text = cleanTranslationText.value;
  if (!text) return;
  await performResultCopy(() => copyText(text), closeOnSuccess);
}

async function copyOcrText(): Promise<void> {
  const text = sourceText.value || textBlocks.value.map((block) => block.text).join("\n").trim();
  if (!text) {
    return;
  }
  try { await copyText(text); screenshotNotice.value = settingsText.value.copied; }
  catch { screenshotNotice.value = settingsText.value.copyFailed; }
}

async function copyTranslatedImage(): Promise<void> {
  await performResultCopy(async () => {
    const dataUrl = renderTranslatedSelectionImage();
    if (!dataUrl) throw new Error("No translated image is available");
    await copyImageDataUrl(dataUrl);
  }, true);
}

async function reverseTranslationDirection(): Promise<void> {
  if (!hasResultSource.value || isResultBusy.value) {
    return;
  }

  manualDirection.value = true;
  translationDirection.value = translationDirection.value === "to-zh" ? "to-en" : "to-zh";
  textBlocks.value = [];
  resetTranslationText();
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  expandedTranslation.value = false;
  screenshotNotice.value = "";
  phase.value = "streaming";
  const generation = ++workflowGeneration.value;

  try {
    await runProcessing(directionForRequest(), generation);
  } catch (error) {
    if (generation !== workflowGeneration.value) return;
    errorMessage.value = error instanceof Error ? error.message : "Failed to reverse translation direction";
    phase.value = "error";
  }
}

async function retryProcessing(): Promise<void> {
  if (!hasResultSource.value || isResultBusy.value) {
    return;
  }

  errorMessage.value = "";
  textBlocks.value = [];
  resetTranslationText();
  copied.value = false;
  copiedImage.value = false;
  expandedTranslation.value = false;
  phase.value = "processing";
  const generation = ++workflowGeneration.value;

  try {
    await runProcessing(directionForRequest(), generation);
  } catch (error) {
    if (generation !== workflowGeneration.value) return;
    errorMessage.value = error instanceof Error ? error.message : "Failed to retry";
    phase.value = "error";
  }
}

async function runProcessing(direction: TranslationDirection, generation: number): Promise<void> {
  if (capture.value?.selectedText) {
    phase.value = "streaming";
    textBlocks.value = capture.value.selectedText.blocks;
    await translateSelection(capture.value.selectedText.id, direction, generation);
    return;
  }
  if (lastImageRegion.value) {
    await translateRegion(lastImageRegion.value, direction, generation);
    return;
  }
  if (currentCropDataUrl.value) {
    await processImage(currentCropDataUrl.value, direction, generation);
    return;
  }
  throw new Error("No selection source is available");
}

function directionForRequest(): TranslationDirection {
  if (savedConfig.autoDirection && !manualDirection.value) {
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

  const target = document.createElement("canvas");
  target.width = imageRect.width;
  target.height = imageRect.height;
  const context = target.getContext("2d");
  if (!context) return null;
  context.drawImage(sourceCanvas, imageRect.x, imageRect.y, imageRect.width, imageRect.height, 0, 0, target.width, target.height);
  context.save();
  context.scale(imageRect.width / rect.width, imageRect.height / rect.height);
  for(const mask of visibleNativeBackdrops.value) {
    context.fillStyle=mask.background;
    context.fillRect(mask.rect.x,mask.rect.y,mask.rect.width,mask.rect.height);
  }
  context.restore();
  paintTranslationOverlay(context, styledOverlayBlocks.value, imageRect.width / rect.width, imageRect.height / rect.height);
  return target.toDataURL("image/png");
}

function openResultTools(): void {
 const canvas=canvasRef.value, rect=resultRect.value;if(!canvas || !rect)return;
 try {
 const bounds=canvas.getBoundingClientRect();
 const imageRect=mapCssRectToImageRect(rect,{width:bounds.width,height:bounds.height},{width:canvas.width,height:canvas.height},capture.value?.displays);
 const cropped=document.createElement("canvas");cropped.width=imageRect.width;cropped.height=imageRect.height;
 const ctx=cropped.getContext("2d");if(!ctx)return;
 ctx.drawImage(canvas,imageRect.x,imageRect.y,imageRect.width,imageRect.height,0,0,cropped.width,cropped.height);
 extensionImage.value=cropped.toDataURL("image/png");
 extensionTranslatedImage.value=cleanTranslationText.value?renderTranslatedSelectionImage()??"":"";
 extensionOrigin.value={x:(capture.value?.originX??0)+imageRect.x,y:(capture.value?.originY??0)+imageRect.y};
 extensionOpen.value=true;
 }catch(error){screenshotNotice.value=String(error);}
}
async function favoriteHistory(entry:HistoryEntry):Promise<void>{
 if(favoritePending.value)return;favoritePending.value=entry.id;
 try{await setHistoryFavorite(entry.id,!entry.favorite);await loadHistory();}catch(error){settingsError.value=String(error);}finally{favoritePending.value="";}
}
async function exportFavorites():Promise<void>{
 try{await exportMarkdown(libraryMarkdown(historyEntries.value.filter(e=>e.favorite && e.kind!=="learning")));}catch(error){settingsError.value=String(error);}
}
function cssColorForSampledColor(color: SampledColor | null): string | null {
  if (!color) {
    return null;
  }

  return `rgb(${Math.round(color.red)}, ${Math.round(color.green)}, ${Math.round(color.blue)})`;
}

async function restore(): Promise<void> {
  extensionOpen.value=false;sourceText.value="";
  phase.value = "idle";
  capture.value = null;
  selection.value = null;
  resultRect.value = null;
  textBlocks.value = [];
  resetTranslationText();
  currentCropDataUrl.value = "";
  lastImageRegion.value = null;
  errorMessage.value = "";
  copied.value = false;
  copiedImage.value = false;
  expandedTranslation.value = false;
  workflowGeneration.value += 1;
  await hideWindow();
}

function discardSettingsDraft(): void {
  Object.assign(config, savedConfig);
  autoStartEnabled.value = savedAutoStart.value;
  connectionTestSequence += 1;
  testStatus.value = "idle";
  testMessage.value = "";
}

async function closeSettings(): Promise<void> {
  if (settingsSaving.value) return;
  discardSettingsDraft();
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
  autoStartLoading.value = true;
  try {
    savedAutoStart.value = await isAutoStartEnabled();
    autoStartEnabled.value = savedAutoStart.value;
  } catch (error) {
    settingsError.value = error instanceof Error ? error.message : "Failed to read autostart";
  } finally {
    autoStartLoading.value = false;
  }
}

async function copyHistoryEntry(entry: HistoryEntry): Promise<void> {
  try {
    await copyText(bilingualHistoryText(entry, settingsLocale.value));
    copiedHistoryID.value = entry.id;
    window.setTimeout(() => { if (copiedHistoryID.value === entry.id) copiedHistoryID.value = ""; }, 1500);
  } catch {
    settingsError.value = settingsText.value.copyFailed;
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
    await loadHistory();
  } catch (error) {
    settingsError.value = error instanceof Error ? error.message : "Failed to clear history";
  }
}

async function runConnectionTest(): Promise<void> {
  const sequence = ++connectionTestSequence;
  testStatus.value = "testing";
  testMessage.value = "";
  try {
    await testConnection({ ...config });
    if (sequence !== connectionTestSequence) return;
    testStatus.value = "ok";
    testMessage.value = settingsText.value.connectionSuccessful;
  } catch (error) {
    if (sequence !== connectionTestSequence) return;
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
  if (settingsSaving.value || autoStartLoading.value) return;
  settingsError.value = "";
  settingsSaving.value = true;
  try {
    const draft = { ...config };
    const autoStart = autoStartEnabled.value;
    await saveConfig(draft, autoStart);
    Object.assign(savedConfig, draft);
    savedAutoStart.value = autoStart;
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
    class="relative h-full w-full overflow-hidden bg-transparent"
    :class="config.theme === 'dark' ? 'dark text-slate-50' : 'text-slate-950'"
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
        <button type="button" class="settings-secondary-button mb-4" @click="triggerScreenshot">{{settingsLocale==='en'?'Screenshot & annotate':'截图与涂鸦'}}</button>
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
      <div v-if="capture.notice && phase === 'ready'" role="status" class="absolute left-1/2 top-5 max-w-[90vw] -translate-x-1/2 rounded-lg bg-slate-900/90 px-4 py-2 text-sm text-white">{{ capture.notice }}</div>
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
      :features="featureFlags"
      :locale="settingsLocale"
      @cancel="cancelCapture"
      @complete="completeScreenshot"
      @save="saveEditedScreenshot"
    />

    <ExtensionWorkbench v-if="extensionOpen" :features="featureFlags" :locale="settingsLocale" :image="extensionImage" :translated-image="extensionTranslatedImage" :source="sourceText" :translation="cleanTranslationText" :origin="extensionOrigin" @close="extensionOpen=false" @pinned="restore" />

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
          ? (capture?.selectedText ? 'rounded-sm' : 'rounded-sm ring-1 ring-emerald-400/70')
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

      <div v-if="phase === 'partial'" data-testid="partial-translation" role="alert"
        class="absolute bottom-full left-0 z-40 mb-2 flex max-w-full items-center gap-3 rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-900 shadow-sm">
        <span :title="errorMessage">{{ settingsText.partialTranslation }}</span>
        <button type="button" class="shrink-0 font-semibold underline" aria-label="Retry translation" @click="retryProcessing">{{ settingsText.retry }}</button>
      </div>
      <div v-if="screenshotNotice" role="status" class="absolute bottom-0 left-0 z-40 rounded bg-slate-900 px-3 py-2 text-xs text-white">{{ screenshotNotice }}</div>
      <template v-if="hasTextBlockLayout && phase !== 'error'">
        <div v-for="mask in visibleNativeBackdrops" :key="'backdrop-'+mask.index" class="pointer-events-none absolute" :style="{left:mask.rect.x+'px',top:mask.rect.y+'px',width:mask.rect.width+'px',height:mask.rect.height+'px',background:mask.background}" />
        <div v-for="block in visibleOverlayBlocks" :key="block.key" class="absolute"
          :style="block.style" :title="block.truncated ? block.text : undefined"
          :data-testid="block.kind === 'prose' ? 'translation-line' : 'ocr-block'">
          <span v-for="(line, index) in block.lines" :key="index" class="block shrink-0" :style="{ paddingLeft: (index > 0 ? block.indent : 0) + 'px' }">{{ line }}</span>
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
              :disabled="!hasResultSource"
              @click="retryProcessing"
            >
              <RefreshCw class="h-3.5 w-3.5" aria-hidden="true" />
              Retry
            </button>
            <button
              class="inline-flex h-8 items-center gap-1.5 rounded-md border border-slate-300 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:bg-zinc-900 dark:text-slate-200 dark:hover:bg-zinc-800"
              type="button"
              title="Copy original text"
              aria-label="Copy original text"
              :disabled="!textBlocks.length"
              @click="copyOcrText"
            >
              <Copy class="h-3.5 w-3.5" aria-hidden="true" />
              {{ settingsLocale === 'en' ? 'Copy original' : '复制原文' }}
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

      <div v-if="capture?.selectedText && preservesSourcePreview" class="pointer-events-none absolute inset-0 z-20 rounded-sm ring-1 ring-emerald-400/70" />
      <aside v-if="expandedTranslation" class="fixed right-4 top-4 z-50 flex max-h-[calc(100vh-32px)] w-[min(420px,calc(100vw-32px))] flex-col rounded-xl border border-slate-200 bg-white p-4 text-slate-900 shadow-xl dark:border-zinc-700 dark:bg-zinc-900 dark:text-white">
        <div class="mb-3 flex items-center justify-between gap-4">
          <strong class="text-sm">{{ settingsLocale === "en" ? "Full translation" : "完整译文" }}</strong>
          <button type="button" aria-label="Close full translation" @click="expandedTranslation = false"><X class="h-4 w-4" /></button>
        </div>
        <div class="overflow-auto whitespace-pre-wrap break-words text-sm leading-6">{{ cleanTranslationText }}</div>
      </aside>
      <div
        class="absolute flex h-10 items-center gap-2 rounded-md border border-white/70 bg-white/95 px-2 shadow-[0_10px_32px_rgba(15,23,42,0.22)] backdrop-blur dark:border-slate-700/70 dark:bg-zinc-950/95"
        :style="resultActionsStyle"
      >
        <button v-if="hasResultTools" type="button" class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-emerald-700 hover:bg-emerald-50" :title="settingsLocale==='en'?'Extension tools':'扩展工具'" :aria-label="settingsLocale==='en'?'Extension tools':'扩展工具'" data-testid="result-extensions" :disabled="isResultBusy" @click="openResultTools"><Sparkles class="h-4 w-4"/></button>
        <button v-if="overlayLayout.truncated" type="button" class="whitespace-nowrap px-1 text-xs font-medium text-emerald-700 dark:text-emerald-300"
          aria-label="Show full translation" @click="expandedTranslation = !expandedTranslation">{{ settingsLocale === "en" ? "Full text" : "完整译文" }}</button>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md bg-slate-900 text-white transition hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-slate-100 dark:text-slate-950 dark:hover:bg-white"
          type="button"
          :title="settingsLocale === 'en' ? 'Copy text and close' : '复制译文并收起'"
          aria-label="Copy translated text"
          :disabled="!cleanTranslationText || copyingGeneration === workflowGeneration"
          @click="copyResult()"
        >
          <Check v-if="copied" class="h-4 w-4" aria-hidden="true" />
          <Copy v-else class="h-4 w-4" aria-hidden="true" />
        </button>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-300 bg-white text-slate-800 transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:bg-zinc-900 dark:text-slate-100 dark:hover:bg-zinc-800"
          type="button"
          :title="settingsLocale === 'en' ? 'Copy image and close' : '复制译后图片并收起'"
          aria-label="Copy translated screenshot"
          :disabled="!cleanTranslationText || copyingGeneration === workflowGeneration"
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
          :disabled="!hasResultSource || isResultBusy"
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
        novalidate
        @submit.prevent="submitSettings"
        @keydown="onShortcutRecorderKeydown"
      >
        <fieldset class="contents" :disabled="settingsSaving">
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
            class="flex shrink-0 items-center gap-2"
            style="--wails-draggable: no-drag"
          >
            <button
              data-testid="theme-toggle"
              class="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-slate-100 text-slate-500 ring-1 ring-slate-200/80 transition hover:text-slate-900 dark:bg-zinc-900 dark:text-slate-400 dark:ring-zinc-800 dark:hover:text-white"
              style="--wails-draggable: no-drag"
              type="button"
              :title="config.theme === 'dark' ? settingsText.useLightTheme : settingsText.useDarkTheme"
              :aria-label="config.theme === 'dark' ? settingsText.useLightTheme : settingsText.useDarkTheme"
              :aria-pressed="config.theme === 'dark'"
              @click="toggleTheme"
            >
              <Sun v-if="config.theme === 'dark'" class="h-4 w-4" aria-hidden="true" />
              <Moon v-else class="h-4 w-4" aria-hidden="true" />
            </button>
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
              data-testid="settings-minimize"
              class="inline-flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 transition hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-zinc-800 dark:hover:text-white"
              style="--wails-draggable: no-drag"
              type="button"
              :title="settingsText.minimize"
              :aria-label="settingsText.minimize"
              @click="minimizeWindow"
            >
              <Minus class="h-4 w-4" aria-hidden="true" />
            </button>
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

        <SettingsNavigation :model-value="settingsSection" :locale="settingsLocale" @update:model-value="selectSettingsSection" />
        <div ref="settingsScrollRef" data-testid="settings-scroll" class="settings-scrollbar min-h-0 flex-1 overflow-y-auto px-6 py-4">
          <div class="mx-auto max-w-3xl">

            <section v-show="settingsSection === 'ai'" id="settings-panel-ai" role="tabpanel" aria-labelledby="settings-tab-ai" tabindex="0" data-testid="settings-section" class="settings-card">
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
          <input
            v-model="config.apiKey"
            data-testid="api-key-input"
            class="settings-input"
            type="password"
            autocomplete="off"
            :placeholder="settingsText.apiKeyPlaceholder"
          />
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

            <section v-show="settingsSection === 'capture'" id="settings-panel-capture" role="tabpanel" aria-labelledby="settings-tab-capture" tabindex="0" data-testid="settings-section" class="settings-card">
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
            data-testid="autostart-toggle"
            v-model="autoStartEnabled"
            class="peer sr-only"
            type="checkbox"
            :disabled="autoStartLoading || settingsSaving"
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

            <section v-show="settingsSection === 'translation'" id="settings-panel-translation" role="tabpanel" aria-labelledby="settings-tab-translation" tabindex="0" data-testid="settings-section" class="settings-card">
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
          <span class="settings-label">{{ settingsText.translationTimeout }}</span>
          <input
            v-model.number="config.translationTimeoutSeconds"
            class="settings-input"
            type="number"
            min="10"
            max="600"
            step="5"
            inputmode="numeric"
          />
          <span class="settings-help">{{ settingsText.translationTimeoutHelp }}</span>
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

            <div v-show="settingsSection === 'productivity'" id="settings-panel-productivity" role="tabpanel" aria-labelledby="settings-tab-productivity" tabindex="0">
              <ExtensionSettings v-model="config.features" :locale="settingsLocale" group="productivity" />
            </div>
            <div v-show="settingsSection === 'experiments'" id="settings-panel-experiments" role="tabpanel" aria-labelledby="settings-tab-experiments" tabindex="0">
              <ExtensionSettings v-model="config.features" :locale="settingsLocale" group="experiments" />
            </div>
            <div v-show="settingsSection === 'library'" id="settings-panel-library" role="tabpanel" aria-labelledby="settings-tab-library" tabindex="0" class="space-y-4">
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
            {{ settingsLocale === "en" ? "Clear recent" : "清空最近" }}
          </button>
        </div>
        <div v-if="featureFlags.historyTools" class="mb-3 flex flex-wrap items-center gap-2">
          <input v-model="historyQuery" type="search" class="settings-input min-w-0 flex-1" :aria-label="settingsLocale==='en'?'Search history':'搜索历史'" :placeholder="settingsLocale==='en'?'Search original or translation':'搜索原文或译文'"/>
          <label class="flex items-center gap-1 text-xs"><input v-model="favoritesOnly" type="checkbox" class="accent-emerald-600"/>{{settingsLocale==='en'?'Favorites':'只看收藏'}}</label>
          <button type="button" class="settings-tertiary-button" :disabled="!historyEntries.some(e=>e.favorite && e.kind!=='learning')" @click="exportFavorites">{{settingsLocale==='en'?'Export favorites':'导出收藏'}}</button>
          <span class="w-full text-[11px] text-slate-500">{{settingsLocale==='en'?'Clearing recent history keeps favorites and learning cards.':'清空最近记录会保留收藏和学习卡片。'}}</span>
        </div>
        <div v-if="visibleHistory.length === 0" class="rounded-lg border border-dashed border-slate-200 bg-slate-50 px-4 py-5 text-center text-xs text-slate-500 dark:border-zinc-800 dark:bg-zinc-900/50 dark:text-slate-400">
          {{ historyLoaded ? settingsText.noRecentTranslations : settingsText.loadingHistory }}
        </div>
        <div v-else class="max-h-64 divide-y divide-slate-100 overflow-y-auto rounded-lg border border-slate-200 dark:divide-zinc-800 dark:border-zinc-800">
          <div
            v-for="entry in visibleHistory"
            :key="entry.id"
            class="group flex items-center gap-3 px-3 py-2.5 transition hover:bg-slate-50 dark:hover:bg-zinc-900"
          >
            <details class="group/history min-w-0 flex-1 text-xs">
              <summary class="cursor-pointer list-none rounded focus-visible:outline-emerald-500">
                <div class="mb-1 flex items-center justify-between gap-2">
                  <time :datetime="entry.timestamp" class="text-[10px] tabular-nums text-slate-500">{{ historyTimestamp(entry.timestamp, settingsLocale) }}</time>
                  <ChevronDown class="h-3 w-3 text-slate-400 transition-transform group-open/history:rotate-180" aria-hidden="true" />
                </div>
                <div class="truncate text-slate-500 group-open/history:hidden dark:text-slate-400">{{ entry.source }}</div>
                <div class="mt-1 truncate font-medium text-slate-800 group-open/history:hidden dark:text-slate-100">{{ readableTranslation(entry.translation) }}</div>
              </summary>
              <div class="mt-3 space-y-3 border-t border-slate-100 pt-3 leading-relaxed dark:border-zinc-800">
                <div><span class="mb-1 block text-[10px] font-medium text-slate-400">{{ settingsLocale === "en" ? "Original" : "原文" }}</span><p class="whitespace-pre-wrap break-words text-slate-500">{{ entry.source }}</p></div>
                <div><span class="mb-1 block text-[10px] font-medium text-slate-400">{{ settingsLocale === "en" ? "Translation" : "译文" }}</span><p class="whitespace-pre-wrap break-words text-slate-800 dark:text-slate-100">{{ readableTranslation(entry.translation) }}</p></div>
              </div>
            </details>
            <button v-if="featureFlags.historyTools" type="button" class="inline-flex h-8 w-8 shrink-0 self-start items-center justify-center rounded-md text-amber-500 hover:bg-amber-50" :disabled="Boolean(favoritePending)" :aria-label="entry.favorite?'Unfavorite':'Favorite'" :title="entry.favorite?(settingsLocale==='en'?'Remove favorite':'取消收藏'):(settingsLocale==='en'?'Favorite':'收藏')" @click="favoriteHistory(entry)"><Star class="h-4 w-4" :fill="entry.favorite?'currentColor':'none'"/></button>
            <button
              class="inline-flex h-8 w-8 shrink-0 self-start items-center justify-center rounded-md text-slate-400 transition hover:bg-white hover:text-slate-700 hover:shadow-sm dark:hover:bg-zinc-800 dark:hover:text-slate-100"
              type="button"
              :title="copiedHistoryID === entry.id ? settingsText.copied : settingsText.copyHistoryEntry"
              aria-label="Copy history entry"
              @click="copyHistoryEntry(entry)"
            >
              <Check v-if="copiedHistoryID === entry.id" class="h-3.5 w-3.5 text-emerald-600" aria-hidden="true" />
              <Copy v-else class="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          </div>
        </div>
            </section>
            <LearningLibrary v-if="featureFlags.learningCards" :entries="historyEntries" :locale="settingsLocale" @changed="loadHistory" />
            </div>
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
              :disabled="settingsSaving || autoStartLoading"
            >
              {{ settingsSaving ? settingsText.saving : settingsText.saveChanges }}
            </button>
          </div>
        </div>
        </footer>
        </fieldset>
      </form>
    </section>
  </main>
</template>
