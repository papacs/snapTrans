<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { cancelCodexLogin, getCodexStatus, loginCodex, type AppConfig, type CodexStatus } from "../services/backend";
import { settingsMessages, type SettingsLocale } from "../i18n/settings";

const props = defineProps<{ config: AppConfig; locale: SettingsLocale; active: boolean }>();
const text = computed(() => settingsMessages[props.locale]);
const copy = computed(() => props.locale === "en" ? {
  provider: "Model connection", api: "API key · OpenAI compatible", codex: "Local Codex · ChatGPT subscription",
  help: "Use your local Codex ChatGPT sign-in and subscription. Codex manages credentials and refreshes them automatically.",
  path: "Codex path (optional)", pathHelp: "Leave blank to detect the desktop Codex runtime, then the CLI on PATH. You can also enter the full path to codex.exe. CLI install: npm install -g @openai/codex.",
  refresh: "Detect login & models", loading: "Detecting…", login: "Sign in with ChatGPT", signingIn: "Complete sign-in in your browser…",
  cancel: "Cancel sign-in", connected: "Subscription connected", missing: "No ChatGPT sign-in. Sign in here or run codex login, then detect again.",
  default: "Use the account's default model", modelHelp: "Choose an available model or enter its exact ID. Leaving this blank uses the default returned by Codex.",
  note: "Sign-in is shared with your local Codex CLI. Saving selects this connection for translation and text actions.",
} : {
  provider: "模型接入方式", api: "API 密钥 · OpenAI 兼容接口", codex: "本地 Codex · ChatGPT 订阅",
  help: "复用本机 Codex 的 ChatGPT 登录与订阅额度，由 Codex 管理凭据和自动续期。",
  path: "Codex 路径（可选）", pathHelp: "留空优先检测桌面版 Codex，再从 PATH 检测 CLI；也可填写 codex.exe 的完整路径。CLI 安装命令：npm install -g @openai/codex。",
  refresh: "检测登录与模型", loading: "正在检测…", login: "登录 ChatGPT 订阅", signingIn: "请在浏览器完成授权…",
  cancel: "取消授权", connected: "订阅已接入", missing: "尚未通过 ChatGPT 登录。可在此授权，或运行 codex login 后重新检测。",
  default: "使用账号默认模型", modelHelp: "可选择可用模型或填写完整模型 ID；留空使用 Codex 返回的默认模型。",
  note: "授权与本机 Codex CLI 共享。保存后，翻译和文字扩展功能均使用此接入方式。",
});
const status = ref<CodexStatus | null>(null);
const error = ref("");
const loading = ref(false);
const signingIn = ref(false);
let sequence = 0;

async function detect(): Promise<void> {
  const current = ++sequence;
  loading.value = true;
  error.value = "";
  status.value = null;
  try {
    const result = await getCodexStatus(props.config.codexPath);
    if (current === sequence) status.value = result;
  } catch (reason) {
    if (current === sequence) error.value = String(reason instanceof Error ? reason.message : reason);
  } finally {
    if (current === sequence) loading.value = false;
  }
}

async function signIn(): Promise<void> {
  const current = ++sequence;
  signingIn.value = true;
  error.value = "";
  try {
    await loginCodex(props.config.codexPath);
    if (current === sequence) await detect();
  } catch (reason) {
    if (current === sequence) error.value = String(reason instanceof Error ? reason.message : reason);
  } finally {
    signingIn.value = false;
  }
}

async function cancelSignIn(): Promise<void> {
  ++sequence;
  loading.value = false;
  try { await cancelCodexLogin(); }
  catch (reason) { error.value = String(reason instanceof Error ? reason.message : reason); }
  // Keep the controls disabled until LoginCodex has actually exited.
}

watch(() => props.config.codexPath, () => {
  ++sequence;
  status.value = null;
  error.value = "";
  loading.value = false;
});
watch(() => [props.config.provider, props.active], () => {
  ++sequence;
  loading.value = false;
  if (signingIn.value) void cancelSignIn();
  if (props.config.provider === "codex" && props.active) void detect();
}, { immediate: true });
onBeforeUnmount(() => { ++sequence; if (signingIn.value) void cancelCodexLogin().catch(() => {}); });
</script>

<template>
  <label class="settings-field mb-4">
    <span class="settings-label">{{ copy.provider }}</span>
    <select v-model="config.provider" class="settings-input" data-testid="model-provider" :disabled="signingIn">
      <option value="openai-compatible">{{ copy.api }}</option>
      <option value="codex">{{ copy.codex }}</option>
    </select>
  </label>

  <template v-if="config.provider !== 'codex'">
    <label class="settings-field">
      <span class="settings-label">{{ text.apiKey }}</span>
      <input v-model="config.apiKey" data-testid="api-key-input" class="settings-input" type="password" autocomplete="off" :placeholder="text.apiKeyPlaceholder" />
    </label>
    <div class="mt-4 grid gap-4 sm:grid-cols-2">
      <label class="settings-field">
        <span class="settings-label">{{ text.baseURL }}</span>
        <input v-model="config.baseURL" class="settings-input" type="url" placeholder="https://your-litellm-host/v1" />
        <span class="settings-help">{{ text.baseURLHelp }}</span>
      </label>
      <label class="settings-field">
        <span class="settings-label">{{ text.model }}</span>
        <input v-model="config.model" class="settings-input" type="text" placeholder="deepseek-v4-flash" />
        <span class="settings-help">{{ text.modelHelp }}</span>
      </label>
    </div>
  </template>

  <div v-else class="space-y-4" data-testid="codex-settings">
    <p class="settings-help">{{ copy.help }}</p>
    <label class="settings-field">
      <span class="settings-label">{{ copy.path }}</span>
      <input v-model="config.codexPath" class="settings-input" data-testid="codex-path" type="text" placeholder="codex" :disabled="signingIn" />
      <span class="settings-help">{{ copy.pathHelp }}</span>
    </label>
    <div class="flex flex-wrap gap-2">
      <button type="button" class="settings-secondary-button" data-testid="codex-detect" :disabled="loading || signingIn" @click="detect">{{ loading ? copy.loading : copy.refresh }}</button>
      <button v-if="!signingIn" type="button" class="settings-secondary-button" data-testid="codex-login" :disabled="loading" @click="signIn">{{ copy.login }}</button>
      <button v-else type="button" class="settings-secondary-button" data-testid="codex-cancel" @click="cancelSignIn">{{ copy.cancel }}</button>
    </div>
    <p v-if="signingIn" class="settings-help" role="status">{{ copy.signingIn }}</p>
    <p v-if="error" class="break-words text-xs text-rose-600 dark:text-rose-400" role="alert">{{ error }}</p>
    <p v-if="status" class="break-words text-xs" :class="status.authenticated ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-700 dark:text-amber-400'" role="status" data-testid="codex-status">
      {{ status.authenticated ? [copy.connected, status.email, status.plan].filter(Boolean).join(' · ') : copy.missing }}
    </p>
    <label class="settings-field">
      <span class="settings-label">{{ text.model }}</span>
      <input v-model="config.codexModel" class="settings-input" data-testid="codex-model" type="text" list="codex-model-options" :placeholder="copy.default" />
      <datalist id="codex-model-options">
        <option v-for="model in status?.models ?? []" :key="model.id" :value="model.id">{{ model.name }}{{ model.isDefault ? ' ★' : '' }}</option>
      </datalist>
      <span class="settings-help">{{ copy.modelHelp }}</span>
    </label>
    <p class="settings-help">{{ copy.note }}</p>
    <label class="settings-field">
      <span class="settings-label">{{ locale === 'en' ? 'Response speed / thinking' : '响应速度与思考强度' }}</span>
      <select v-model="config.codexReasoningEffort" class="settings-input" data-testid="codex-effort">
        <option value="low">{{ locale === 'en' ? 'Fast · light thinking (recommended)' : '快速 · 轻量思考（推荐）' }}</option>
        <option value="medium">{{ locale === 'en' ? 'Balanced · medium thinking' : '均衡 · 适度思考' }}</option>
        <option value="high">{{ locale === 'en' ? 'Thorough · more thinking' : '深入 · 更多思考' }}</option>
        <option value="xhigh">{{ locale === 'en' ? 'Extended thinking · slower' : '深度思考 · 等待更久' }}</option>
        <option value="none">{{ locale === 'en' ? 'No thinking (e.g. GPT-6 Luna)' : '不启用思考（如 GPT-6 Luna）' }}</option>
        <option value="default">{{ locale === 'en' ? 'Follow local Codex settings' : '跟随本机 Codex 设置' }}</option>
      </select>
      <span class="settings-help">{{ locale === 'en' ? 'Light thinking suits translation and short replies. Your local Codex configuration is unchanged.' : '轻量思考适合翻译与简短回复，不会修改本机 Codex 配置。' }}</span>
    </label>
  </div>
  <label v-if="config.provider === 'codex'" class="settings-toggle mt-4">
    <span class="min-w-0 pr-3">
      <span class="settings-toggle-title">{{ locale === 'en' ? 'Prioritize translation speed' : '翻译优先速度（推荐）' }}</span>
      <span class="settings-toggle-description">{{ locale === 'en' ? 'Use concise translation instructions and no thinking on Luna. Replies keep the thinking setting above. Recent successful translations are reused locally.' : '翻译使用精简指令，Luna 不启用思考；回复仍使用上方强度。近期成功译文在本机复用。' }}</span>
    </span>
    <input v-model="config.fastTranslation" data-testid="fast-translation" type="checkbox" class="peer sr-only" />
    <span class="settings-switch" aria-hidden="true" />
  </label>
  <label class="settings-toggle mt-4">
    <span class="min-w-0 pr-3">
      <span class="settings-toggle-title">{{ locale === 'en' ? 'Read screenshots directly · skip OCR' : '直接看图 · 跳过 OCR' }}</span>
      <span class="settings-toggle-description">{{ locale === 'en' ? 'Send the selected screenshot to a vision-capable model. Connection tests use a built-in color swatch, never your screen. Selected text still uses text input.' : '截图翻译和回复直接上传选区截图，需要支持图片的模型。连接测试使用内置色块，不上传屏幕内容；选词仍使用文字。' }}</span>
    </span>
    <input v-model="config.directImageInput" data-testid="direct-image-input" type="checkbox" class="peer sr-only" />
    <span class="settings-switch" aria-hidden="true" />
  </label>
</template>
