<script setup lang="ts">
import { computed } from "vue";
import { Check, Copy, LoaderCircle, RefreshCw, Table2, X } from "lucide-vue-next";

const props = defineProps<{
  cells: string[][];
  loading: boolean;
  error: string;
  copied: "tsv" | "markdown" | "";
  locale: string;
  panelStyle: Record<string, string>;
}>();
const emit = defineEmits<{
  "update:cells": [cells: string[][]];
  retry: [];
  close: [];
  copy: [format: "tsv" | "markdown"];
}>();
const en = computed(() => props.locale === "en");
const columns = computed(() => Math.max(0, ...props.cells.map((row) => row.length)));
function updateCell(rowIndex: number, columnIndex: number, value: string) {
  const cells = props.cells.map((row) => [...row]);
  cells[rowIndex] ??= [];
  cells[rowIndex]![columnIndex] = value;
  emit("update:cells", cells);
}
</script>

<template>
  <aside
    class="absolute z-[61] flex select-text flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_18px_48px_rgba(15,23,42,0.30)]"
    :style="panelStyle"
    data-testid="table-extraction-panel"
    @mousedown.stop
    @contextmenu.stop.prevent
  >
    <header class="flex items-center justify-between gap-3 border-b border-slate-200 px-4 py-3">
      <div class="min-w-0">
        <div class="flex items-center gap-2 text-sm font-semibold text-slate-900">
          <Table2 class="h-4 w-4 text-emerald-600" aria-hidden="true" />
          <span>{{ en ? "Extract table" : "提取表格" }}</span>
        </div>
        <p class="mt-0.5 text-[11px] text-slate-500">
          {{ en ? "Review and edit cells before copying" : "复制前可检查并编辑单元格" }}
        </p>
      </div>
      <div class="flex shrink-0 items-center gap-1">
        <button
          type="button"
          class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100 disabled:opacity-50"
          :disabled="loading"
          :aria-label="en ? 'Recognize again' : '重新识别表格'"
          data-testid="retry-table-extraction"
          @click="emit('retry')"
        >
          <LoaderCircle v-if="loading" class="h-4 w-4 animate-spin" />
          <RefreshCw v-else class="h-4 w-4" />
        </button>
        <button
          type="button"
          class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100"
          :aria-label="en ? 'Close table extraction' : '关闭表格提取'"
          @click="emit('close')"
        >
          <X class="h-4 w-4" />
        </button>
      </div>
    </header>

    <div v-if="error" role="alert" class="mx-4 mt-3 rounded-lg bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">
      {{ error }}
    </div>

    <div class="relative min-h-0 flex-1 overflow-auto p-4">
      <div v-if="cells.length" class="min-w-max overflow-hidden rounded-lg border border-slate-200">
        <table class="border-collapse text-xs text-slate-800" data-testid="table-preview">
          <tbody>
            <tr v-for="(row, rowIndex) in cells" :key="rowIndex">
              <td
                v-for="columnIndex in columns"
                :key="columnIndex"
                class="border border-slate-200 p-0"
              >
                <input
                  :value="row[columnIndex - 1] ?? ''"
                  class="h-9 w-32 bg-white px-2 outline-none focus:bg-emerald-50 focus:ring-2 focus:ring-inset focus:ring-emerald-500/30"
                  :aria-label="en ? `Row ${rowIndex + 1}, column ${columnIndex}` : `第 ${rowIndex + 1} 行，第 ${columnIndex} 列`"
                  :data-testid="`table-cell-${rowIndex}-${columnIndex - 1}`"
                  @input="updateCell(rowIndex, columnIndex - 1, ($event.target as HTMLInputElement).value)"
                  @keydown.escape.stop.prevent="emit('close')"
                />
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else-if="!loading && !error" class="text-sm text-slate-500">
        {{ en ? "No table cells found." : "未找到表格单元格。" }}
      </p>
      <div v-if="loading" class="absolute inset-4 flex flex-col items-center justify-center rounded-lg bg-white/90 text-slate-600 backdrop-blur-sm" data-testid="table-extraction-loading">
        <LoaderCircle class="h-8 w-8 animate-spin text-emerald-500" />
        <span class="mt-3 text-sm font-medium">{{ en ? "Recognizing table..." : "正在识别表格..." }}</span>
      </div>
    </div>

    <footer class="flex flex-wrap items-center justify-between gap-2 border-t border-slate-200 px-4 py-3">
      <span class="text-[11px] text-slate-500">
        {{ cells.length }} × {{ columns }} · {{ en ? "simple tables only" : "仅支持规则表格" }}
      </span>
      <div class="flex gap-2">
        <button
          v-for="format in (['tsv', 'markdown'] as const)"
          :key="format"
          type="button"
          class="settings-secondary-button"
          :disabled="loading || !cells.length"
          :data-testid="`copy-table-${format}`"
          @click="emit('copy', format)"
        >
          <Check v-if="copied === format" class="h-4 w-4" />
          <Copy v-else class="h-4 w-4" />
          {{ copied === format ? (en ? "Copied" : "已复制") : format === "tsv" ? "TSV" : "Markdown" }}
        </button>
      </div>
    </footer>
  </aside>
</template>
