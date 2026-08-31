import { describe, expect, it } from "vitest";
import { historyTimestamp, readableTranslation } from "./history";

describe("history formatting", () => {
 it("strips only internal indices, sorts missing-line retries, and keeps multiline content", () => {
  expect(readableTranslation("[1] 第一行\n延续\n[3] 第三行\n[2] 第二行")).toBe("第一行\n延续\n第二行\n第三行");
  expect(readableTranslation("1. A\n2. B")).toBe("1. A\n2. B");
  expect(readableTranslation("[1] 1. A\n[2] 2. B")).toBe("1. A\n2. B");
 });
 it("uses the newest correction for duplicated indices", () => {
  expect(readableTranslation("[1] 错误\n[1] 正确")).toBe("正确");
 });
 it("formats a timestamp in the user's local time zone, including seconds", () => {
  const input = "2026-08-31T10:20:30+08:00";
  const expected = new Intl.DateTimeFormat("zh-CN", {year:"numeric",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",second:"2-digit",hourCycle:"h23"}).format(new Date(input));
  expect(historyTimestamp(input, "zh-CN")).toBe(expected);
  expect(historyTimestamp("invalid", "zh-CN")).toBe("时间未知");
 });
});
