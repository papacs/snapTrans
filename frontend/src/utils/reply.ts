import type { Rect, Size } from "./selection";

export const replyScenes = [
  { id: "general", zh: "自然得体", en: "Everyday", hint: "回应重点，表达自然、有分寸", hintEn: "Warm, clear and appropriately bounded" },
  { id: "work", zh: "职场沟通", en: "Workplace", hint: "专业清晰，不替你轻易承诺", hintEn: "Professional and clear, without invented commitments" },
  { id: "social", zh: "社交评论", en: "Social", hint: "贴合内容，有观点、不尬聊", hintEn: "Relevant and thoughtful, with a natural voice" },
] as const;
export type ReplyScene = typeof replyScenes[number]["id"];

export function replyPanelPosition(anchor: Rect, viewport: Size) {
  const width = Math.min(440, Math.max(0, viewport.width - 32));
  const height = Math.min(620, Math.max(0, viewport.height - 32));
  const right = anchor.x + anchor.width + 16;
  const left = anchor.x - width - 16;
  const x = right + width <= viewport.width - 16 ? right : left >= 16 ? left : viewport.width - width - 16;
  return {
    left: Math.max(16, x),
    top: Math.max(16, Math.min(anchor.y, viewport.height - height - 16)),
    width, height,
  };
}
