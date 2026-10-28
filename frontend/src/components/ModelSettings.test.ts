import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { reactive } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ModelSettings from "./ModelSettings.vue";
import { cancelCodexLogin, defaultConfig, getCodexStatus, loginCodex, type CodexStatus } from "../services/backend";

vi.mock("../services/backend", async (original) => ({
  ...await original<typeof import("../services/backend")>(),
  getCodexStatus: vi.fn(), loginCodex: vi.fn(), cancelCodexLogin: vi.fn(async () => {}),
}));
enableAutoUnmount(afterEach);
const connected: CodexStatus = { authenticated: true, email: "test@example.com", plan: "plus", models: [{id: "available-model", name: "Available model", isDefault: true}] };
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getCodexStatus).mockResolvedValue(connected);
  vi.mocked(loginCodex).mockResolvedValue();
});

describe("model settings", () => {
  it("provides a translation-only speed setting without changing reply thinking", async () => {
    const config = reactive({...defaultConfig,provider:"codex" as const,codexReasoningEffort:"high" as const});
    const wrapper = mount(ModelSettings,{props:{config,locale:"zh-CN",active:true}});
    await flushPromises();
    expect((wrapper.get('[data-testid="fast-translation"]').element as HTMLInputElement).checked).toBe(true);
    await wrapper.get('[data-testid="fast-translation"]').setValue(false);
    expect(config.fastTranslation).toBe(false);
    expect(config.codexReasoningEffort).toBe("high");
    expect(wrapper.text()).toContain("回复仍使用上方强度");
  });
  it("keeps direct image input opt-in across providers", async () => {
    const config = reactive({...defaultConfig});
    const wrapper = mount(ModelSettings, {props: {config, locale: "zh-CN", active: true}});
    expect(config.directImageInput).toBe(false);
    await wrapper.get('[data-testid="direct-image-input"]').setValue(true);
    expect(config.directImageInput).toBe(true);
    await wrapper.get('[data-testid="model-provider"]').setValue("codex");
    await flushPromises();
    expect((wrapper.get('[data-testid="direct-image-input"]').element as HTMLInputElement).checked).toBe(true);
    expect(wrapper.text()).toContain("上传选区截图");
  });
  it("switches connections without losing API configuration or validating hidden API fields", async () => {
    const config = reactive({...defaultConfig, apiKey: "saved-key", baseURL: "unfinished-url", codexModel: "chosen-model"});
    const wrapper = mount(ModelSettings, {props: {config, locale: "zh-CN", active: true}});
    await wrapper.get('[data-testid="model-provider"]').setValue("codex");
    await flushPromises();
    expect(wrapper.find('input[type="url"]').exists()).toBe(false);
    expect(wrapper.get('[data-testid="codex-status"]').text()).toContain("订阅已接入");
    expect(wrapper.get("datalist option").attributes("value")).toBe("available-model");
    await wrapper.get('[data-testid="codex-model"]').setValue("available-model");
    await wrapper.get('[data-testid="model-provider"]').setValue("openai-compatible");
    expect(config.apiKey).toBe("saved-key");
    expect(config.baseURL).toBe("unfinished-url");
    expect(config.codexModel).toBe("available-model");
  });

  it("refreshes models after browser sign-in using the draft CLI path", async () => {
    vi.mocked(getCodexStatus).mockResolvedValueOnce({...connected, authenticated: false, models: []});
    const config = reactive({...defaultConfig, provider: "codex" as const, codexPath: "C:/Tools/codex.exe"});
    const wrapper = mount(ModelSettings, {props: {config, locale: "en", active: true}});
    await flushPromises();
    expect(wrapper.text()).toContain("No ChatGPT sign-in");
    await wrapper.get('[data-testid="codex-login"]').trigger("click");
    await flushPromises();
    expect(loginCodex).toHaveBeenCalledWith(config.codexPath);
    expect(getCodexStatus).toHaveBeenLastCalledWith(config.codexPath);
    expect(wrapper.text()).toContain("Subscription connected");
  });

  it("discards a detection result when its path has changed", async () => {
    let resolve!: (status: CodexStatus) => void;
    vi.mocked(getCodexStatus).mockImplementationOnce(() => new Promise(done => {resolve = done;}));
    const config = reactive({...defaultConfig, provider: "codex" as const});
    const wrapper = mount(ModelSettings, {props: {config, locale: "en", active: true}});
    await wrapper.get('[data-testid="codex-path"]').setValue("other.exe");
    resolve(connected);
    await flushPromises();
    expect(wrapper.find('[data-testid="codex-status"]').exists()).toBe(false);
    expect(wrapper.findAll("datalist option")).toHaveLength(0);
  });

  it("cancels a pending sign-in when settings close and ignores its late result", async () => {
    let resolve!: () => void;
    vi.mocked(loginCodex).mockImplementationOnce(() => new Promise(done => {resolve = done;}));
    const config = reactive({...defaultConfig, provider: "codex" as const});
    const wrapper = mount(ModelSettings, {props: {config, locale: "en", active: true}});
    await flushPromises();
    await wrapper.get('[data-testid="codex-login"]').trigger("click");
    expect(wrapper.get('[data-testid="model-provider"]').attributes("disabled")).toBeDefined();
    await wrapper.setProps({active: false});
    expect(cancelCodexLogin).toHaveBeenCalledOnce();
    resolve();
    await flushPromises();
    expect(getCodexStatus).toHaveBeenCalledOnce();
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
  });

  it("leaves detection available after canceling the post-login refresh", async () => {
    let resolve!: (status: CodexStatus) => void;
    vi.mocked(getCodexStatus).mockResolvedValueOnce(connected)
      .mockImplementationOnce(() => new Promise(done => {resolve = done;}));
    const config = reactive({...defaultConfig, provider: "codex" as const});
    const wrapper = mount(ModelSettings, {props: {config, locale: "en", active: true}});
    await flushPromises();
    await wrapper.get('[data-testid="codex-login"]').trigger("click");
    await flushPromises();
    await wrapper.get('[data-testid="codex-cancel"]').trigger("click");
    resolve(connected);
    await flushPromises();
    expect(wrapper.get('[data-testid="codex-detect"]').attributes("disabled")).toBeUndefined();
    expect(wrapper.find('[data-testid="codex-status"]').exists()).toBe(false);
  });
});
