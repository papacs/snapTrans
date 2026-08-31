<script setup lang="ts">
import {
  ArrowUpRight,
  Sparkles,
  Check,
  Copy,
  Circle,
  ChevronsDown,
  Download,
  Grid3X3,
  LoaderCircle,
  Pencil,
  ScanText,
  Smile,
  Square,
  Type,
  Undo2,
  X
} from "lucide-vue-next";
import ExtensionWorkbench from "./ExtensionWorkbench.vue";
import { normalizeFeatures, type FeatureFlags } from "../utils/features";
import { mergeTextLines } from "../utils/extensions";
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";

import {
  beginScrollingScreenshot,
  cancelScrollingScreenshot,
  copyText,
  extractText,
  finishScrollingScreenshot,
  showCaptureWindow,
  stepScrollingScreenshot,
  type CapturePayload,
  type ManualScrollStatus,
  type ScrollCaptureRegion
} from "../services/backend";
import {
  createPixelatedCanvas,
  renderAnnotations,
  toolbarPosition,
  type Annotation,
  type AnnotationTool
} from "../utils/annotations";
import { mapCssRectToImageRect, type Point, type Rect } from "../utils/selection";

const props = defineProps<{
  sourceCanvas: HTMLCanvasElement;
  capture: CapturePayload;
  rect: Rect;
  features?: FeatureFlags;
  locale?: string;
}>();

const emit = defineEmits<{
  cancel: [];
  complete: [dataUrl: string];
  save: [dataUrl: string];
}>();

const featureFlags = computed(() => normalizeFeatures(props.features));
const toolsOpen = ref(false);
const toolsImage = ref("");
const hasTools = computed(() => Object.entries(featureFlags.value).some(([key,value]) => value && !["textExtraction","historyTools"].includes(key)));
const toolsOrigin = computed(() => {
 const bounds=props.sourceCanvas.getBoundingClientRect();
 const rect=mapCssRectToImageRect(props.rect,{width:bounds.width,height:bounds.height},{width:props.sourceCanvas.width,height:props.sourceCanvas.height},props.capture.displays);
 return {x:(props.capture.originX??0)+rect.x,y:(props.capture.originY??0)+rect.y};
});
function openTools(){commitText();toolsImage.value=exportDataUrl();toolsOpen.value=Boolean(toolsImage.value);}
function applyRedaction(rects:Rect[]){annotations.value.push({tool:"redact",rects});redraw();toolsOpen.value=false;}
const originalExtractedText = ref("");
const editorCanvasRef = ref<HTMLCanvasElement | null>(null);
const textInputRef = ref<HTMLInputElement | null>(null);
const activeTool = ref<AnnotationTool>("pen");
const color = ref("#ff4d4f");
const annotations = ref<Annotation[]>([]);
const currentAnnotation = ref<Annotation | null>(null);
const textDraft = ref<{ point: Point; value: string } | null>(null);
const startingScroll = ref(false);
const manualScrolling = ref(false);
const finishingScroll = ref(false);
const scrollNotice = ref("");
const scrollProgress = ref<ManualScrollStatus>({ frames: 1, width: 0, height: 0 });
const currentFrameImage = ref("");
const stitchedPreviewImage = ref("");
const scrollStepPending = ref(false);
const textExtractionOpen = ref(false);
const extractingText = ref(false);
const extractedText = ref("");
const textExtractionError = ref("");
const copiedExtractedText = ref(false);
const extractedTextRef = ref<HTMLTextAreaElement | null>(null);
let extractionSequence = 0;
let copiedTextTimer: number | null = null;
let baseCanvas: HTMLCanvasElement | null = null;
let pixelatedCanvas: HTMLCanvasElement | null = null;
let drawing = false;
let redrawFrame: number | null = null;
let disposed = false;
let scrollPollTimer: number | null = null;

const SCROLL_POLL_INTERVAL_MS = 180;

const TOOLBAR_WIDTH = 600;
const TOOLBAR_HEIGHT = 48;

const editorStyle = computed(() => ({
  left: `${props.rect.x}px`,
  top: `${props.rect.y}px`,
  width: `${props.rect.width}px`,
  height: `${props.rect.height}px`
}));

const toolbarStyle = computed(() => {
  const point = toolbarPosition(
    props.rect,
    { width: window.innerWidth, height: window.innerHeight },
    { width: Math.min(TOOLBAR_WIDTH, window.innerWidth - 16), height: TOOLBAR_HEIGHT }
  );
  return { left: `${point.x}px`, top: `${point.y}px`, maxWidth: "calc(100vw - 16px)" };
});

const manualToolbarStyle = computed(() => {
  const point = toolbarPosition(
    props.rect,
    { width: window.innerWidth, height: window.innerHeight },
    { width: 104, height: 48 }
  );
  return { left: `${point.x}px`, top: `${point.y}px` };
});

const previewPanelStyle = computed(() => {
  const gap = 12;
  const padding = 8;
  const width = Math.min(220, Math.max(150, window.innerWidth * 0.18));
  const height = Math.min(420, Math.max(220, props.rect.height));
  const right = props.rect.x + props.rect.width + gap;
  const left = props.rect.x - width - gap;
  const x = right + width <= window.innerWidth - padding
    ? right
    : left >= padding
      ? left
      : clamp(props.rect.x + props.rect.width - width - padding, padding, window.innerWidth - width - padding);
  const y = clamp(props.rect.y, padding, Math.max(padding, window.innerHeight - height - padding));
  return { left: `${x}px`, top: `${y}px`, width: `${width}px`, height: `${height}px` };
});

const extractionPanelStyle = computed(() => {
  const gap = 12;
  const padding = 8;
  const width = Math.min(360, Math.max(240, Math.round(window.innerWidth * 0.28)), Math.max(160, window.innerWidth - padding * 2));
  const height = Math.min(Math.max(260, props.rect.height), Math.max(180, window.innerHeight - padding * 2));
  const right = props.rect.x + props.rect.width + gap;
  const left = props.rect.x - width - gap;
  const x = right + width <= window.innerWidth - padding
    ? right
    : left >= padding
      ? left
      : Math.max(padding, window.innerWidth - width - padding);
  const y = clamp(
    props.rect.y,
    padding,
    Math.max(padding, window.innerHeight - height - padding)
  );
  return { left: `${x}px`, top: `${y}px`, width: `${width}px`, height: `${height}px` };
});

const textDraftStyle = computed(() => {
  const draft = textDraft.value;
  const canvas = editorCanvasRef.value;
  if (!draft || !canvas) {
    return {};
  }
  const bounds = canvas.getBoundingClientRect();
  return {
    left: `${bounds.left + (draft.point.x / canvas.width) * bounds.width}px`,
    top: `${bounds.top + (draft.point.y / canvas.height) * bounds.height}px`,
    color: color.value
  };
});

const toolButtons: Array<{
  tool: AnnotationTool;
  label: string;
  icon: typeof Square;
}> = [
  { tool: "rectangle", label: "矩形", icon: Square },
  { tool: "ellipse", label: "椭圆", icon: Circle },
  { tool: "sticker", label: "表情", icon: Smile },
  { tool: "arrow", label: "箭头", icon: ArrowUpRight },
  { tool: "pen", label: "画笔", icon: Pencil },
  { tool: "mosaic", label: "马赛克", icon: Grid3X3 },
  { tool: "text", label: "文字", icon: Type }
];

onMounted(() => {
  prepareCanvas();
  window.addEventListener("keydown", onEditorKeyDown);
});

onBeforeUnmount(() => {
  disposed = true;
  if (redrawFrame !== null) window.cancelAnimationFrame(redrawFrame);
  extractionSequence += 1;
  if (copiedTextTimer !== null) {
    window.clearTimeout(copiedTextTimer);
    copiedTextTimer = null;
  }
  stopScrollPolling();
  if (manualScrolling.value || startingScroll.value) {
    void cancelScrollingScreenshot();
  }
  detachDrawingListeners();
  window.removeEventListener("keydown", onEditorKeyDown);
});

function prepareCanvas(): void {
  const editor = editorCanvasRef.value;
  if (!editor) {
    return;
  }
  const sourceBounds = props.sourceCanvas.getBoundingClientRect();
  const imageRect = mapCssRectToImageRect(
    props.rect,
    { width: sourceBounds.width, height: sourceBounds.height },
    { width: props.sourceCanvas.width, height: props.sourceCanvas.height },
    props.capture.displays
  );

  editor.width = Math.max(1, imageRect.width);
  editor.height = Math.max(1, imageRect.height);
  baseCanvas = document.createElement("canvas");
  baseCanvas.width = editor.width;
  baseCanvas.height = editor.height;
  const baseContext = baseCanvas.getContext("2d");
  if (!baseContext) {
    return;
  }
  baseContext.drawImage(
    props.sourceCanvas,
    imageRect.x,
    imageRect.y,
    imageRect.width,
    imageRect.height,
    0,
    0,
    editor.width,
    editor.height
  );
  pixelatedCanvas = createPixelatedCanvas(baseCanvas, Math.max(8, Math.round(canvasScale() * 10)));
  redraw();
}

function canvasScale(): number {
  const canvas = editorCanvasRef.value;
  if (!canvas) {
    return 1;
  }
  const bounds = canvas.getBoundingClientRect();
  return Math.max(1, (canvas.width / bounds.width + canvas.height / bounds.height) / 2);
}

function canvasPoint(event: MouseEvent): Point {
  const canvas = editorCanvasRef.value;
  if (!canvas) {
    return { x: 0, y: 0 };
  }
  const bounds = canvas.getBoundingClientRect();
  return {
    x: clamp((event.clientX - bounds.left) * (canvas.width / bounds.width), 0, canvas.width),
    y: clamp((event.clientY - bounds.top) * (canvas.height / bounds.height), 0, canvas.height)
  };
}

function onMouseDown(event: MouseEvent): void {
  if (event.button !== 0 || startingScroll.value) {
    return;
  }
  event.preventDefault();
  event.stopPropagation();
  const point = canvasPoint(event);
  const scale = canvasScale();

  if (activeTool.value === "text") {
    textDraft.value = { point, value: "" };
    void nextTick(() => textInputRef.value?.focus());
    return;
  }
  if (activeTool.value === "sticker") {
    annotations.value.push({
      tool: "sticker",
      point,
      text: "😊",
      color: color.value,
      fontSize: 30 * scale
    });
    redraw();
    return;
  }

  drawing = true;
  if (activeTool.value === "pen" || activeTool.value === "mosaic") {
    currentAnnotation.value = {
      tool: activeTool.value,
      points: [point],
      color: color.value,
      width: (activeTool.value === "mosaic" ? 24 : 3) * scale
    };
  } else {
    currentAnnotation.value = {
      tool: activeTool.value,
      start: point,
      end: point,
      color: color.value,
      width: 3 * scale
    };
  }
  window.addEventListener("mousemove", onMouseMove);
  window.addEventListener("mouseup", onMouseUp);
}

async function captureDownwardScroll(): Promise<void> {
  const editor = editorCanvasRef.value;
  if (!editor || startingScroll.value || manualScrolling.value || annotations.value.length > 0) {
    return;
  }
  const sourceBounds = props.sourceCanvas.getBoundingClientRect();
  const imageRect = mapCssRectToImageRect(
    props.rect,
    { width: sourceBounds.width, height: sourceBounds.height },
    { width: props.sourceCanvas.width, height: props.sourceCanvas.height },
    props.capture.displays
  );
  const regionX = Math.round((props.capture.originX ?? 0) + imageRect.x);
  const regionY = Math.round((props.capture.originY ?? 0) + imageRect.y);

  startingScroll.value = true;
  const captureRegion: ScrollCaptureRegion = {
    x: regionX,
    y: regionY,
    width: imageRect.width,
    height: imageRect.height
  };
  manualScrolling.value = true;
  scrollNotice.value = "";
  scrollProgress.value = { frames: 1, width: imageRect.width, height: imageRect.height };
  currentFrameImage.value = baseCanvas?.toDataURL("image/png") ?? "";
  stitchedPreviewImage.value = currentFrameImage.value;
  await nextTick();

  try {
    scrollProgress.value = await beginScrollingScreenshot(captureRegion);
    scheduleScrollPoll(120);
    if (disposed || !manualScrolling.value) {
      await cancelScrollingScreenshot();
      return;
    }
  } catch (error) {
    manualScrolling.value = false;
    scrollNotice.value = error instanceof Error ? error.message : "滚动截图失败";
    if (!disposed) {
      await nextTick();
      await showCaptureWindow();
    }
  } finally {
    startingScroll.value = false;
  }
}

function stopScrollPolling(): void {
  if (scrollPollTimer !== null) {
    window.clearTimeout(scrollPollTimer);
    scrollPollTimer = null;
  }
}

function scheduleScrollPoll(delay = SCROLL_POLL_INTERVAL_MS): void {
  stopScrollPolling();
  if (disposed || !manualScrolling.value || finishingScroll.value) {
    return;
  }
  scrollPollTimer = window.setTimeout(() => {
    scrollPollTimer = null;
    void pollScrollingCapture();
  }, delay);
}

async function pollScrollingCapture(): Promise<void> {
  if (disposed || !manualScrolling.value || startingScroll.value || finishingScroll.value || scrollStepPending.value) {
    scheduleScrollPoll();
    return;
  }

  scrollStepPending.value = true;
  let pollingFailed = false;
  try {
    const result = await stepScrollingScreenshot();
    scrollProgress.value = { frames: result.frames, width: result.width, height: result.height };
    if (result.appended && result.currentImage && result.previewImage) {
      await Promise.all([preloadImage(result.currentImage), preloadImage(result.previewImage)]);
      currentFrameImage.value = result.currentImage;
      stitchedPreviewImage.value = result.previewImage;
      scrollNotice.value = "";
    }
    if (result.limitReached) {
      pollingFailed = true;
      scrollNotice.value = "已达到长截图大小或帧数上限，请点击完成以保留当前结果。";
    }
  } catch (error) {
    pollingFailed = true;
    scrollNotice.value = error instanceof Error ? error.message : "滚动画面采集失败";
  } finally {
    scrollStepPending.value = false;
    if (!pollingFailed) {
      scheduleScrollPoll();
    }
  }
}

function preloadImage(source: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve();
    image.onerror = () => reject(new Error("滚动截图预览加载失败"));
    image.src = source;
  });
}

async function finishManualScrollingCapture(): Promise<void> {
  if (!manualScrolling.value || finishingScroll.value) {
    return;
  }
  finishingScroll.value = true;
  stopScrollPolling();
  try {
    const result = await finishScrollingScreenshot();
    manualScrolling.value = false;
    emit("complete", result.image);
  } catch (error) {
    manualScrolling.value = false;
    scrollNotice.value = error instanceof Error ? error.message : "滚动截图失败";
    if (!disposed) {
      await nextTick();
      await showCaptureWindow();
    }
  } finally {
    finishingScroll.value = false;
  }
}

async function cancelManualScrollingCapture(): Promise<void> {
  if (!manualScrolling.value && !startingScroll.value) {
    return;
  }
  manualScrolling.value = false;
  stopScrollPolling();
  startingScroll.value = false;
  await cancelScrollingScreenshot();
  emit("cancel");
}

function onMouseMove(event: MouseEvent): void {
  if (!drawing || !currentAnnotation.value) {
    return;
  }
  const point = canvasPoint(event);
  const current = currentAnnotation.value;
  if (current.tool === "pen" || current.tool === "mosaic") {
    current.points.push(point);
  } else if (current.tool === "rectangle" || current.tool === "ellipse" || current.tool === "arrow") {
    currentAnnotation.value = { ...current, end: point };
  }
  scheduleRedraw();
}

function onMouseUp(event: MouseEvent): void {
  if (!drawing) {
    return;
  }
  onMouseMove(event);
  drawing = false;
  detachDrawingListeners();
  if (currentAnnotation.value) {
    annotations.value.push(currentAnnotation.value);
    currentAnnotation.value = null;
  }
  redraw();
}

function detachDrawingListeners(): void {
  window.removeEventListener("mousemove", onMouseMove);
  window.removeEventListener("mouseup", onMouseUp);
}

function scheduleRedraw(): void {
  if (redrawFrame !== null) return;
  if (typeof window.requestAnimationFrame !== "function") { redraw(); return; }
  redrawFrame = window.requestAnimationFrame(() => { redrawFrame = null; redraw(); });
}

function redraw(): void {
  if (redrawFrame !== null) { window.cancelAnimationFrame(redrawFrame); redrawFrame = null; }
  const editor = editorCanvasRef.value;
  if (!editor) {
    return;
  }
  const context = editor.getContext("2d");
  if (!context) {
    return;
  }
  context.clearRect(0, 0, editor.width, editor.height);
  const visible = currentAnnotation.value
    ? [...annotations.value, currentAnnotation.value]
    : annotations.value;
  renderAnnotations(context, visible, pixelatedCanvas ?? undefined);
}

function commitText(): void {
  const draft = textDraft.value;
  const value = draft?.value.trim();
  if (!draft || !value) {
    textDraft.value = null;
    return;
  }
  annotations.value.push({
    tool: "text",
    point: draft.point,
    text: value,
    color: color.value,
    fontSize: 20 * canvasScale()
  });
  textDraft.value = null;
  redraw();
}

function undo(): void {
  annotations.value.pop();
  redraw();
}
function onEditorKeyDown(event: KeyboardEvent): void {
  if (
    event.key.toLowerCase() !== "z" ||
    (!event.ctrlKey && !event.metaKey) ||
    event.shiftKey ||
    event.altKey ||
    toolsOpen.value ||
    annotations.value.length === 0 ||
    textDraft.value !== null ||
    manualScrolling.value
  ) {
    return;
  }

  const target = event.target;
  if (
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    (target instanceof HTMLElement && target.isContentEditable)
  ) {
    return;
  }

  event.preventDefault();
  undo();
}


function exportDataUrl(): string {
  const editor = editorCanvasRef.value;
  const source = baseCanvas;
  if (!editor || !source) {
    return "";
  }

  const target = document.createElement("canvas");
  target.width = editor.width;
  target.height = editor.height;
  const context = target.getContext("2d");
  if (!context) {
    return "";
  }

  context.drawImage(source, 0, 0);
  const visible = currentAnnotation.value
    ? [...annotations.value, currentAnnotation.value]
    : annotations.value;
  renderAnnotations(context, visible, pixelatedCanvas ?? undefined);
  return target.toDataURL("image/png");
}

function complete(): void {
  const dataUrl = exportDataUrl();
  if (dataUrl) {
    emit("complete", dataUrl);
  }
}

function save(): void {
  const dataUrl = exportDataUrl();
  if (dataUrl) {
    emit("save", dataUrl);
  }
}

async function extractImageText(): Promise<void> {
  const imageDataUrl = baseCanvas?.toDataURL("image/png") ?? "";
  if (!imageDataUrl || extractingText.value) {
    return;
  }

  const sequence = ++extractionSequence;
  textExtractionOpen.value = true;
  extractingText.value = true;
  extractedText.value = "";
  textExtractionError.value = "";
  copiedExtractedText.value = false;

  try {
    const result = await extractText(imageDataUrl);
    if (disposed || sequence !== extractionSequence) {
      return;
    }
    extractedText.value = result.text.trim();
    originalExtractedText.value = extractedText.value;
    if (!extractedText.value) {
      textExtractionError.value = "\u672a\u8bc6\u522b\u5230\u6587\u5b57\uff0c\u53ef\u4ee5\u91cd\u65b0\u8bc6\u522b\u6216\u624b\u52a8\u8f93\u5165\u3002";
    }
  } catch (error) {
    if (disposed || sequence !== extractionSequence) {
      return;
    }
    textExtractionError.value = error instanceof Error ? error.message : "\u6587\u5b57\u8bc6\u522b\u5931\u8d25";
  } finally {
    if (sequence === extractionSequence) {
      extractingText.value = false;
    }
  }

  if (!disposed && sequence === extractionSequence) {
    await nextTick();
    extractedTextRef.value?.focus();
  }
}

function closeTextExtraction(): void {
  extractionSequence += 1;
  textExtractionOpen.value = false;
  extractingText.value = false;
  textExtractionError.value = "";
  copiedExtractedText.value = false;
}

async function copyExtractedText(): Promise<void> {
  if (!extractedText.value.trim() || extractingText.value) {
    return;
  }

  try {
    await copyText(extractedText.value);
    textExtractionError.value = "";
    copiedExtractedText.value = true;
    if (copiedTextTimer !== null) {
      window.clearTimeout(copiedTextTimer);
    }
    copiedTextTimer = window.setTimeout(() => {
      copiedExtractedText.value = false;
      copiedTextTimer = null;
    }, 1200);
  } catch (error) {
    textExtractionError.value = error instanceof Error ? error.message : "\u590d\u5236\u6587\u5b57\u5931\u8d25";
  }
}
function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
</script>

<template>
  <section
    v-if="manualScrolling"
    class="fixed inset-0 z-[90] select-none text-slate-900"
    data-testid="manual-scroll-controller"
    @contextmenu.prevent="cancelManualScrollingCapture"
  >
    <div
      class="absolute overflow-hidden border-2 border-emerald-400 bg-white shadow-[0_0_0_9999px_rgba(2,6,23,0.48),0_0_0_1px_rgba(255,255,255,0.95)]"
      :style="editorStyle"
      data-testid="manual-scroll-current"
    >
      <img :src="currentFrameImage" class="h-full w-full object-fill" alt="当前滚动画面" draggable="false" />
    </div>

    <aside
      class="absolute flex flex-col overflow-hidden rounded-xl border border-slate-200 bg-white/98 p-2.5 shadow-[0_16px_40px_rgba(15,23,42,0.34)] backdrop-blur"
      :style="previewPanelStyle"
      data-testid="manual-scroll-preview"
      @wheel.stop
    >
      <div class="mb-2 flex items-center justify-between gap-2 px-0.5">
        <span class="text-xs font-semibold text-slate-700">完整长图预览</span>
        <span class="rounded bg-emerald-50 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-700">{{ scrollProgress.frames }} 帧</span>
      </div>
      <div class="flex min-h-0 flex-1 items-center justify-center overflow-hidden rounded-lg bg-slate-100 p-1">
        <img :src="stitchedPreviewImage" class="max-h-full max-w-full object-contain" alt="完整滚动截图预览" draggable="false" />
      <div class="mb-2 px-0.5 text-[10px] leading-4 text-slate-500">在选区内使用原窗口滚轮或滚动条，右侧会自动更新预览</div>
      </div>
      <div class="mt-2 truncate px-0.5 text-[10px] text-slate-500">{{ scrollProgress.width }} × {{ scrollProgress.height }}</div>
    </aside>

    <div
      class="absolute flex h-12 items-center gap-1 rounded-[10px] border border-slate-200 bg-white px-2 shadow-[0_8px_24px_rgba(15,23,42,0.28)]"
      :style="manualToolbarStyle"
    >
      <button
        class="inline-flex h-9 w-9 items-center justify-center rounded-lg text-rose-500 transition hover:bg-rose-50"
        type="button"
        title="取消滚动截图"
        aria-label="取消滚动截图"
        data-testid="manual-scroll-cancel"
        @click="cancelManualScrollingCapture"
      >
        <X class="h-5 w-5" aria-hidden="true" />
      </button>
      <span class="mx-1 h-6 w-px bg-slate-200" />
      <button
        class="inline-flex h-9 w-9 items-center justify-center rounded-lg text-emerald-500 transition hover:bg-emerald-50 disabled:opacity-40"
        type="button"
        title="完成滚动截图"
        aria-label="完成滚动截图"
        :disabled="startingScroll || finishingScroll"
        data-testid="manual-scroll-complete"
        @click="finishManualScrollingCapture"
      >
        <LoaderCircle v-if="finishingScroll" class="h-5 w-5 animate-spin" aria-hidden="true" />
        <Check v-else class="h-5 w-5" aria-hidden="true" />
      </button>
    </div>
  </section>

  <section v-else class="absolute inset-0 z-30 select-none" data-testid="screenshot-editor" @contextmenu.prevent="emit('cancel')">
    <div
      class="absolute overflow-hidden border-2 border-emerald-400 shadow-[0_0_0_9999px_rgba(2,6,23,0.42),0_0_0_1px_rgba(255,255,255,0.95)]"
      :style="editorStyle"
      data-testid="screenshot-viewport"
    >
      <canvas
        ref="editorCanvasRef"
        class="block h-auto w-full cursor-crosshair bg-transparent"
        :class="startingScroll ? 'pointer-events-none' : ''"
        @mousedown="onMouseDown"
      />
    </div>

    <input
      v-if="textDraft"
      ref="textInputRef"
      v-model="textDraft.value"
      class="absolute z-40 min-w-28 border-b-2 border-current bg-white/90 px-1 py-0.5 text-lg font-semibold outline-none shadow-sm"
      :style="textDraftStyle"
      type="text"
      placeholder="输入文字"
      @mousedown.stop
      @keydown.enter.prevent.stop="commitText"
      @keydown.escape.prevent.stop="textDraft = null"
      @blur="commitText"
    />

    <div
      class="absolute z-50 flex h-12 items-center gap-1 overflow-x-auto rounded-[10px] border border-slate-200 bg-white px-2 shadow-[0_8px_24px_rgba(15,23,42,0.24)]"
      :style="toolbarStyle"
      data-testid="screenshot-toolbar"
      @mousedown.stop
      @contextmenu.stop.prevent
    >
      <button
        v-for="item in toolButtons"
        :key="item.tool"
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition"
        :class="activeTool === item.tool ? 'bg-slate-100 text-slate-950' : 'text-slate-700 hover:bg-slate-100 hover:text-slate-950'"
        type="button"
        :title="item.label"
        :aria-label="item.label"
        :data-testid="`annotation-${item.tool}`"
        @click="activeTool = item.tool"
      >
        <component :is="item.icon" class="h-[18px] w-[18px]" aria-hidden="true" />
      </button>
      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition disabled:cursor-wait disabled:opacity-60"
        v-if="featureFlags.textExtraction"
        :class="textExtractionOpen ? 'bg-emerald-50 text-emerald-700' : 'text-slate-600 hover:bg-slate-100 hover:text-slate-950'"
        type="button"
        :title="'\u63d0\u53d6\u6587\u5b57'"
        :aria-label="'\u63d0\u53d6\u6587\u5b57'"
        :disabled="extractingText"
        data-testid="screenshot-extract-text"
        @click="extractImageText"
      >
        <LoaderCircle v-if="extractingText" class="h-[18px] w-[18px] animate-spin" aria-hidden="true" />
        <ScanText v-else class="h-[18px] w-[18px]" aria-hidden="true" />
      </button>

      <button v-if="hasTools" type="button" class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-emerald-700 hover:bg-emerald-50" :title="locale==='en'?'Extension tools':'扩展工具'" :aria-label="locale==='en'?'Extension tools':'扩展工具'" data-testid="screenshot-extensions" @click="openTools"><Sparkles class="h-[18px] w-[18px]"/></button>
      <span class="mx-1 h-6 w-px shrink-0 bg-slate-200" />
      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-600 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-35"
        type="button"
        :title="annotations.length > 0 ? '请先撤销标注，再进行滚动截图' : '向下滚动截图'"
        aria-label="向下滚动截图"
        :disabled="startingScroll || annotations.length > 0"
        data-testid="screenshot-scroll-down"
        @click="captureDownwardScroll"
      >
        <ChevronsDown class="h-[18px] w-[18px]" :class="startingScroll ? 'animate-pulse' : ''" aria-hidden="true" />
      </button>
      <label class="relative h-8 w-8 shrink-0 cursor-pointer overflow-hidden rounded-full border-2 border-white shadow ring-1 ring-slate-300" title="颜色">
        <span class="absolute inset-0" :style="{ backgroundColor: color }" />
        <input v-model="color" class="absolute inset-0 cursor-pointer opacity-0" type="color" aria-label="标注颜色" />
      </label>
      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-600 transition hover:bg-slate-100 disabled:opacity-35"
        type="button"
        title="撤销"
        aria-label="撤销"
        :disabled="annotations.length === 0"
        data-testid="annotation-undo"
        @click="undo"
      >
        <Undo2 class="h-[18px] w-[18px]" aria-hidden="true" />
      </button>

      <span class="mx-1 h-6 w-px shrink-0 bg-slate-200" />
      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-600 transition hover:bg-slate-100"
        type="button"
        title="保存图片"
        aria-label="保存图片"
        data-testid="screenshot-save"
        @click="save"
      >
        <Download class="h-[18px] w-[18px]" aria-hidden="true" />
      </button>
      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-rose-500 transition hover:bg-rose-50"
        type="button"
        title="取消"
        aria-label="取消"
        data-testid="screenshot-cancel"
        @click="emit('cancel')"
      >
        <X class="h-[19px] w-[19px]" aria-hidden="true" />
      </button>
      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-emerald-500 transition hover:bg-emerald-50 hover:text-emerald-600"
        type="button"
        title="完成并复制"
        aria-label="完成并复制"
        data-testid="screenshot-complete"
        @click="complete"
      >
        <Check class="h-5 w-5" aria-hidden="true" />
      </button>
    </div>

    <aside
      v-if="textExtractionOpen"
      class="absolute z-[60] flex select-text flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_18px_48px_rgba(15,23,42,0.30)]"
      :style="extractionPanelStyle"
      data-testid="text-extraction-panel"
      @mousedown.stop
      @contextmenu.stop.prevent
    >
      <header class="flex items-center justify-between gap-3 border-b border-slate-200 px-4 py-3">
        <div class="min-w-0">
          <div class="flex items-center gap-2 text-sm font-semibold text-slate-900">
            <ScanText class="h-4 w-4 text-emerald-600" aria-hidden="true" />
            <span>{{ "\u63d0\u53d6\u6587\u5b57" }}</span>
          </div>
          <p class="mt-0.5 text-[11px] text-slate-500">{{ "\u8bc6\u522b\u7ed3\u679c\u53ef\u4ee5\u76f4\u63a5\u7f16\u8f91\u548c\u590d\u5236" }}</p>
        </div>
        <div class="flex shrink-0 items-center gap-1">
          <button
            class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 transition hover:bg-slate-100 hover:text-slate-900 disabled:cursor-wait disabled:opacity-50"
            type="button"
            :title="'\u91cd\u65b0\u8bc6\u522b'"
            :aria-label="'\u91cd\u65b0\u8bc6\u522b'"
            :disabled="extractingText"
            data-testid="retry-text-extraction"
            @click="extractImageText"
          >
            <LoaderCircle v-if="extractingText" class="h-4 w-4 animate-spin" aria-hidden="true" />
            <ScanText v-else class="h-4 w-4" aria-hidden="true" />
          </button>
          <button
            class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 transition hover:bg-slate-100 hover:text-slate-900"
            type="button"
            :title="'\u5173\u95ed\u63d0\u53d6\u6587\u5b57'"
            :aria-label="'\u5173\u95ed\u63d0\u53d6\u6587\u5b57'"
            @click="closeTextExtraction"
          >
            <X class="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </header>

      <div v-if="textExtractionError" class="mx-4 mt-3 rounded-lg bg-rose-50 px-3 py-2 text-xs leading-5 text-rose-700">
        {{ textExtractionError }}
      </div>

      <div class="relative min-h-0 flex-1 p-4">
        <textarea
          ref="extractedTextRef"
          v-model="extractedText"
          class="h-full w-full resize-none rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-sm leading-6 text-slate-900 outline-none transition placeholder:text-slate-400 focus:border-emerald-500 focus:bg-white focus:ring-2 focus:ring-emerald-500/15 disabled:cursor-wait"
          :disabled="extractingText"
          :placeholder="'\u8bc6\u522b\u7ed3\u679c\u4f1a\u663e\u793a\u5728\u8fd9\u91cc\uff0c\u4e5f\u53ef\u4ee5\u624b\u52a8\u8f93\u5165\u6587\u5b57'"
          spellcheck="false"
          data-testid="extracted-text-editor"
          @keydown.escape.stop.prevent="closeTextExtraction"
        />
        <div
          v-if="extractingText"
          class="absolute inset-4 flex flex-col items-center justify-center rounded-lg bg-white/90 text-slate-600 backdrop-blur-sm"
          data-testid="text-extraction-loading"
        >
          <LoaderCircle class="h-8 w-8 animate-spin text-emerald-500" aria-hidden="true" />
          <span class="mt-3 text-sm font-medium">{{ "\u6b63\u5728\u8bc6\u522b\u56fe\u7247\u6587\u5b57..." }}</span>
        </div>
      </div>

      <footer class="flex items-center justify-between gap-3 border-t border-slate-200 px-4 py-3">
        <div class="flex flex-wrap gap-2"><button type="button" class="text-xs text-emerald-700 disabled:opacity-40" :disabled="extractingText" @click="extractedText=mergeTextLines(extractedText)">{{locale==='en'?'Merge lines':'合并换行'}}</button><button type="button" class="text-xs text-slate-500 disabled:opacity-40" :disabled="extractingText" @click="extractedText=originalExtractedText">{{locale==='en'?'Original breaks':'恢复换行'}}</button></div>
        <span class="text-[11px] text-slate-400">{{ extractedText.length }} {{ "\u4e2a\u5b57\u7b26" }}</span>
        <button
          class="inline-flex h-9 items-center justify-center gap-2 rounded-lg bg-emerald-600 px-4 text-xs font-semibold text-white transition hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-45"
          type="button"
          :disabled="extractingText || !extractedText.trim()"
          data-testid="copy-extracted-text"
          @click="copyExtractedText"
        >
          <Check v-if="copiedExtractedText" class="h-4 w-4" aria-hidden="true" />
          <Copy v-else class="h-4 w-4" aria-hidden="true" />
          {{ copiedExtractedText ? "\u5df2\u590d\u5236" : "\u590d\u5236\u6587\u5b57" }}
        </button>
      </footer>
    </aside>

    <div
      v-if="scrollNotice"
      class="absolute z-50 rounded-lg bg-slate-950/85 px-3 py-1.5 text-xs font-medium text-white shadow-lg"
      :style="{ left: `${props.rect.x + 8}px`, top: `${props.rect.y + 8}px` }"
      data-testid="scroll-capture-status"
    >
      {{ scrollNotice }}
    </div>
  </section>
    <ExtensionWorkbench v-if="toolsOpen" :features="featureFlags" :locale="locale??'zh-CN'" :image="toolsImage" :origin="toolsOrigin" allow-redaction @close="toolsOpen=false" @pinned="emit('cancel')" @redact="applyRedaction" />
</template>
