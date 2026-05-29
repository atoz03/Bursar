let typesetTimer: number | null = null;
let typesetting = false;
let rerunAfterTypeset = false;

declare global {
  interface Window {
    MathJax?: {
      typesetPromise?: (elements?: Element[]) => Promise<void>;
      typesetClear?: (elements?: Element[]) => void;
    };
    __gpuopsMathReady?: boolean;
  }
}

function mathTargets(): Element[] {
  if (typeof document === "undefined") return [];
  return Array.from(document.querySelectorAll(".md-body"));
}

function runMathTypeset(): void {
  const mj = window.MathJax;
  if (!mj?.typesetPromise) return;
  const targets = mathTargets();
  if (targets.length === 0) return;
  if (typesetting) {
    rerunAfterTypeset = true;
    return;
  }
  typesetting = true;
  try {
    mj.typesetClear?.(targets);
    mj
      .typesetPromise(targets)
      .catch(() => {})
      .finally(() => {
        typesetting = false;
        if (rerunAfterTypeset) {
          rerunAfterTypeset = false;
          queueMathTypeset();
        }
      });
  } catch {
    typesetting = false;
  }
}

export function queueMathTypeset(): void {
  if (typeof window === "undefined") return;
  if (typesetTimer != null) {
    window.clearTimeout(typesetTimer);
  }
  typesetTimer = window.setTimeout(() => {
    typesetTimer = null;
    runMathTypeset();
  }, 60);
}

function injectMathJaxScript() {
  const exists = document.querySelector('script[data-mathjax="gpuops"]');
  if (exists) return;
  const script = document.createElement("script");
  script.src = "https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js";
  script.async = true;
  script.defer = true;
  script.setAttribute("data-mathjax", "gpuops");
  script.onload = () => queueMathTypeset();
  document.head.appendChild(script);
}

export function setupMathSupport(): void {
  if (typeof window === "undefined" || typeof document === "undefined") return;
  if (window.__gpuopsMathReady) return;
  window.__gpuopsMathReady = true;

  (window as any).MathJax = (window as any).MathJax ?? {
    tex: {
      inlineMath: [["\\(", "\\)"], ["$", "$"]],
      displayMath: [["\\[", "\\]"], ["$$", "$$"]],
      processEscapes: true,
    },
    options: {
      skipHtmlTags: ["script", "noscript", "style", "textarea", "pre", "code"],
    },
    startup: {
      typeset: false,
    },
  };

  injectMathJaxScript();
}
