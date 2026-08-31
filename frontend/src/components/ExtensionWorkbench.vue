<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { X, LoaderCircle } from "lucide-vue-next";
import {
  cancelTextAction,
  copyImageDataUrl,
  copyText,
  extractText,
  onBackendEvent,
  pinImage,
  saveLearningCard,
  saveScreenshot,
  startTextAction,
  type TextAction,
  type TextActionEvent,
} from "../services/backend";
import { featureDescriptions, type FeatureFlags } from "../utils/features";
import {
  compareScreenshot,
  detectSensitiveBlocks,
  hasComparisonBaseline,
  imageCanvas,
  redactPreview,
  rememberComparison,
  shareCard,
  type RedactionCandidate,
} from "../utils/extensions";
import type { Rect } from "../utils/selection";
const props = defineProps<{
  features: FeatureFlags;
  locale: string;
  image: string;
  translatedImage?: string;
  source?: string;
  translation?: string;
  origin: { x: number; y: number };
  allowRedaction?: boolean;
}>();
const emit = defineEmits<{ close: []; pinned: []; redact: [rects: Rect[]] }>();
const dialogRef = ref<HTMLElement | null>(null);
const closeRef = ref<HTMLButtonElement | null>(null);
const tab = ref<keyof FeatureFlags | "">("");
const busy = ref(false),
  error = ref(""),
  notice = ref("");
const source = ref(props.source ?? ""),
  answer = ref(""),
  example = ref("");
const preview = ref(props.image),
  title = ref(""),
  bilingual = ref(false),
  meaning = ref(props.translation ?? "");
const candidates = ref<RedactionCandidate[]>([]),
  selected = ref<number[]>([]),
  baseline = ref(hasComparisonBaseline());
const activeRequest = ref("");
let sequence = 0,
  disposed = false;
const en = computed(() => props.locale === "en");
const label = (zh: string, english: string) => (en.value ? english : zh);
const tabs = computed(() =>
  featureDescriptions.filter(
    (f) =>
      props.features[f.key] &&
      !["historyTools", "textExtraction"].includes(f.key) &&
      (f.key !== "redaction" || props.allowRedaction),
  ),
);
const selectedRects = computed(() =>
  candidates.value
    .filter((c) => selected.value.includes(c.index))
    .map((c) => c.rect),
);
const usesText = computed(() =>
  ["textActions", "memeExplanation", "learningCards"].includes(tab.value),
);
const off = onBackendEvent<TextActionEvent>("text-action", (event) => {
  if (disposed || event.id !== activeRequest.value) return;
  if (event.token) answer.value += event.token;
  if (event.done) {
    busy.value = false;
    error.value = event.error ?? "";
    activeRequest.value = "";
    if (tab.value === "learningCards" && !event.error)
      meaning.value = answer.value;
  }
});
onMounted(async () => {
  tab.value = tabs.value[0]?.key ?? "";
  await nextTick();
  closeRef.value?.focus();
});
onBeforeUnmount(() => {
  disposed = true;
  sequence++;
  off();
  if (activeRequest.value)
    void cancelTextAction(activeRequest.value).catch(() => {});
});
function trapTab(event: KeyboardEvent) {
  const controls = Array.from(
    dialogRef.value?.querySelectorAll<HTMLElement>(
      'button:not(:disabled),input:not(:disabled),textarea:not(:disabled),select:not(:disabled),[tabindex="0"]',
    ) ?? [],
  ).filter((el) => el.getClientRects().length > 0);
  const first = controls[0],
    last = controls.at(-1);
  if (!first || !last) return;
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}
function choose(key: keyof FeatureFlags) {
  if (busy.value) return;
  tab.value = key;
  preview.value = props.image;
  error.value = "";
  notice.value = "";
  answer.value = "";
}
async function localTask(work: () => Promise<void>) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  notice.value = "";
  const current = ++sequence;
  try {
    await work();
  } catch (e) {
    if (!disposed && current === sequence)
      error.value = e instanceof Error ? e.message : String(e);
  } finally {
    if (!disposed && current === sequence) busy.value = false;
  }
}
async function ensureText() {
  if (!source.value.trim()) {
    const result = await extractText(props.image);
    if (disposed) return;
    source.value = result.text;
  }
  if (!source.value.trim())
    throw new Error(label("未识别到文字", "No text found"));
}
async function runAction(action: TextAction) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  notice.value = "";
  answer.value = "";
  const current = ++sequence;
  try {
    await ensureText();
    if (disposed || current !== sequence) return;
    activeRequest.value = crypto.randomUUID();
    await startTextAction({
      id: activeRequest.value,
      action,
      text: source.value,
    });
  } catch (e) {
    if (!disposed && current === sequence) {
      error.value = e instanceof Error ? e.message : String(e);
      busy.value = false;
      activeRequest.value = "";
    }
  }
}
async function stop() {
  sequence++;
  const id = activeRequest.value;
  activeRequest.value = "";
  busy.value = false;
  notice.value = label(
    "已停止，保留已返回内容",
    "Stopped; partial output retained",
  );
  if (id)
    try {
      await cancelTextAction(id);
    } catch (e) {
      error.value = String(e);
    }
}
async function pin(translated = false) {
  await localTask(async () => {
    await pinImage(
      translated ? props.translatedImage! : props.image,
      props.origin.x,
      props.origin.y,
    );
    if (!disposed) emit("pinned");
  });
}
async function scanSensitive() {
  await localTask(async () => {
    const result = await extractText(props.image),
      canvas = await imageCanvas(props.image);
    if (disposed) return;
    candidates.value = detectSensitiveBlocks(
      result.blocks,
      canvas.width,
      canvas.height,
    );
    selected.value = candidates.value.map((c) => c.index);
    notice.value = candidates.value.length
      ? label(
          "请检查候选区域；应用后仍可撤销。",
          "Review candidates; applying can be undone.",
        )
      : label(
          "没有匹配项不代表没有隐私信息，请自行检查。",
          "No matches does not guarantee there is no private information.",
        );
    preview.value = await redactPreview(props.image, selectedRects.value);
  });
}
async function previewRedaction() {
  await localTask(async () => {
    const image = await redactPreview(props.image, selectedRects.value);
    if (!disposed) preview.value = image;
  });
}
async function makeCard() {
  await localTask(async () => {
    if (bilingual.value) await ensureText();
    if (disposed) return;
    const image = await shareCard(
      props.image,
      title.value,
      source.value,
      props.translation ?? "",
      bilingual.value,
    );
    if (!disposed) preview.value = image;
  });
}
async function remember() {
  await localTask(async () => {
    await rememberComparison(props.image);
    if (disposed) return;
    baseline.value = true;
    notice.value = label(
      "已记住基准。关闭本次截图后，在相同位置截第二张，再点比较。",
      "Baseline saved in memory. Capture the same region again, then compare.",
    );
  });
}
async function compare() {
  await localTask(async () => {
    const result = await compareScreenshot(props.image);
    if (disposed) return;
    preview.value = result.image;
    notice.value =
      label("变化像素：", "Changed pixels: ") +
      result.percent.toFixed(2) +
      label(
        "%；红色表示变化，不进行自动对齐。",
        "%; red marks changes. No automatic alignment.",
      );
  });
}
async function saveCard() {
  await localTask(async () => {
    await saveLearningCard(source.value, meaning.value, example.value);
    if (!disposed)
      notice.value = label(
        "已保存到设置里的学习卡片",
        "Saved to learning cards in Settings",
      );
  });
}
async function copyOutput() {
  await localTask(async () => {
    await copyText(answer.value);
    if (!disposed) notice.value = label("已复制", "Copied");
  });
}
async function copyPreview() {
  await localTask(async () => {
    await copyImageDataUrl(preview.value);
    if (!disposed) notice.value = label("已复制图片", "Image copied");
  });
}
async function savePreview() {
  await localTask(async () => {
    const path = await saveScreenshot(preview.value);
    if (!disposed)
      notice.value = path ? label("已保存图片", "Image saved") : "";
  });
}
</script>
<template>
  <div
    data-extension-workbench
    class="fixed inset-0 z-[90] flex items-center justify-center bg-slate-950/35 p-4"
    @mousedown.stop
    @contextmenu.stop.prevent
    @keydown.esc.stop.prevent="emit('close')"
  >
    <section
      ref="dialogRef"
      @keydown.tab="trapTab"
      role="dialog"
      aria-modal="true"
      :aria-label="label('扩展工具', 'Extension tools')"
      class="flex max-h-[calc(100vh-32px)] w-[760px] max-w-full flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white text-slate-900 shadow-2xl dark:border-zinc-700 dark:bg-zinc-900 dark:text-slate-100"
    >
      <header
        class="flex shrink-0 items-center justify-between border-b border-slate-100 px-5 py-4 dark:border-zinc-800"
      >
        <div>
          <h2 class="text-base font-semibold">
            {{ label("扩展工具", "Extension tools") }}
          </h2>
          <p class="mt-1 text-xs text-slate-500">
            {{
              label("按需使用 · 截图留在本机", "On demand · Images stay local")
            }}
          </p>
        </div>
        <button
          ref="closeRef"
          type="button"
          class="rounded p-2 hover:bg-slate-100 dark:hover:bg-zinc-800"
          :aria-label="label('关闭扩展工具', 'Close extension tools')"
          @click="emit('close')"
        >
          <X class="h-5 w-5" />
        </button>
      </header>
      <nav
        class="flex shrink-0 flex-wrap gap-2 border-b border-slate-100 px-5 py-3 dark:border-zinc-800"
        :aria-label="label('工具', 'Tools')"
      >
        <button
          v-for="item in tabs"
          :key="item.key"
          type="button"
          :disabled="busy"
          class="rounded-lg px-3 py-2 text-xs font-medium disabled:opacity-50"
          :class="
            tab === item.key
              ? 'bg-emerald-600 text-white'
              : 'bg-slate-100 text-slate-600 dark:bg-zinc-800 dark:text-slate-300'
          "
          :data-testid="'extension-tab-' + item.key"
          :aria-pressed="tab === item.key"
          @click="choose(item.key)"
        >
          {{ en ? item.en : item.zh }}
        </button>
      </nav>
      <div class="overflow-y-auto p-5 text-sm">
        <div v-if="!tabs.length" class="text-slate-500">
          {{ label("请在设置中开启扩展功能。", "Enable tools in Settings.") }}
        </div>
        <template v-if="usesText">
          <label class="block text-xs text-slate-500"
            >{{
              label(
                "原文（可编辑；留空时点击操作将运行本地 OCR）",
                "Original (editable; actions run local OCR when empty)",
              )
            }}<textarea
              v-model="source"
              :disabled="busy"
              class="settings-textarea mt-2 min-h-24"
              :aria-label="label('操作原文', 'Action source')"
            />
          </label>
          <p class="my-3 text-xs leading-5 text-slate-500">
            {{
              label(
                "点击解释、摘要或生成笔记会把上方文字发送给已配置的模型。结果可能有误，请核对。",
                "Actions send the text above to your configured model. Results may be incorrect; verify them.",
              )
            }}
          </p>
          <div class="flex flex-wrap gap-2">
            <template v-if="tab === 'textActions'"
              ><button
                class="settings-primary-button"
                type="button"
                :disabled="busy"
                @click="runAction('explain')"
              >
                {{ label("解释一下", "Explain") }}</button
              ><button
                class="settings-secondary-button"
                type="button"
                :disabled="busy"
                @click="runAction('summarize')"
              >
                {{ label("三句话摘要", "Summarize") }}
              </button></template
            >
            <button
              v-if="tab === 'memeExplanation'"
              class="settings-primary-button"
              type="button"
              :disabled="busy"
              @click="runAction('meme')"
            >
              {{ label("这是什么梗？", "Explain this expression") }}
            </button>
            <button
              v-if="tab === 'learningCards'"
              class="settings-secondary-button"
              type="button"
              :disabled="busy"
              @click="runAction('learning')"
            >
              {{ label("生成释义与例句", "Generate meaning & example") }}
            </button>
            <button
              v-if="activeRequest"
              class="settings-secondary-button"
              type="button"
              @click="stop"
            >
              {{ label("停止", "Stop") }}
            </button>
            <button
              v-if="answer"
              class="settings-secondary-button"
              type="button"
              :disabled="busy"
              @click="copyOutput"
            >
              {{ label("复制结果", "Copy result") }}
            </button>
          </div>
          <pre
            v-if="answer && tab !== 'learningCards'"
            class="mt-4 whitespace-pre-wrap break-words rounded-xl bg-slate-50 p-4 font-sans text-sm leading-7 dark:bg-zinc-800"
            data-testid="text-action-answer"
            >{{ answer }}</pre>
          <template v-if="tab === 'learningCards'">
            <label class="mt-4 block text-xs text-slate-500"
              >{{
                label(
                  "释义／学习笔记（可编辑）",
                  "Meaning / learning note (editable)",
                )
              }}<textarea
                v-model="meaning"
                :disabled="busy"
                class="settings-textarea mt-2 min-h-32"
                :aria-label="label('卡片释义', 'Card meaning')"
              />
            </label>
            <label class="mt-3 block text-xs text-slate-500"
              >{{ label("补充例句（可选）", "Additional example (optional)")
              }}<textarea
                v-model="example"
                :disabled="busy"
                class="settings-textarea mt-2"
              />
            </label>
            <p v-if="busy && answer" class="mt-2 whitespace-pre-wrap text-sm">
              {{ answer }}
            </p>
            <button
              type="button"
              class="settings-primary-button mt-3"
              :disabled="busy || !source.trim() || !meaning.trim()"
              @click="saveCard"
            >
              {{ label("保存学习卡片", "Save learning card") }}
            </button>
          </template>
        </template>
        <template v-else-if="tab === 'pin'">
          <p class="mb-4 text-sm leading-6 text-slate-500">
            {{
              label(
                "拖动移动，滚轮缩放，Ctrl+滚轮调整透明度。右键开启鼠标穿透后，可从托盘恢复交互或关闭所有贴钉。",
                "Drag to move, wheel to zoom, Ctrl+wheel for opacity. Right-click enables click-through. Use the tray to unlock or close all pins.",
              )
            }}
          </p>
          <div class="flex gap-2">
            <button
              class="settings-primary-button"
              type="button"
              :disabled="busy"
              @click="pin()"
            >
              {{ label("贴钉当前截图", "Pin screenshot") }}</button
            ><button
              v-if="translatedImage"
              class="settings-secondary-button"
              type="button"
              :disabled="busy"
              @click="pin(true)"
            >
              {{ label("贴钉译后图片", "Pin translated image") }}
            </button>
          </div>
        </template>
        <template v-else-if="tab === 'redaction'">
          <p class="mb-3 text-xs leading-6 text-slate-500">
            {{
              label(
                "只检测邮箱与常见电话，可能漏检。为避免猜测字符位置，将遮挡命中的整段 OCR 文本块。请检查后再分享。",
                "Detects emails and common phone numbers only and may miss information. Covers the whole matching OCR block. Review before sharing.",
              )
            }}
          </p>
          <button
            type="button"
            class="settings-primary-button"
            :disabled="busy"
            @click="scanSensitive"
          >
            {{ label("本地检测", "Detect locally") }}
          </button>
          <label
            v-for="item in candidates"
            :key="item.index"
            class="mt-3 flex items-start gap-2 break-all text-xs"
            ><input
              v-model="selected"
              type="checkbox"
              :value="item.index"
              :disabled="busy"
              @change="previewRedaction"
            /><span>{{ item.text }}</span></label
          >
          <button
            v-if="candidates.length"
            type="button"
            class="settings-secondary-button mt-3"
            :disabled="busy || !selectedRects.length"
            @click="emit('redact', selectedRects)"
          >
            {{ label("应用遮挡（可撤销）", "Apply covers (undoable)") }}
          </button>
        </template>
        <template v-else-if="tab === 'shareCards'">
          <label class="block text-xs text-slate-500"
            >{{ label("卡片标题", "Card title")
            }}<input
              v-model="title"
              maxlength="120"
              class="settings-input mt-2"
              :disabled="busy"
          /></label>
          <label v-if="translation" class="my-3 flex gap-2 text-sm"
            ><input v-model="bilingual" type="checkbox" :disabled="busy" />{{
              label("制作原文＋译文双语卡片", "Make a bilingual card")
            }}</label
          >
          <button
            type="button"
            class="settings-primary-button mt-3"
            :disabled="busy"
            @click="makeCard"
          >
            {{ label("生成卡片", "Create card") }}
          </button>
        </template>
        <template v-else-if="tab === 'imageCompare'">
          <p class="mb-3 text-xs leading-6 text-slate-500">
            {{
              label(
                "只比较同尺寸截图的像素变化，不自动对齐。基准仅保留在内存，关闭程序或关闭此功能后清除。",
                "Compares pixels in equal-sized images without alignment. Baseline is cleared on exit or when this feature is disabled.",
              )
            }}
          </p>
          <div class="flex gap-2">
            <button
              type="button"
              class="settings-secondary-button"
              :disabled="busy"
              @click="remember"
            >
              {{
                label(
                  baseline ? "替换基准图" : "记住基准图",
                  baseline ? "Replace baseline" : "Remember baseline",
                )
              }}</button
            ><button
              type="button"
              class="settings-primary-button"
              :disabled="busy || !baseline"
              @click="compare"
            >
              {{ label("与基准比较", "Compare with baseline") }}
            </button>
          </div>
        </template>
        <div
          v-if="busy"
          role="status"
          class="mt-4 flex items-center gap-2 text-xs text-emerald-600"
        >
          <LoaderCircle class="h-4 w-4 animate-spin" />{{
            label("正在处理…", "Working…")
          }}
        </div>
        <p
          v-if="error"
          role="alert"
          class="mt-4 break-words rounded-lg bg-rose-50 p-3 text-xs leading-6 text-rose-700"
        >
          {{ error }}
        </p>
        <p
          v-if="notice"
          role="status"
          class="mt-4 rounded-lg bg-emerald-50 p-3 text-xs leading-6 text-emerald-800"
        >
          {{ notice }}
        </p>
        <template v-if="!usesText && tab && preview">
          <div
            class="mt-4 max-h-[45vh] overflow-auto rounded-lg border border-slate-200 bg-slate-100 dark:border-zinc-700 dark:bg-zinc-800"
          >
            <img
              :src="preview"
              class="mx-auto block max-w-full"
              :alt="label('处理结果预览', 'Result preview')"
            />
          </div>
          <div
            v-if="tab === 'shareCards' || tab === 'imageCompare'"
            class="mt-3 flex gap-2"
          >
            <button
              type="button"
              class="settings-secondary-button"
              :disabled="busy"
              @click="copyPreview"
            >
              {{ label("复制图片", "Copy image") }}</button
            ><button
              type="button"
              class="settings-secondary-button"
              :disabled="busy"
              @click="savePreview"
            >
              {{ label("保存 PNG", "Save PNG") }}
            </button>
          </div>
        </template>
      </div>
    </section>
  </div>
</template>
