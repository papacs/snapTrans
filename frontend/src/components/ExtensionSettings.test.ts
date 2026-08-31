import { mount } from "@vue/test-utils";
import { describe, it, expect } from "vitest";
import ExtensionSettings from "./ExtensionSettings.vue";
import { defaultFeatures } from "../utils/features";
describe("extension settings", () => {
  it("separates productivity and experiments without exposing duplicate controls", () => {
    const productivity = mount(ExtensionSettings, {
      props: {
        modelValue: defaultFeatures,
        locale: "zh-CN",
        group: "productivity",
      },
    });
    const experiments = mount(ExtensionSettings, {
      props: {
        modelValue: defaultFeatures,
        locale: "zh-CN",
        group: "experiments",
      },
    });
    expect(productivity.findAll('[role="switch"]')).toHaveLength(5);
    expect(
      productivity.find('[data-testid="feature-shareCards"]').exists(),
    ).toBe(false);
    expect(experiments.findAll('[role="switch"]')).toHaveLength(4);
    expect(experiments.find('[data-testid="feature-pin"]').exists()).toBe(
      false,
    );
  });
  it("emits a new feature object and never mutates the saved draft", async () => {
    const saved = { ...defaultFeatures };
    const wrapper = mount(ExtensionSettings, {
      props: { modelValue: saved, locale: "zh-CN" },
    });
    await wrapper.get('[data-testid="feature-shareCards"]').setValue(true);
    expect(saved.shareCards).toBe(false);
    expect(wrapper.emitted("update:modelValue")?.[0]?.[0]).toEqual({
      ...defaultFeatures,
      shareCards: true,
    });
  });
  it("keeps experiments individually switchable", async () => {
    const wrapper = mount(ExtensionSettings, {
      props: {
        modelValue: { ...defaultFeatures, memeExplanation: true },
        locale: "en",
      },
    });
    expect(
      (
        wrapper.get('[data-testid="feature-memeExplanation"]')
          .element as HTMLInputElement
      ).checked,
    ).toBe(true);
    expect(
      (
        wrapper.get('[data-testid="feature-learningCards"]')
          .element as HTMLInputElement
      ).checked,
    ).toBe(false);
  });
});
