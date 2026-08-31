<script setup lang="ts">
import { computed } from "vue";
import {
  featureDescriptions,
  normalizeFeatures,
  type FeatureFlags,
} from "../utils/features";
const props = defineProps<{
  modelValue: FeatureFlags;
  locale: string;
  group?: "productivity" | "experiments";
}>();
const groups = computed(() =>
  props.group ? [props.group === "experiments"] : [false, true],
);
const emit = defineEmits<{ "update:modelValue": [value: FeatureFlags] }>();
const flags = computed(() => normalizeFeatures(props.modelValue));
function toggle(key: keyof FeatureFlags, event: Event) {
  emit("update:modelValue", {
    ...flags.value,
    [key]: (event.target as HTMLInputElement).checked,
  });
}
</script>
<template>
  <section class="settings-card" data-testid="extension-settings">
    <div
      v-for="(fun, index) in groups"
      :key="String(fun)"
      :class="
        index > 0
          ? 'mt-5 border-t border-slate-200 pt-4 dark:border-zinc-800'
          : ''
      "
    >
      <h2 class="settings-section-title">
        {{
          locale === "en"
            ? fun
              ? "Fun experiments"
              : "Productivity tools"
            : fun
              ? "趣味实验"
              : "效率工具"
        }}
      </h2>
      <p class="settings-section-description mt-1">
        {{
          locale === "en"
            ? fun
              ? "Off by default. Enable each tool when you need it."
              : "Translation still starts immediately on release."
            : fun
              ? "默认关闭，按需开启。关闭开关不会删除收藏。"
              : "不改变松手即翻译，只在主动操作时运行。"
        }}
      </p>
      <label
        v-for="item in featureDescriptions.filter(
          (f) => Boolean(f.fun) === fun,
        )"
        :key="item.key"
        class="flex cursor-pointer items-start justify-between gap-4 border-b border-slate-100 py-3 last:border-0 dark:border-zinc-800"
      >
        <span
          ><span class="block text-sm font-medium">{{
            locale === "en" ? item.en : item.zh
          }}</span
          ><span
            class="mt-1 block text-xs leading-5 text-slate-500 dark:text-slate-400"
            >{{ locale === "en" ? item.helpEn : item.help }}</span
          ></span
        >
        <input
          type="checkbox"
          role="switch"
          class="mt-1 h-4 w-4 shrink-0 accent-emerald-600"
          :checked="flags[item.key]"
          :aria-label="locale === 'en' ? item.en : item.zh"
          :data-testid="'feature-' + item.key"
          @change="toggle(item.key, $event)"
        />
      </label>
    </div>
    <p class="mt-3 text-xs leading-5 text-slate-500">
      {{
        locale === "en"
          ? "Images stay local. Explain, summarize, slang and generated learning notes send only text to your configured API."
          : "截图留在本机。解释、摘要、梗解释和生成学习笔记仅向已配置的 API 发送文字。"
      }}
    </p>
  </section>
</template>
