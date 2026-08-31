<script setup lang="ts">
import { computed, ref } from "vue";
import {
  deleteSavedEntry,
  exportMarkdown,
  type HistoryEntry,
} from "../services/backend";
import { filterLibrary, libraryMarkdown } from "../utils/extensions";
const props = defineProps<{ entries: HistoryEntry[]; locale: string }>();
const emit = defineEmits<{ changed: [] }>();
const query = ref(""),
  notice = ref(""),
  pending = ref(false),
  deleteID = ref("");
const cards = computed(() =>
  filterLibrary(props.entries, query.value, "learning"),
);
async function exportCards() {
  pending.value = true;
  try {
    const path = await exportMarkdown(libraryMarkdown(cards.value));
    notice.value = path ? (props.locale === "en" ? "Exported" : "已导出") : "";
  } catch (e) {
    notice.value = String(e);
  } finally {
    pending.value = false;
  }
}
async function remove(id: string) {
  pending.value = true;
  try {
    await deleteSavedEntry(id);
    deleteID.value = "";
    emit("changed");
  } catch (e) {
    notice.value = String(e);
  } finally {
    pending.value = false;
  }
}
</script>
<template>
  <section class="settings-card" data-testid="learning-library">
    <div class="flex items-center justify-between gap-3">
      <h2 class="settings-section-title">
        {{ locale === "en" ? "Learning cards" : "学习卡片" }}
      </h2>
      <button
        class="settings-secondary-button"
        type="button"
        :disabled="pending || !cards.length"
        @click="exportCards"
      >
        {{ locale === "en" ? "Export Markdown" : "导出 Markdown" }}
      </button>
    </div>
    <input
      v-model="query"
      type="search"
      class="settings-input mt-3"
      :aria-label="locale === 'en' ? 'Search learning cards' : '搜索学习卡片'"
      :placeholder="
        locale === 'en'
          ? 'Search original, meaning or example'
          : '搜索原句、释义、例句'
      "
    />
    <p v-if="!cards.length" class="mt-3 text-xs text-slate-500">
      {{
        locale === "en"
          ? "No matching cards. Save one from Extension tools."
          : "暂无匹配卡片。可以在扩展工具中保存学习卡片。"
      }}
    </p>
    <div class="mt-3 max-h-80 space-y-3 overflow-auto">
      <article
        v-for="card in cards"
        :key="card.id"
        class="rounded-xl border border-slate-200 p-3 text-sm dark:border-zinc-700"
      >
        <p class="whitespace-pre-wrap break-words font-medium">
          {{ card.source }}
        </p>
        <p
          class="mt-2 whitespace-pre-wrap break-words leading-6 text-slate-500"
        >
          {{ card.translation }}
        </p>
        <p
          v-if="card.example"
          class="mt-2 whitespace-pre-wrap break-words text-emerald-700 dark:text-emerald-300"
        >
          {{ card.example }}
        </p>
        <button
          v-if="deleteID !== card.id"
          type="button"
          class="settings-tertiary-button mt-2"
          :disabled="pending"
          @click="deleteID = card.id"
        >
          {{ locale === "en" ? "Delete" : "删除" }}
        </button>
        <div v-else class="mt-2 flex items-center gap-2 text-xs">
          <span>{{
            locale === "en" ? "Delete this saved card?" : "删除这张已保存卡片？"
          }}</span
          ><button
            type="button"
            class="settings-secondary-button"
            :disabled="pending"
            @click="remove(card.id)"
          >
            {{ locale === "en" ? "Delete" : "确认删除" }}</button
          ><button
            type="button"
            class="settings-tertiary-button"
            @click="deleteID = ''"
          >
            {{ locale === "en" ? "Cancel" : "取消" }}
          </button>
        </div>
      </article>
    </div>
    <p v-if="notice" role="status" class="mt-3 break-words text-xs">
      {{ notice }}
    </p>
  </section>
</template>
