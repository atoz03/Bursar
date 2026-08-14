<template>
  <div class="turnstile-shell" :class="{ 'is-loading': loading, 'is-verified': verified }">
    <div ref="containerRef" class="turnstile-container" />
    <div v-if="loading" class="turnstile-loading">
      <span class="turnstile-spinner" />
      <span>正在进行安全验证</span>
    </div>
    <div v-if="verified" class="turnstile-success">
      <span class="turnstile-check">✓</span>
      <span>安全验证已通过</span>
    </div>
    <div v-if="message" class="turnstile-message">{{ message }}</div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

type TurnstileAPI = {
  render: (container: HTMLElement, options: Record<string, unknown>) => string;
  reset: (widgetId?: string) => void;
  remove: (widgetId: string) => void;
};

declare global {
  interface Window {
    turnstile?: TurnstileAPI;
  }
}

const props = withDefaults(defineProps<{
  sitekey: string;
  action: string;
  language?: string;
}>(), {
  language: "auto",
});

const emit = defineEmits<{
  (event: "verified", token: string): void;
  (event: "expired"): void;
  (event: "error"): void;
}>();

const containerRef = ref<HTMLElement | null>(null);
const loading = ref(true);
const verified = ref(false);
const message = ref("");
let widgetId = "";
let disposed = false;
let scriptPromise: Promise<void> | null = null;

function loadTurnstileScript(): Promise<void> {
  if (window.turnstile) return Promise.resolve();
  if (scriptPromise) return scriptPromise;
  scriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>('script[data-gpuops-turnstile="true"]');
    if (existing) {
      existing.addEventListener("load", () => resolve(), { once: true });
      existing.addEventListener("error", () => reject(new Error("Turnstile script failed")), { once: true });
      return;
    }
    const script = document.createElement("script");
    script.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
    script.async = true;
    script.defer = true;
    script.dataset.gpuopsTurnstile = "true";
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("Turnstile script failed"));
    document.head.appendChild(script);
  });
  return scriptPromise;
}

async function renderWidget() {
  loading.value = true;
  verified.value = false;
  message.value = "";
  emit("verified", "");
  try {
    await loadTurnstileScript();
    await nextTick();
    if (disposed || !containerRef.value || !window.turnstile) return;
    if (widgetId) {
      window.turnstile.remove(widgetId);
      widgetId = "";
    }
    containerRef.value.replaceChildren();
    widgetId = window.turnstile.render(containerRef.value, {
      sitekey: props.sitekey,
      action: props.action,
      language: props.language,
      theme: "light",
      size: "flexible",
      appearance: "interaction-only",
      "refresh-expired": "auto",
      callback: (token: string) => {
        loading.value = false;
        verified.value = true;
        message.value = "";
        emit("verified", token);
      },
      "expired-callback": () => {
        loading.value = false;
        verified.value = false;
        message.value = "验证已过期，正在重新验证";
        emit("expired");
      },
      "error-callback": () => {
        loading.value = false;
        verified.value = false;
        message.value = "安全验证加载失败，请检查网络后重试";
        emit("error");
      },
    });
    // render() 返回后 Cloudflare 控件已经可交互，不能继续用加载层覆盖复选框。
    loading.value = false;
  } catch {
    loading.value = false;
    verified.value = false;
    message.value = "无法连接安全验证服务，请检查网络后重试";
    emit("error");
  }
}

function reset() {
  emit("verified", "");
  loading.value = true;
  verified.value = false;
  message.value = "";
  if (window.turnstile && widgetId) {
    window.turnstile.reset(widgetId);
    loading.value = false;
    return;
  }
  void renderWidget();
}

defineExpose({ reset });

watch(() => [props.sitekey, props.action, props.language], () => {
  void renderWidget();
});

onMounted(() => {
  void renderWidget();
});

onBeforeUnmount(() => {
  disposed = true;
  if (window.turnstile && widgetId) window.turnstile.remove(widgetId);
});
</script>

<style scoped>
.turnstile-shell {
  position: relative;
  width: 100%;
  min-height: 44px;
}
.turnstile-container {
  width: 100%;
  min-height: 44px;
}
.turnstile-loading {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  min-height: 44px;
  border: 1px solid rgba(148, 163, 184, .34);
  border-radius: 11px;
  color: #64748b;
  background: rgba(248, 250, 252, .82);
  font-size: 13px;
}
.turnstile-success {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  min-height: 44px;
  border: 1px solid rgba(16, 185, 129, .28);
  border-radius: 11px;
  color: #047857;
  background: linear-gradient(120deg, rgba(236, 253, 245, .96), rgba(240, 253, 250, .9));
  font-size: 13px;
  font-weight: 650;
}
.turnstile-check {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  color: #fff;
  background: #10b981;
  font-size: 13px;
  line-height: 1;
}
.turnstile-spinner {
  width: 15px;
  height: 15px;
  border: 2px solid rgba(37, 99, 235, .18);
  border-top-color: #2563eb;
  border-radius: 50%;
  animation: turnstileSpin .7s linear infinite;
}
.turnstile-message {
  margin-top: 7px;
  color: #b45309;
  font-size: 12px;
}
@keyframes turnstileSpin {
  to { transform: rotate(360deg); }
}
@media (prefers-reduced-motion: reduce) {
  .turnstile-spinner { animation: none; }
}
</style>
