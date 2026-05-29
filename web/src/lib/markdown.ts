import { queueMathTypeset } from "./math";

const MAX_MATH_EXPRESSIONS = 100;
const MAX_MATH_EXPR_LEN = 4000;

function escapeHtml(input: string): string {
  return (input || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function buildMathPlaceholders(md: string): { text: string; dict: Map<string, string> } {
  let seq = 0;
  const dict = new Map<string, string>();
  const nextToken = (): string => {
    seq += 1;
    return `@@MATH_${seq}@@`;
  };

  let text = md;
  const addMathPlaceholder = (expr: string, block: boolean): string | null => {
    const content = String(expr || "").trim();
    if (!content || content.length > MAX_MATH_EXPR_LEN || dict.size >= MAX_MATH_EXPRESSIONS) {
      return null;
    }
    const token = nextToken();
    const escaped = escapeHtml(content);
    dict.set(
      token,
      block ? `<div class="md-math-block">\\[${escaped}\\]</div>` : `<span class="md-math-inline">\\(${escaped}\\)</span>`,
    );
    return token;
  };

  text = text.replace(/\$\$([\s\S]+?)\$\$/g, (match: string, expr: string) => {
    const token = addMathPlaceholder(expr, true);
    if (!token) return match;
    return `\n${token}\n`;
  });

  text = text.replace(/\\\((.+?)\\\)/g, (match: string, expr: string) => {
    return addMathPlaceholder(expr, false) ?? match;
  });

  text = text.replace(/(^|[^\\$])\$([^\n$]+?)\$/g, (match: string, prefix: string, expr: string) => {
    const token = addMathPlaceholder(expr, false);
    if (!token) return match;
    return `${prefix}${token}`;
  });

  return { text, dict };
}

function renderInline(md: string, restore: (s: string) => string): string {
  let s = escapeHtml(md);
  s = s.replace(/`([^`]+)`/g, "<code>$1</code>");
  s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  s = s.replace(/\*([^*]+)\*/g, "<em>$1</em>");
  s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_m, text: string, url: string) => {
    const safe = /^(https?:\/\/|mailto:)/i.test(url) ? url : "#";
    return `<a href="${safe}" target="_blank" rel="noopener noreferrer">${text}</a>`;
  });
  return restore(s);
}

export function renderMarkdown(md: string): string {
  try {
    const MAX_MD_LEN = 200000;
    const source = String(md || "");
    const clipped = source.length > MAX_MD_LEN ? source.slice(0, MAX_MD_LEN) : source;
    const prepared = buildMathPlaceholders(clipped.replace(/\r\n/g, "\n"));
    const restore = (s: string): string =>
      s.replace(/@@MATH_\d+@@/g, (token) => prepared.dict.get(token) ?? token);
    const lines = prepared.text.split("\n");
    const out: string[] = [];
    let inList = false;

    const closeList = () => {
      if (inList) {
        out.push("</ul>");
        inList = false;
      }
    };

    for (const raw of lines) {
      const line = raw.trimEnd();
      const t = line.trim();
      if (!t) {
        closeList();
        continue;
      }
      if (/^@@MATH_\d+@@$/.test(t)) {
        closeList();
        out.push(restore(t));
        continue;
      }
      if (/^#{1,6}\s+/.test(t)) {
        closeList();
        const n = Math.min(6, t.match(/^#+/)?.[0].length || 1);
        const body = t.replace(/^#{1,6}\s+/, "");
        out.push(`<h${n}>${renderInline(body, restore)}</h${n}>`);
        continue;
      }
      if (/^[-*]\s+/.test(t)) {
        if (!inList) {
          out.push("<ul>");
          inList = true;
        }
        out.push(`<li>${renderInline(t.replace(/^[-*]\s+/, ""), restore)}</li>`);
        continue;
      }
      if (/^>\s*/.test(t)) {
        closeList();
        out.push(`<blockquote>${renderInline(t.replace(/^>\s*/, ""), restore)}</blockquote>`);
        continue;
      }
      closeList();
      out.push(`<p>${renderInline(t, restore)}</p>`);
    }
    closeList();
    if (source.length > MAX_MD_LEN) {
      out.push(`<p><em>内容较长，已截断显示前 ${MAX_MD_LEN} 个字符。</em></p>`);
    }
    const html = out.join("\n");
    if (prepared.dict.size > 0) {
      queueMathTypeset();
    }
    return html;
  } catch {
    const fallback = escapeHtml(String(md || ""));
    return `<pre>${fallback}</pre>`;
  }
}
