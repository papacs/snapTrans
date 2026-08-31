export interface FeatureFlags {
  textExtraction: boolean;
  pin: boolean;
  textActions: boolean;
  redaction: boolean;
  historyTools: boolean;
  memeExplanation: boolean;
  learningCards: boolean;
  shareCards: boolean;
  imageCompare: boolean;
}
export const defaultFeatures: FeatureFlags = {
  textExtraction: true,
  pin: true,
  textActions: true,
  redaction: true,
  historyTools: true,
  memeExplanation: false,
  learningCards: false,
  shareCards: false,
  imageCompare: false,
};
export function normalizeFeatures(
  value?: Partial<FeatureFlags> | null,
): FeatureFlags {
  const result = { ...defaultFeatures };
  for (const key of Object.keys(result) as (keyof FeatureFlags)[]) {
    if (typeof value?.[key] === "boolean") result[key] = value[key]!;
  }
  return result;
}
export const featureDescriptions: Array<{
  key: keyof FeatureFlags;
  zh: string;
  en: string;
  help: string;
  helpEn: string;
  fun?: boolean;
}> = [
  {
    key: "textExtraction",
    zh: "提取文字",
    en: "Extract text",
    help: "仅本地 OCR；编辑原文、保留换行或合并段落。",
    helpEn: "Local OCR, editable text and line-break controls.",
  },
  {
    key: "pin",
    zh: "截图与译文贴钉",
    en: "Desktop pins",
    help: "独立置顶小窗；滚轮缩放、Ctrl+滚轮调透明度，右键更多操作。",
    helpEn:
      "Topmost windows; wheel to zoom, Ctrl+wheel for opacity, right-click for options.",
  },
  {
    key: "textActions",
    zh: "解释与摘要",
    en: "Explain & summarize",
    help: "仅点击后发送选区文字，使用当前模型配置。",
    helpEn: "Sends selected text to your configured model only when clicked.",
  },
  {
    key: "redaction",
    zh: "隐私遮挡助手",
    en: "Redaction assistant",
    help: "本地检测邮箱和常见电话号码，检查候选项后用实心色块遮挡。",
    helpEn:
      "Locally detect emails and common phone numbers; review before covering.",
  },
  {
    key: "historyTools",
    zh: "历史搜索与收藏",
    en: "Search & favorites",
    help: "收藏不会被最近 50 条记录淘汰；关闭开关不删除数据。",
    helpEn:
      "Favorites survive the 50-entry recent limit. Disabling does not delete them.",
  },
  {
    key: "memeExplanation",
    zh: "这是什么梗？",
    en: "Explain slang & memes",
    help: "只分析文字梗和俚语，不上传图片；背景不足时可能无法解释。",
    helpEn: "Text only, no image uploads; context may be insufficient.",
    fun: true,
  },
  {
    key: "learningCards",
    zh: "学习卡片",
    en: "Learning cards",
    help: "编辑原句、释义与例句，本地收藏并导出 Markdown；生成例句需调用模型。",
    helpEn:
      "Edit and save locally; export Markdown. Generating examples uses your model.",
    fun: true,
  },
  {
    key: "shareCards",
    zh: "分享卡片",
    en: "Share cards",
    help: "本地生成带标题、留白和背景的截图或双语 PNG。",
    helpEn: "Locally render a framed screenshot or bilingual PNG.",
    fun: true,
  },
  {
    key: "imageCompare",
    zh: "截图找不同",
    en: "Compare screenshots",
    help: "手动记住基准图，再比较同尺寸截图；基准仅存于本次运行内存。",
    helpEn:
      "Remember a baseline, then compare equal-sized captures. Memory only.",
    fun: true,
  },
];
