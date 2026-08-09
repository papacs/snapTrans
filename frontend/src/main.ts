import { createApp } from "vue";
import App from "./App.vue";
import "./styles.css";

const app = createApp(App);

app.config.errorHandler = (error, _instance, info) => {
  const message = error instanceof Error ? `${error.message}\n${error.stack ?? ""}` : String(error);
  console.error(`[Vue error: ${info}]`, error);
  const runtime = window.runtime as typeof window.runtime & { LogError?: (message: string) => void };
  runtime?.LogError?.(`[Vue error: ${info}] ${message}`);

  const root = document.getElementById("app");
  if (!root) {
    return;
  }

  const diagnostic = document.createElement("main");
  diagnostic.style.cssText =
    "box-sizing:border-box;display:flex;width:100%;height:100%;padding:32px;align-items:center;justify-content:center;background:#0f172a;color:#e2e8f0;font:14px/1.6 Segoe UI,sans-serif";

  const card = document.createElement("section");
  card.style.cssText =
    "max-width:440px;padding:24px;border:1px solid #334155;border-radius:12px;background:#111827;box-shadow:0 18px 60px rgba(0,0,0,.35)";
  const title = document.createElement("h1");
  title.style.cssText = "margin:0 0 8px;font-size:17px;color:#f8fafc";
  title.textContent = "snapTrans couldn't render this view";
  const help = document.createElement("p");
  help.style.cssText = "margin:0;color:#94a3b8";
  help.textContent = "Please restart snapTrans. Technical details were written to the application log.";
  card.append(title, help);
  diagnostic.append(card);
  root.replaceChildren(diagnostic);
};

app.mount("#app");

