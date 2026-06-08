/**
 * Markdown 渲染单例：marked + highlight.js + 安全转义。
 *
 * 集中放这里有两个原因：
 * 1. marked 的全局 use(...) 是有副作用的；多处调用会重复注册插件、报警告。
 *    在 module 顶层只配置一次，所有 caller 直接 import 用。
 * 2. WikiView 和 QueryView 都需要相同的渲染（含 wikilink 占位符 + 代码高亮）；
 *    把通用部分剥到这里避免漂移。
 *
 * 代码高亮策略：
 *   - 用 highlight.js（hljs），核心 ~25KB + 自动检测语言
 *   - 主题：直接在 style.css 里写 .hljs-* 规则匹配 ink/vermilion/indigo 三色
 *     不引入 hljs 自带 CSS（github-dark 之类的），避免色板冲突
 *   - 失败 / 不识别的语言 fallback 到原文本
 */
import { marked } from 'marked'
import { markedHighlight } from 'marked-highlight'
import hljs from 'highlight.js/lib/common'

let _initialized = false

function initOnce() {
  if (_initialized) return
  _initialized = true

  marked.use(
    markedHighlight({
      // 语言提示：```python 这种 fence 会传 lang
      langPrefix: 'hljs language-',
      highlight(code, lang) {
        if (lang && hljs.getLanguage(lang)) {
          try {
            return hljs.highlight(code, { language: lang, ignoreIllegals: true }).value
          } catch {
            // 落到 auto 路径
          }
        }
        try {
          return hljs.highlightAuto(code).value
        } catch {
          return code
        }
      },
    }),
  )

  /*
   * 自定义 code renderer：在每个代码块外面包一层 <div class="code-block">，
   * 加一个右上角的「复制」按钮（带 data-action="copy-code" 标识）。
   * 实际复制逻辑由调用方（QueryView / WikiView 的 onMessageClick）
   * 接管：点击时定位到对应 <pre><code> 元素读 textContent 写剪贴板。
   *
   * 为什么不在按钮 data 里直接塞代码 base64：
   *  1) 长代码块塞进 attribute 会让 v-html 的 HTML 体积翻倍
   *  2) 转义复杂（特别是双引号 / 反斜杠混合）
   *  3) 已经在 DOM 里了，直接读 textContent 最干净
   *
   * markedHighlight 已经把 code 高亮成 HTML 字符串塞进 text；
   * marked 内部把 escaped=true 标记上来，这里直接原样输出，不要再 escape。
   */
  marked.use({
    renderer: {
      code({ text, lang, escaped }: { text: string; lang?: string; escaped?: boolean }) {
        const langClass = lang ? ` language-${escapeAttr(lang)}` : ''
        const langLabel = lang ? `<span class="code-lang">${escapeAttr(lang)}</span>` : ''
        // markedHighlight 处理过的 code 是已转义/高亮的 HTML 字符串；
        // 没处理过的（无 lang 也没 hljs 接管）则需要自己转义防 XSS
        const body = escaped ? text : escapeHtml(text)
        return (
          `<div class="code-block">` +
          `<div class="code-block-head">` +
            langLabel +
            `<button type="button" class="code-copy-btn" data-action="copy-code" aria-label="copy">⎘</button>` +
          `</div>` +
          `<pre><code class="hljs${langClass}">${body}</code></pre>` +
          `</div>`
        )
      },
    },
  })

  // 一些通用约定：链接默认新窗口、不渲染原始 HTML 标签（防 XSS，下游 v-html 才安全）
  marked.setOptions({
    breaks: false,
    gfm: true,
  })
}

/**
 * 转义 HTML 文本（用于无 lang 的 code 块原文输出）。
 * marked 默认会处理，但我们自定义 renderer 覆盖了它，所以得自己来。
 */
function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/** 转义放进 attribute 的字符串。lang 一般是 [a-z0-9]+，但保险起见。 */
function escapeAttr(s: string): string {
  return s.replace(/[&"<>]/g, c => ({ '&': '&amp;', '"': '&quot;', '<': '&lt;', '>': '&gt;' }[c]!))
}

initOnce()

/**
 * 把含 [[category/slug|alias]] 的 markdown 渲染成 HTML，wikilink 替换为 <a class="wikilink">。
 * 通过 @@WIKILINK_n@@ 占位符避免 marked 把链接当成普通文本转义。
 */
export function renderMarkdownWithWikilinks(src: string): string {
  const links: { category: string; name: string; label: string }[] = []
  const text = src.replace(/\[\[([^\]]+)\]\]/g, (_m, inner: string) => {
    const raw = String(inner).trim()
    // 拆 "target|alias"
    const pipe = raw.indexOf('|')
    let target = raw
    let alias = ''
    if (pipe >= 0) {
      target = raw.slice(0, pipe).trim()
      alias = raw.slice(pipe + 1).trim()
    }
    // 拆 "category/slug"
    const slash = target.indexOf('/')
    let category = 'concepts'
    let name = target
    if (slash >= 0) {
      category = target.slice(0, slash)
      name = target.slice(slash + 1)
    }
    const idx = links.length
    links.push({ category, name, label: alias || name })
    return `@@WIKILINK_${idx}@@`
  })
  let html = marked.parse(text) as string
  html = html.replace(/@@WIKILINK_(\d+)@@/g, (_m, i: string) => {
    const l = links[Number(i)]
    if (!l) return ''
    return `<a class="wikilink" data-category="${l.category}" data-name="${encodeURIComponent(l.name)}">${l.label}</a>`
  })
  return html
}

/**
 * 处理 markdown 渲染区域的 click 事件，专门处理代码块的「复制」按钮。
 *
 * 用法：在 v-html 容器的 @click 里调 `if (handleCodeCopyClick(e)) return`，
 * 同步返回 true 表示"这是代码块复制按钮点击且已接管（含 preventDefault/stopPropagation）"，
 * 调用方应直接 return，不再走自己的逻辑。复制写剪贴板的异步部分已 fire-and-forget 处理。
 *
 * 复制策略：
 *  1) 找最近的 .code-block 父级
 *  2) 取里面的 <code> 的 textContent（高亮 hljs 跨 span 时也会拿到正确明文）
 *  3) navigator.clipboard.writeText 优先；失败降级 textarea + execCommand
 *  4) 在按钮上短暂改 textContent / 加 class.copied 给视觉反馈
 */
export function handleCodeCopyClick(e: Event): boolean {
  const target = e.target as HTMLElement | null
  if (!target) return false
  const btn = target.closest<HTMLButtonElement>('button[data-action="copy-code"]')
  if (!btn) return false
  e.preventDefault()
  e.stopPropagation()
  const block = btn.closest<HTMLElement>('.code-block')
  const code = block?.querySelector<HTMLElement>('pre code')
  if (!code) return true
  const text = code.textContent || ''
  // 异步部分：fire-and-forget，不阻塞调用方
  void copyToClipboard(text).then(ok => {
    const original = btn.dataset.origText ?? btn.textContent ?? '⎘'
    btn.dataset.origText = original
    btn.textContent = ok ? '✓' : '×'
    btn.classList.toggle('copied', ok)
    btn.classList.toggle('failed', !ok)
    window.setTimeout(() => {
      if (btn.isConnected) {
        btn.textContent = original
        btn.classList.remove('copied', 'failed')
      }
    }, 1400)
  })
  return true
}

async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    // 降级
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'; ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      let ok = false
      try { ok = document.execCommand('copy') } catch { /* ignore */ }
      document.body.removeChild(ta)
      return ok
    } catch {
      return false
    }
  }
}
