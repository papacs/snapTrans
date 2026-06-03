<script setup lang="ts">
import { Camera, Check, Copy, Settings, X } from "lucide-vue-next";
import MarkdownIt from "markdown-it";
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import {
  copyText,
  defaultConfig,
  hasWailsBackend,
  hideWindow,
  loadConfig,
  onBackendEvent,
  processImage,
  saveConfig,
  triggerCapture,
  type AppConfig,
  type CapturePayload,
  type WorkflowErrorPayload
} from "./services/backend";
import {
  cropCanvasToDataUrl,
  isUsableSelection,
  mapCssRectToImageRect,
  normalizeResultBox,
  normalizeRect,
  type Point,
  type Rect
} from "./utils/selection";

type Phase = "idle" | "ready" | "drawing" | "processing" | "streaming" | "done" | "error";

const canvasRef = ref<HTMLCanvasElement | null>(null);
const resultPanelRef = ref<HTMLElement | null>(null);
const phase = ref<Phase>("idle");
const capture = ref<CapturePayload | null>(null);
const dragStart = ref<Point | null>(null);
const selection = ref<Rect | null>(null);
const resultRect = ref<Rect | null>(null);
const viewport = reactive({ width: window.innerWidth, height: window.innerHeight });
const translationText = ref("");
const errorMessage = ref("");
const settingsOpen = ref(false);
const copied = ref(false);
const isDesktop = hasWailsBackend();
const markdown = new MarkdownIt({ breaks: true, linkify: false });
const config = reactive<AppConfig>({ ...defaultConfig });

const renderedTranslation = computed(() => {
  if (translationText.value.trim().length === 0) {
    return "";
  }
  return markdown.render(translationText.value);
});

const resultStyle = computed(() => {
  const rect = resultRect.value;
  if (!rect) {
    return {};
  }

  const box = normalizeResultBox(rect, viewport);

  return {
    left: `${box.x}px`,
    top: `${box.y}px`,
    width: `${box.width}px`,
    minHeight: `${box.height}px`
  };
});

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
      errorMessage.value = payload.message;
      phase.value = "error";
    }),
    onBackendEvent("settings-open", () => {
      capture.value = null;
      resultRect.value = null;
      selection.value = null;
      translationText.value = "";
      errorMessage.value = "";
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
  translationText.value = "";
  errorMessage.value = "";
  copied.value = false;
  phase.value = "ready";

  await nextTick();
  await drawCapture(payload);
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
  translationText.value = "";
  errorMessage.value = "";
  copied.value = false;
  phase.value = "processing";

  try {
    const crop = cropCanvasToDataUrl(canvas, imageRect);
    await processImage(crop);
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
  return phase.value === "ready" || phase.value === "drawing";
}

async function cancelCapture(): Promise<void> {
  dragStart.value = null;
  selection.value = null;
  capture.value = null;
  resultRect.value = null;
  translationText.value = "";
  errorMessage.value = "";
  copied.value = false;
  phase.value = "idle";
  await hideWindow();
}

async function copyResult(): Promise<void> {
  if (!translationText.value) {
    return;
  }

  await copyText(translationText.value);
  copied.value = true;
  window.setTimeout(() => {
    copied.value = false;
  }, 1100);
}

async function restore(): Promise<void> {
  phase.value = "idle";
  capture.value = null;
  selection.value = null;
  resultRect.value = null;
  translationText.value = "";
  errorMessage.value = "";
  copied.value = false;
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
      class="absolute inset-0 z-10 cursor-crosshair select-none"
      @mousedown.left="onMouseDown"
      @mousemove="onMouseMove"
      @mouseup.left="onMouseUp"
      @contextmenu="onContextMenu"
    >
      <canvas ref="canvasRef" class="h-full w-full object-fill" />
      <div
        v-if="phase === 'ready' || phase === 'drawing'"
        class="pointer-events-none absolute inset-0 bg-slate-950/20"
      />
      <div
        v-if="selection"
        class="pointer-events-none absolute border-2 border-emerald-300 bg-emerald-200/10 shadow-[0_0_0_9999px_rgba(2,6,23,0.26)] outline outline-1 outline-white/90"
        :style="selectionStyle"
      />
    </section>

    <section
      v-if="resultRect"
      ref="resultPanelRef"
      class="absolute z-20 max-h-[calc(100vh-32px)] overflow-hidden rounded-lg border border-white/50 bg-white/95 p-3 shadow-floating backdrop-blur-lg transition dark:border-slate-700/70 dark:bg-zinc-950/95 sm:p-4"
      :style="resultStyle"
    >
      <div v-if="phase === 'processing'" class="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-200">
        <span class="h-4 w-4 animate-spin rounded-full border-2 border-emerald-500 border-t-transparent" />
        <span>OCR...</span>
      </div>

      <div
        v-else-if="phase === 'streaming' && !translationText"
        class="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-200"
      >
        <span class="h-4 w-4 animate-spin rounded-full border-2 border-emerald-500 border-t-transparent" />
        <span>Translating...</span>
      </div>

      <div v-else-if="phase === 'error'" class="text-sm leading-6 text-rose-700 dark:text-rose-300">
        {{ errorMessage }}
      </div>

      <div
        v-else
        class="markdown-body max-h-[min(330px,calc(100vh-150px))] overflow-auto text-[15px] leading-7 text-slate-900 dark:text-slate-100"
        v-html="renderedTranslation"
      />

      <div class="mt-4 flex items-center justify-end gap-2 border-t border-slate-200/70 pt-3 dark:border-slate-800">
        <button
          class="inline-flex h-9 items-center gap-2 rounded-md bg-slate-900 px-3 text-sm font-medium text-white transition hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-slate-100 dark:text-slate-950 dark:hover:bg-white"
          type="button"
          :disabled="!translationText"
          @click="copyResult"
        >
          <Check v-if="copied" class="h-4 w-4" aria-hidden="true" />
          <Copy v-else class="h-4 w-4" aria-hidden="true" />
          Copy
        </button>
        <button
          class="inline-flex h-9 items-center gap-2 rounded-md border border-slate-300 bg-white px-3 text-sm font-medium text-slate-800 transition hover:bg-slate-50 dark:border-slate-700 dark:bg-zinc-900 dark:text-slate-100 dark:hover:bg-zinc-800"
          type="button"
          @click="restore"
        >
          <X class="h-4 w-4" aria-hidden="true" />
          Close
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
          <span class="mb-1 block">DeepSeek API Key</span>
          <input
            v-model="config.deepSeekAPIKey"
            class="h-10 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none transition focus:border-emerald-500 focus:ring-2 focus:ring-emerald-500/20 dark:border-slate-700 dark:bg-zinc-900"
            type="password"
            autocomplete="off"
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
