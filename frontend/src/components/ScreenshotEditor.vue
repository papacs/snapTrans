<script setup lang="ts">
import {
  ArrowUpRight,
  Check,
  Circle,
  Download,
  Grid3X3,
  Pencil,
  Smile,
  Square,
  Type,
  Undo2,
  X
} from "lucide-vue-next";
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";

import type { CapturePayload } from "../services/backend";
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
}>();

const emit = defineEmits<{
  cancel: [];
  complete: [dataUrl: string];
  save: [dataUrl: string];
}>();

const editorCanvasRef = ref<HTMLCanvasElement | null>(null);
const textInputRef = ref<HTMLInputElement | null>(null);
const activeTool = ref<AnnotationTool>("pen");
const color = ref("#ff4d4f");
const annotations = ref<Annotation[]>([]);
const currentAnnotation = ref<Annotation | null>(null);
const textDraft = ref<{ point: Point; value: string } | null>(null);
let baseCanvas: HTMLCanvasElement | null = null;
let pixelatedCanvas: HTMLCanvasElement | null = null;
let drawing = false;

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
    { width: Math.min(612, window.innerWidth - 16), height: 52 }
  );
  return { left: `${point.x}px`, top: `${point.y}px`, maxWidth: "calc(100vw - 16px)" };
});

const textDraftStyle = computed(() => {
  const draft = textDraft.value;
  const canvas = editorCanvasRef.value;
  if (!draft || !canvas) {
    return {};
  }
  return {
    left: `${props.rect.x + (draft.point.x / canvas.width) * props.rect.width}px`,
    top: `${props.rect.y + (draft.point.y / canvas.height) * props.rect.height}px`,
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
});

onBeforeUnmount(() => {
  detachDrawingListeners();
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
  return Math.max(1, (canvas.width / props.rect.width + canvas.height / props.rect.height) / 2);
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
  if (event.button !== 0) {
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

function onMouseMove(event: MouseEvent): void {
  if (!drawing || !currentAnnotation.value) {
    return;
  }
  const point = canvasPoint(event);
  const current = currentAnnotation.value;
  if (current.tool === "pen" || current.tool === "mosaic") {
    currentAnnotation.value = { ...current, points: [...current.points, point] };
  } else if (current.tool === "rectangle" || current.tool === "ellipse" || current.tool === "arrow") {
    currentAnnotation.value = { ...current, end: point };
  }
  redraw();
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

function redraw(): void {
  const editor = editorCanvasRef.value;
  const source = baseCanvas;
  if (!editor || !source) {
    return;
  }
  const context = editor.getContext("2d");
  if (!context) {
    return;
  }
  context.clearRect(0, 0, editor.width, editor.height);
  context.drawImage(source, 0, 0);
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

function exportDataUrl(): string {
  redraw();
  return editorCanvasRef.value?.toDataURL("image/png") ?? "";
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

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
</script>

<template>
  <section class="absolute inset-0 z-30 select-none" data-testid="screenshot-editor" @contextmenu.prevent="emit('cancel')">
    <canvas
      ref="editorCanvasRef"
      class="absolute cursor-crosshair border-2 border-emerald-400 bg-white shadow-[0_0_0_9999px_rgba(2,6,23,0.42),0_0_0_1px_rgba(255,255,255,0.95)]"
      :style="editorStyle"
      @mousedown="onMouseDown"
    />

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
      class="absolute z-50 flex h-[52px] items-center gap-1 overflow-x-auto rounded-xl border border-slate-200 bg-white/98 px-2 shadow-[0_12px_36px_rgba(15,23,42,0.30)] backdrop-blur"
      :style="toolbarStyle"
      data-testid="screenshot-toolbar"
      @mousedown.stop
      @contextmenu.stop.prevent
    >
      <button
        v-for="item in toolButtons"
        :key="item.tool"
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition"
        :class="activeTool === item.tool ? 'bg-emerald-100 text-emerald-700' : 'text-slate-600 hover:bg-slate-100 hover:text-slate-950'"
        type="button"
        :title="item.label"
        :aria-label="item.label"
        :data-testid="`annotation-${item.tool}`"
        @click="activeTool = item.tool"
      >
        <component :is="item.icon" class="h-[18px] w-[18px]" aria-hidden="true" />
      </button>

      <span class="mx-1 h-6 w-px shrink-0 bg-slate-200" />
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
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-emerald-500 text-white shadow-sm transition hover:bg-emerald-600"
        type="button"
        title="完成并复制"
        aria-label="完成并复制"
        data-testid="screenshot-complete"
        @click="complete"
      >
        <Check class="h-5 w-5" aria-hidden="true" />
      </button>
    </div>
  </section>
</template>
