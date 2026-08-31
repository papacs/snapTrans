<script setup lang="ts">
import {
  Bot,
  Cpu,
  Languages,
  Wrench,
  Sparkles,
  History,
} from "lucide-vue-next";
import { nextTick, ref } from "vue";
import {
  settingsSections,
  type SettingsSection,
} from "../utils/settings-navigation";
const props = defineProps<{ modelValue: SettingsSection; locale: string }>();
const emit = defineEmits<{ "update:modelValue": [section: SettingsSection] }>();
const navRef = ref<HTMLElement | null>(null);
const icons = {
  ai: Bot,
  capture: Cpu,
  translation: Languages,
  productivity: Wrench,
  experiments: Sparkles,
  library: History,
};
async function navigate(event: KeyboardEvent, index: number): Promise<void> {
  const steps: Record<string, number> = {
    ArrowRight: 1,
    ArrowLeft: -1,
    ArrowDown: 3,
    ArrowUp: -3,
  };
  let target: number;
  if (event.key === "Home") target = 0;
  else if (event.key === "End") target = settingsSections.length - 1;
  else if (event.key in steps)
    target =
      (index + steps[event.key]! + settingsSections.length) %
      settingsSections.length;
  else return;
  event.preventDefault();
  emit("update:modelValue", settingsSections[target]!.id);
  await nextTick();
  navRef.value
    ?.querySelectorAll<HTMLButtonElement>('[role="tab"]')
    [target]?.focus();
}
</script>
<template>
  <nav
    ref="navRef"
    role="tablist"
    :aria-label="locale === 'en' ? 'Settings categories' : '设置分类'"
    data-testid="settings-navigation"
    class="grid shrink-0 grid-cols-3 gap-2 border-b border-slate-200/80 bg-white/70 px-6 py-3 dark:border-zinc-800 dark:bg-zinc-950/70"
  >
    <button
      v-for="(item, index) in settingsSections"
      :key="item.id"
      type="button"
      role="tab"
      :id="'settings-tab-' + item.id"
      :aria-controls="'settings-panel-' + item.id"
      :aria-selected="modelValue === item.id"
      :tabindex="modelValue === item.id ? 0 : -1"
      :aria-label="locale === 'en' ? item.en : item.zh"
      :data-testid="'settings-tab-' + item.id"
      class="group flex min-w-0 items-center gap-2.5 rounded-xl border px-3 py-2.5 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/60"
      :class="
        modelValue === item.id
          ? 'border-emerald-500/60 bg-emerald-50 text-emerald-800 shadow-sm dark:border-emerald-500/50 dark:bg-emerald-500/10 dark:text-emerald-300'
          : 'border-slate-200/80 bg-white text-slate-600 hover:border-slate-300 hover:bg-slate-50 dark:border-zinc-800 dark:bg-zinc-900 dark:text-slate-400 dark:hover:border-zinc-700 dark:hover:bg-zinc-800'
      "
      @click="emit('update:modelValue', item.id)"
      @keydown="navigate($event, index)"
    >
      <component
        :is="icons[item.id]"
        class="h-4 w-4 shrink-0"
        aria-hidden="true"
      />
      <span class="min-w-0"
        ><span class="block text-xs font-semibold leading-4">{{
          locale === "en" ? item.en : item.zh
        }}</span>
        <span
          class="mt-1 hidden truncate text-[10px] leading-3 opacity-65 min-[560px]:block"
          >{{ locale === "en" ? item.hintEn : item.hint }}</span
        >
      </span>
    </button>
  </nav>
</template>
