import { computed, reactive } from "vue";
import zhCn from "element-plus/es/locale/lang/zh-cn";
import en from "element-plus/es/locale/lang/en";

export type UiLanguage = "zh-CN" | "en";

const STORAGE_KEY = "gpuops.ui.language";

function loadSavedLanguage(): UiLanguage {
  try {
    const raw = String(localStorage.getItem(STORAGE_KEY) || "").trim();
    if (raw === "en") return "en";
  } catch {
    // ignore localStorage errors
  }
  return "zh-CN";
}

export const uiLocaleState = reactive({
  language: loadSavedLanguage() as UiLanguage,
});

export const elementPlusLocale = computed(() => (uiLocaleState.language === "en" ? en : zhCn));

export function toggleUiLanguage() {
  uiLocaleState.language = uiLocaleState.language === "en" ? "zh-CN" : "en";
  try {
    localStorage.setItem(STORAGE_KEY, uiLocaleState.language);
  } catch {
    // ignore localStorage errors
  }
}

export function pickText(zh: string, enText: string): string {
  return uiLocaleState.language === "en" ? enText : zh;
}
