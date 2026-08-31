import type { HistoryEntry } from "../services/backend";
import type { SettingsLocale } from "../i18n/settings";

// Only strip our bracketed wire indices; preserve ordinary numbered lists.
export function readableTranslation(text: string): string {
  const entries = new Map<number, string>();
  let current: number | null = null;
  for (const line of text.trim().split("\n")) {
    const match = line.match(/^\s*\[(\d+)\]\s*(.*)$/);
    if (match) { current = Number(match[1]); entries.set(current, match[2]!); }
    else if (current !== null) entries.set(current, entries.get(current) + "\n" + line);
    else if (line.trim()) return text.trim();
  }
  return entries.size ? [...entries].sort(([a], [b]) => a - b).map(([, value]) => value.trim()).join("\n") : text.trim();
}

export function bilingualHistoryText(entry: HistoryEntry, locale: SettingsLocale): string {
  const labels = locale === "en" ? ["Original", "Translation"] : ["原文", "译文"];
  return labels[0] + "：\n" + entry.source.trim() + "\n\n" + labels[1] + "：\n" + readableTranslation(entry.translation);
}

export function historyTimestamp(timestamp: string, locale: SettingsLocale): string {
  const date = new Date(timestamp);
  if (!Number.isFinite(date.getTime())) return locale === "en" ? "Unknown time" : "时间未知";
  return new Intl.DateTimeFormat(locale, {year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23"}).format(date);
}
