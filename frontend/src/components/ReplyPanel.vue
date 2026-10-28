<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { Check, Copy, LoaderCircle, MessageSquare, RefreshCw, Square, X } from "lucide-vue-next";
import { cancelTextAction, copyText, onBackendEvent, startTextAction, type SelectionRegion, type TextActionEvent } from "../services/backend";
import { replyPanelPosition, replyScenes, type ReplyScene } from "../utils/reply";
import type { Rect, Size } from "../utils/selection";

const props = defineProps<{ source?: string; region?: SelectionRegion; anchor: Rect; viewport: Size; locale: string; directImageInput?: boolean }>();
const emit = defineEmits<{ close: []; copied: [] }>();
const en = computed(() => props.locale === "en");
const label = (zh: string, english: string) => en.value ? english : zh;
const remembered = sessionStorage.getItem("snaptrans.reply.scene");
const scene = ref<ReplyScene>(remembered === "work" || remembered === "social" ? remembered : "general");
const source = ref(props.source ?? ""), intent = ref(""), answer = ref("");
const inputMode = ref<"ocr" | "image">(props.directImageInput && props.region ? "image" : "ocr");
const previewImage = ref("");
const stage = ref<"ocr" | "image" | "writing" | "done">("ocr");
const busy = ref(false), error = ref(""), copied = ref(false);
const requestID = ref("");
const answerRef = ref<HTMLTextAreaElement | null>(null);
const generatedFor = ref("");
const signature = computed(() => JSON.stringify([source.value, scene.value, intent.value, inputMode.value]));
const dirty = computed(() => Boolean(answer.value) && generatedFor.value !== signature.value);
const selectedScene = computed(() => replyScenes.find(item => item.id === scene.value)!);
const style = computed(() => {
  const position = replyPanelPosition(props.anchor, props.viewport);
  return { left: position.left + "px", top: position.top + "px", width: position.width + "px", height: position.height + "px" };
});
let disposed = false, buffer = "", animation: number | null = null;
let off = () => {};
function flush() {
  if (animation !== null) { window.cancelAnimationFrame(animation); animation = null; }
  if (!buffer) return;
  const editor = answerRef.value;
  const follow = !editor || editor.scrollHeight - editor.scrollTop - editor.clientHeight < 48;
  answer.value += buffer;
  buffer = "";
  if (follow) void nextTick(() => { if (answerRef.value) answerRef.value.scrollTop = answerRef.value.scrollHeight; });
}
function queue(token: string) {
  buffer += token;
  if (animation === null) animation = window.requestAnimationFrame(flush);
}
async function generate() {
  if (busy.value) return;
  if (!source.value.trim() && !props.region) { error.value = label("请先提供需要回复的消息。", "Provide the message to reply to first."); return; }
  error.value = ""; answer.value = ""; copied.value = false; previewImage.value = "";
  busy.value = true; stage.value = inputMode.value === "image" ? "image" : source.value.trim() ? "writing" : "ocr";
  generatedFor.value = signature.value;
  const id = crypto.randomUUID();
  requestID.value = id;
  sessionStorage.setItem("snaptrans.reply.scene", scene.value);
  try {
    await startTextAction({ id, action: "reply", text: source.value, scene: scene.value, intent: intent.value, region: props.region, inputMode: inputMode.value });
  } catch (reason) {
    if (disposed || requestID.value !== id) return;
    error.value = String(reason instanceof Error ? reason.message : reason);
    busy.value = false; requestID.value = "";
  }
}
async function stop() {
  const id = requestID.value;
  requestID.value = "";
  flush(); busy.value = false;
  if (id) try { await cancelTextAction(id); } catch (reason) { error.value = String(reason); }
}
async function copy(close = false) {
  if (busy.value || dirty.value || !answer.value.trim()) return;
  try { await copyText(answer.value.trim()); copied.value = true; if (close && !disposed) emit("copied"); }
  catch (reason) { if (!disposed) error.value = String(reason instanceof Error ? reason.message : reason); }
}
function keydown(event: KeyboardEvent) {
  if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); emit("close"); }
  if ((event.ctrlKey || event.metaKey) && event.key === "Enter") { event.preventDefault(); void copy(true); }
}
function globalCopyShortcut(event: KeyboardEvent) {
  if (!event.defaultPrevented && (event.ctrlKey || event.metaKey) && event.key === "Enter") keydown(event);
}
onMounted(() => {
  void nextTick(() => answerRef.value?.focus({preventScroll:true}));
  window.addEventListener("keydown", globalCopyShortcut);
  off = onBackendEvent<TextActionEvent>("text-action", event => {
    if (!requestID.value || event.id !== requestID.value) return;
    if (event.source !== undefined) { source.value = event.source; generatedFor.value = signature.value; }
    if (event.image) previewImage.value = event.image;
    if (event.stage) stage.value = event.stage;
    if (event.token) queue(event.token);
    if (event.error) error.value = event.error;
    if (event.done) { flush(); busy.value = false; stage.value = "done"; requestID.value = ""; }
  });
  void generate();
});
onBeforeUnmount(() => {
  window.removeEventListener("keydown", globalCopyShortcut);
  disposed = true; off();
  if (animation !== null) window.cancelAnimationFrame(animation);
  if (requestID.value) void cancelTextAction(requestID.value).catch(() => {});
  requestID.value = "";
});
</script>

<template>
  <section data-reply-panel data-testid="reply-panel" role="dialog" :aria-label="label('回复助手', 'Reply assistant')"
    class="fixed z-[80] flex flex-col overflow-hidden rounded-2xl border border-emerald-200/80 bg-white text-slate-900 shadow-[0_20px_80px_rgba(15,23,42,0.28)] dark:border-emerald-900 dark:bg-zinc-950 dark:text-slate-100"
    :style="style" @mousedown.stop @contextmenu.stop @keydown="keydown">
    <header class="flex shrink-0 items-center justify-between border-b border-slate-100 bg-gradient-to-r from-emerald-50 to-white px-5 py-4 dark:border-zinc-800 dark:from-emerald-950/50 dark:to-zinc-950">
      <div class="flex items-center gap-3">
        <span class="rounded-xl bg-emerald-600 p-2 text-white"><MessageSquare class="h-4 w-4" /></span>
        <div><h2 class="text-sm font-semibold">{{ label('回复助手', 'Reply assistant') }}</h2><p class="mt-0.5 text-[11px] text-slate-500">{{ label('看懂上下文，表达你的意思', 'Your context. Your voice.') }}</p></div>
      </div>
      <button type="button" :aria-label="label('关闭回复助手', 'Close reply assistant')" class="rounded-lg p-2 text-slate-400 hover:bg-slate-100 dark:hover:bg-zinc-800" @click="emit('close')"><X class="h-4 w-4" /></button>
    </header>
    <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-5">
      <div class="grid grid-cols-3 gap-1 rounded-xl bg-slate-100 p-1 dark:bg-zinc-900" :aria-label="label('回复场景', 'Reply scenario')">
        <button v-for="item in replyScenes" :key="item.id" type="button" :aria-pressed="scene === item.id" :disabled="busy" :data-testid="'reply-scene-' + item.id"
          class="rounded-lg px-1 py-2 text-xs font-medium transition disabled:cursor-wait" :class="scene === item.id ? 'bg-white text-emerald-700 shadow-sm dark:bg-zinc-800 dark:text-emerald-300' : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'" @click="scene = item.id">{{ en ? item.en : item.zh }}</button>
      </div>
      <p class="text-[11px] text-slate-500">{{ en ? selectedScene.hintEn : selectedScene.hint }}</p>
      <label v-if="region" class="flex items-start gap-2 rounded-lg border border-slate-200 px-3 py-2 text-xs dark:border-zinc-800">
        <input v-model="inputMode" type="checkbox" true-value="image" false-value="ocr" data-testid="reply-image-mode" :disabled="busy" class="mt-0.5 accent-emerald-600" />
        <span>{{ label('直接看图，跳过 OCR', 'Read the screenshot directly, skip OCR') }}<span class="mt-1 block text-[10px] text-slate-400">{{ label('开启后会把选区截图发送给模型', 'Uploads the selected screenshot to your model') }}</span></span>
      </label>
      <label class="text-xs font-medium">{{ label('我想表达（可选）', 'My intent (optional)') }}
        <input v-model="intent" data-testid="reply-intent" type="text" maxlength="500" :disabled="busy" class="settings-input mt-1.5" :placeholder="label('例如：礼貌说明今天排期已满，请对方确认优先级', 'e.g. Politely ask which task should take priority')" />
      </label>
      <details class="rounded-xl border border-slate-200 px-3 py-2 dark:border-zinc-800">
        <summary class="cursor-pointer text-xs text-slate-500">{{ inputMode === 'image' ? label('截图上下文 · 可补充文字', 'Screenshot context · optional notes') : label('参考原文 · 可修正识别结果', 'Context · review and edit') }}</summary>
        <img v-if="inputMode === 'image' && previewImage" :src="previewImage" :alt="label('本次发送的选区截图', 'Selected screenshot sent for this reply')" class="mt-2 max-h-40 w-full rounded-lg object-contain" data-testid="reply-image-preview" />
        <textarea v-model="source" data-testid="reply-source" :disabled="busy" :aria-label="label('回复参考原文', 'Reply context')" class="settings-textarea mt-2 min-h-20 text-xs" />
      </details>
      <div class="flex items-center justify-between text-xs">
        <span class="font-medium">{{ label('建议回复', 'Suggested reply') }}</span>
        <span v-if="busy" role="status" class="inline-flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400"><LoaderCircle class="h-3.5 w-3.5 animate-spin" />{{ stage === 'ocr' ? label('正在识别文字…', 'Reading text…') : stage === 'image' ? label('正在准备截图…', 'Preparing image…') : inputMode === 'image' ? label('正在看图回复…', 'Drafting from image…') : label('正在生成回复…', 'Drafting…') }}</span>
        <span v-else class="text-slate-400">{{ label('可直接编辑', 'Editable') }}</span>
      </div>
      <textarea ref="answerRef" v-model="answer" data-testid="reply-answer" :readonly="busy" :aria-label="label('建议回复内容', 'Suggested reply text')"
        class="min-h-32 flex-1 resize-none rounded-xl border border-emerald-100 bg-emerald-50/40 p-4 text-sm leading-7 outline-none focus:ring-2 focus:ring-emerald-400/40 dark:border-zinc-800 dark:bg-zinc-900"
        :placeholder="busy ? label('回复即将出现在这里…', 'Your draft will appear here…') : label('调整意图后，点击重新生成。', 'Adjust your intent and generate again.')" />
      <p v-if="error" role="alert" class="break-words text-xs text-rose-600 dark:text-rose-400">{{ error }}</p>
      <p v-if="dirty" role="status" class="text-xs text-amber-700 dark:text-amber-400">{{ label('上下文或意图已修改，请重新生成。', 'Context or intent changed. Generate a fresh reply.') }}</p>
    </div>
    <footer class="shrink-0 border-t border-slate-100 px-5 py-3 dark:border-zinc-800">
      <div class="flex items-center justify-between gap-2">
        <button v-if="busy" type="button" class="settings-secondary-button" data-testid="reply-stop" @click="stop"><Square class="h-3 w-3" />{{ label('停止', 'Stop') }}</button>
        <button v-else type="button" class="settings-secondary-button" data-testid="reply-generate" @click="generate"><RefreshCw class="h-3.5 w-3.5" />{{ label('重新生成', 'Regenerate') }}</button>
        <div class="flex gap-2">
          <button type="button" class="settings-secondary-button" :disabled="busy || dirty || !answer.trim()" :aria-label="label('复制回复', 'Copy reply')" @click="copy()"><Check v-if="copied" class="h-3.5 w-3.5" /><Copy v-else class="h-3.5 w-3.5" /></button>
          <button type="button" class="settings-primary-button" data-testid="reply-copy-close" :disabled="busy || dirty || !answer.trim()" @click="copy(true)">{{ label('复制并收起', 'Copy & close') }}</button>
        </div>
      </div>
      <p class="mt-2 text-[10px] text-slate-400">{{ inputMode === 'image' ? label('上传选区截图 · 由你确认后使用 · Ctrl+Enter 复制收起', 'Selected image upload · Review before use · Ctrl+Enter to copy & close') : label('仅发送识别文字 · 由你确认后使用 · Ctrl+Enter 复制收起', 'Text only · Review before use · Ctrl+Enter to copy & close') }}</p>
    </footer>
  </section>
</template>
