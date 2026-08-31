export type SettingsSection =
  "ai" | "capture" | "translation" | "productivity" | "experiments" | "library";

export const settingsSections: Array<{
  id: SettingsSection;
  zh: string;
  en: string;
  hint: string;
  hintEn: string;
}> = [
  {
    id: "ai",
    zh: "AI 服务",
    en: "AI service",
    hint: "模型与连接",
    hintEn: "Model & connection",
  },
  {
    id: "capture",
    zh: "截图与 OCR",
    en: "Capture & OCR",
    hint: "快捷键与识别",
    hintEn: "Shortcuts & recognition",
  },
  {
    id: "translation",
    zh: "翻译设置",
    en: "Translation",
    hint: "方向、术语与输出",
    hintEn: "Language & output",
  },
  {
    id: "productivity",
    zh: "效率工具",
    en: "Productivity",
    hint: "提取、贴钉与隐私",
    hintEn: "Text, pins & privacy",
  },
  {
    id: "experiments",
    zh: "趣味实验",
    en: "Experiments",
    hint: "梗、卡片与对比",
    hintEn: "Slang, cards & compare",
  },
  {
    id: "library",
    zh: "历史与收藏",
    en: "History & saved",
    hint: "记录与学习卡片",
    hintEn: "History & learning cards",
  },
];
