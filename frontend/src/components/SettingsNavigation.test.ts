import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import SettingsNavigation from "./SettingsNavigation.vue";

describe("settings category navigation", () => {
  it("links each tab to its panel and uses one keyboard tab stop", () => {
    const wrapper = mount(SettingsNavigation, {
      props: { modelValue: "ai", locale: "zh-CN" },
    });
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(6);
    expect(wrapper.findAll('[tabindex="0"]')).toHaveLength(1);
    for (const tab of wrapper.findAll('[role="tab"]')) {
      expect(tab.attributes("aria-controls")).toBe(
        tab.attributes("id")!.replace("settings-tab-", "settings-panel-"),
      );
    }
  });
  it("supports arrow keys, Home and End without submitting the settings form", async () => {
    const wrapper = mount(SettingsNavigation, {
      props: { modelValue: "ai", locale: "en" },
    });
    const first = wrapper.get('[data-testid="settings-tab-ai"]');
    await first.trigger("keydown", { key: "ArrowDown" });
    await first.trigger("keydown", { key: "ArrowLeft" });
    await first.trigger("keydown", { key: "End" });
    await wrapper
      .get('[data-testid="settings-tab-library"]')
      .trigger("keydown", { key: "Home" });
    expect(wrapper.emitted("update:modelValue")).toEqual([
      ["productivity"],
      ["library"],
      ["library"],
      ["ai"],
    ]);
    expect(first.attributes("type")).toBe("button");
    await wrapper.setProps({ modelValue: "library", locale: "zh-CN" });
    expect(
      wrapper
        .get('[data-testid="settings-tab-library"]')
        .attributes("aria-selected"),
    ).toBe("true");
    expect(
      wrapper
        .get('[data-testid="settings-tab-library"]')
        .attributes("aria-label"),
    ).toBe("历史与收藏");
  });
});
