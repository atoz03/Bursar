let typesetTimer: number | null = null;

declare global {
  interface Window {
    MathJax?: {
      typesetPromise?: () => Promise<void>;
      typesetClear?: () => void;
    };
    __gpuopsMathReady?: boolean;
    __gpuopsMathObserver?: MutationObserver;
  }
}

export function queueMathTypeset(): void {
  if (typeof window === "undefined") return;
  if (typesetTimer != null) {
    window.clearTimeout(typesetTimer);
  }
  typesetTimer = window.setTimeout(() => {
    typesetTimer = null;
    const mj = window.MathJax;
    if (!mj?.typesetPromise) return;
    try {
      mj.typesetClear?.();
      mj.typesetPromise().catch(() => {});
    } catch {
      // ignore
    }
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

  if (!window.__gpuopsMathObserver) {
    window.__gpuopsMathObserver = new MutationObserver(() => {
      queueMathTypeset();
    });
    window.__gpuopsMathObserver.observe(document.body, {
      childList: true,
      subtree: true,
      characterData: true,
    });
  }
}

