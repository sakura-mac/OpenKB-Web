/**
 * i18n setup. Two locales: zh-CN (default) and en.
 *
 * Editorial language note: the "brand" / decorative bits (e.g. "OKB",
 * "Knowledge Workshop", roman-numeral tabs) are intentionally English in
 * BOTH locales — they are typographic marks, not interface text.
 */
import { createI18n } from 'vue-i18n'

const messages = {
  'zh-CN': {
    brand: {
      mark: 'OKB',
      sub: '知识工坊',
      tagline: '一份开放的知识库 · 在此编辑与装订',
    },
    sidebar: {
      spaces: '空间',
      newSpace: '新建空间',
      edit: '管理',
      delete: '删除',
      done: '完成',
      noSpaces: '还没有空间，请先在下方新建。',
      docsConcepts: '{docs} 文档 · {concepts} 概念',
    },
    topbar: {
      eyebrow: '知识库',
      welcomeEyebrow: '欢迎',
      noSpace: '尚未选择空间',
    },
    tabs: {
      // key 必须与 App.vue 里 tab.key 一致：wiki / graph / query
      // （docs 已移到侧边栏 hover popover，但保留键以防降级渲染）
      wiki: '知识',
      docs: '文档',
      graph: '图谱',
      query: '问答',
    },
    empty: {
      pickLine1: '从左侧目录里挑一个知识库，',
      pickLine2: '或者新建一个。',
    },
    common: {
      cancel: '取消',
      submit: '提交',
      create: '创建',
      creating: '创建中…',
      delete: '删除',
      open: '打开',
      download: '下载',
      remove: '移除',
      copy: '复制',
      copied: '✓ 已复制',
      back: '返回',
      browse: '浏览',
      up: '↑ 上级',
      use: '选择此处',
      empty: '空目录',
      loading: '加载中…',
    },
    create: {
      formNo: '表单 01',
      title: '新建知识库空间',
      nameLabel: '名称',
      namePlaceholder: '例如：ml-papers, design-system',
      pathLabel: '存放路径 · 可选',
      pathPlaceholder: '默认使用系统目录',
      errEmpty: '请输入名称',
      errInvalidName: '名称只能包含英文/数字/下划线/短横线',
      timeoutMsg: '初始化超时',
      initFail: '初始化失败',
      initialising: '正在初始化空间「{name}」…',
      ready: '空间「{name}」初始化完成',
      deleteConfirmOne: '确认删除空间「{name}」？',
      deleteConfirmMany: '确认删除 {count} 个空间？\n\n{names}',
      deleteWarn: '\n\n⚠️ 不可撤销，所有文档和知识将被永久删除！',
    },
    wiki: {
      summaries: '摘要',
      concepts: '概念',
      entities: '实体',
      explorations: '探索',
      backHistory: '返回上一页（栈中还有 {n} 页）',
      backToList: '返回列表',
      jumpGraph: '在图谱中聚焦此节点',
      jumpGraphLabel: '在图谱中查看 →',
      conceptCount: '共 {n} 个概念',
      entityCount: '共 {n} 个实体',
      readSummary: '查看摘要 →',
      readExploration: '查看分析 →',
      noSummaries: '尚无摘要 — 添加一份文档让它编译。',
      noConcepts: '尚无概念。',
      noEntities: '尚无实体。',
      noExplorations: '尚无探索笔记 — agent 在对话中分析时会自动生成。',
      pageLoading: '正在加载…',
    },
    docs: {
      sectionA: '甲 · 资料',
      sectionB: '乙 · 产物',
      titleSources: '文档',
      titleDecks: 'HTML 幻灯片',
      btnFromUrl: '从 URL',
      btnFromServer: '从服务器',
      btnUpload: '上传',
      btnCompileDeck: '生成幻灯片',
      emptyDocs: '空空如也 — 拖一份 PDF 或粘贴 URL 开始。',
      emptyDecks: '尚未生成幻灯片 — 由 wiki 编译（约 30~90 秒）。',
      compiling: '· 编译中 …',

      // upload modal
      uploadFormNo: '表单 02',
      uploadTitle: '上传文档',
      uploadDropLine: '拖文件到此处，或',
      uploadBrowseLink: '浏览选择',
      uploadHint: '支持 PDF / Markdown / TXT / Word / 幻灯片等。',
      uploadNote: '上传完成后 OpenKB 会编译进 wiki（每个文件约 30~60 秒）。',
      uploadSubmit: '上传并编译',

      // server browser modal
      browserFormNo: '表单 03',
      browserTitle: '从服务器添加文件',
      browserSelected: '已选 {n} 项',
      browserAdd: '添加',

      // url modal
      urlFormNo: '表单 04',
      urlTitle: '从 URL 添加',
      urlLabel: '一行一个 URL · 网页 / PDF / arXiv',
      urlPlaceholder: 'https://arxiv.org/pdf/2509.11420\nhttps://example.com/article.html',
      urlNote: '每个 URL 后台抓取并编译（约 30~120 秒），可关闭弹窗，进度显示在页脚。',
      urlBadProtocol: 'URL 必须以 http:// 或 https:// 开头：{u}',

      // deck modal
      deckFormNo: '表单 05',
      deckTitle: '生成 HTML 幻灯片',
      deckNameLabel: '名称 · kebab-case，作为目录名',
      deckNamePlaceholder: '如：transformers-pitch',
      deckIntentLabel: '意图 · 这份幻灯片要讲什么、面向谁',
      deckIntentPlaceholder: '如：向工程师解释 attention 机制，含示意图和示例。',
      deckCritique: '开启二次评审（更慢更优）',
      deckErrName: '请填写名称',
      deckErrSlug: '只能英文/数字/下划线/短横线',
      deckErrIntent: '请描述这份幻灯片要讲什么',
      deckErrSubmit: '提交失败',
      deckSubmitting: '提交中…',
      deckSubmit: '生成',
      deckCompiling: '正在生成幻灯片「{name}」（30~90 秒）…',
      deckDeleteConfirm: '确认删除幻灯片「{name}」？\n\n包含迭代历史，不可恢复。',

      // task messages
      taskFiles: '{n} 份文件已编译',
      taskFailUpload: '上传失败',
      taskAddFail: '添加失败',
      taskNetworkErr: '网络错误：{e}',
      taskUrlReqFail: 'URL 请求失败：{u}',
      taskUrlSubmitFail: 'URL 提交失败：{u}',
      taskUrlSubmitted: '已提交：{label}，等待编译…',
      taskRemoveConfirm: '确认删除文档「{name}」？\n\n会重新编译知识库（概念和实体会更新，约 30~60 秒）。',
      taskRemoveSubmitted: '已提交删除「{name}」，等待重新编译…',
      taskRemoveFail: '删除「{name}」失败',
      taskUnknown: '未知错误',
    },
    graph: {
      filterConcepts: '概念',
      filterEntities: '实体',
      filterDocs: '文档',
      search: '搜索节点…',
      loading: '正在加载图谱…',
      empty: '尚无图谱 — 添加文档并等待编译。',
      tipShowAll: '展开全图（cose 布局）',
      tipFit: '适配视口',
      tipZoomIn: '放大',
      tipZoomOut: '缩小',
      tipRelayout: '重新布局（选中节点时仅作用于高亮节点）',
      hint: '右键打开 · 双击重聚焦',
    },
    query: {
      sessionsEyebrow: '会话',
      sessionsTitle: '历史对话',
      collapse: '收起',
      expand: '展开',
      newSession: '＋ 新建',
      noSessions: '尚无对话。',
      turn: '轮',
      turns: '轮',
      relJustNow: '刚刚',
      relMinAgo: '{n} 分钟前',
      relHourAgo: '{n} 小时前',
      relDayAgo: '{n} 天前',
      threadContinuing: '继续',
      threadNew: '新对话',
      threadUntitled: '未命名对话',
      loadingSession: '正在加载会话…',
      emptyAsk: '随便问，会话会自动归档。',
      emptyKb: '知识库 · {name}',
      suggestionsLabel: '不知道问什么？试试这些',
      suggestionsRefresh: '换一批',
      followUpLabel: '继续问',
      traceTools: '思考过程',
      traceGraph: '本轮涉及',
      placeholder: '输入问题 — Enter 发送，Shift+Enter 换行。',
      send: '发送',
      thinkingConnecting: '连接中',
      thinkingDefault: '思考中',
      generating: '正在生成回答',
      generatingProgress: '正在生成…已 {n} 字',
      toolRunning: '调用工具：{name}',
      abortHint: '点击中断',
      aborted: '（已中断）',
      justDone: '答案已就绪',
      errInference: '推理失败',
      errNetwork: '网络错误：{e}',
      deleteConfirm: '删除此对话？',
    },
  },

  // ---------------- ENGLISH ----------------
  en: {
    brand: {
      mark: 'OKB',
      sub: 'Knowledge\nWorkshop',
      tagline: 'An open knowledge base, edited and bound.',
    },
    sidebar: {
      spaces: 'Spaces',
      newSpace: 'New space',
      edit: 'Edit',
      delete: 'Delete',
      done: 'Done',
      noSpaces: 'No spaces yet — start one below.',
      docsConcepts: '{docs} · {concepts}',
    },
    topbar: {
      eyebrow: 'Knowledge base',
      welcomeEyebrow: 'Welcome',
      noSpace: 'No space chosen',
    },
    tabs: {
      wiki: 'Knowledge',
      docs: 'Documents',
      graph: 'Graph',
      query: 'Query',
    },
    empty: {
      pickLine1: 'Pick a knowledge base from the table of contents,',
      pickLine2: 'or open a new one.',
    },
    common: {
      cancel: 'Cancel',
      submit: 'Submit',
      create: 'Create',
      creating: 'Creating…',
      delete: 'Delete',
      open: 'Open',
      download: 'Download',
      remove: 'remove',
      copy: 'copy',
      copied: '✓ copied',
      back: '← Back',
      browse: 'Browse',
      up: '↑ Up',
      use: 'Use this',
      empty: 'empty directory',
      loading: 'Loading…',
    },
    create: {
      formNo: 'Form 01',
      title: 'Open a new knowledge base',
      nameLabel: 'Name',
      namePlaceholder: 'e.g. ml-papers, design-system',
      pathLabel: 'Storage path · optional',
      pathPlaceholder: 'defaults to system directory',
      errEmpty: 'A name is required.',
      errInvalidName: 'Letters, digits, underscores, hyphens only.',
      timeoutMsg: 'Initialisation timed out',
      initFail: 'Initialisation failed',
      initialising: 'Initialising space "{name}"…',
      ready: 'Space "{name}" ready',
      deleteConfirmOne: 'Delete space "{name}"?',
      deleteConfirmMany: 'Delete {count} spaces?\n\n{names}',
      deleteWarn: '\n\nThis cannot be undone — all documents and knowledge will be lost.',
    },
    wiki: {
      summaries: 'Summaries',
      concepts: 'Concepts',
      entities: 'Entities',
      explorations: 'Explorations',
      backHistory: 'Go back ({n} in history)',
      backToList: 'Back to list',
      jumpGraph: 'Locate in graph',
      jumpGraphLabel: 'Find in graph →',
      conceptCount: '{n} concepts',
      entityCount: '{n} entities',
      readSummary: 'read summary →',
      readExploration: 'read analysis →',
      noSummaries: 'No summaries yet — add a document and let it compile.',
      noConcepts: 'No concepts yet.',
      noEntities: 'No entities yet.',
      noExplorations: 'No explorations yet — agents create these when synthesizing across pages in chat.',
      pageLoading: 'Loading…',
    },
    docs: {
      sectionA: 'Section A · Sources',
      sectionB: 'Section B · Output',
      titleSources: 'Documents',
      titleDecks: 'HTML decks',
      btnFromUrl: 'From URL',
      btnFromServer: 'From server',
      btnUpload: 'Upload',
      btnCompileDeck: 'Compile a deck',
      emptyDocs: 'Nothing here yet — drop a PDF or paste a URL to begin.',
      emptyDecks: 'No decks yet — compile one from your wiki (LLM, ~30–90s).',
      compiling: '· compiling …',

      uploadFormNo: 'Form 02',
      uploadTitle: 'Upload documents',
      uploadDropLine: 'Drop files here, or',
      uploadBrowseLink: 'browse',
      uploadHint: 'PDF, Markdown, TXT, Word, slides — all welcome.',
      uploadNote: 'After upload, OpenKB will compile the source into the wiki (≈30–60s per file).',
      uploadSubmit: 'Upload & compile',

      browserFormNo: 'Form 03',
      browserTitle: 'Add files from server',
      browserSelected: '{n} selected',
      browserAdd: 'Add',

      urlFormNo: 'Form 04',
      urlTitle: 'Add from URL',
      urlLabel: 'One URL per line · webpages, PDFs, arXiv',
      urlPlaceholder: 'https://arxiv.org/pdf/2509.11420\nhttps://example.com/article.html',
      urlNote: 'Each URL fetches and compiles in the background (≈30–120s). The window can be closed; progress shows at the foot of the page.',
      urlBadProtocol: 'URL must start with http:// or https:// — got: {u}',

      deckFormNo: 'Form 05',
      deckTitle: 'Compile an HTML deck',
      deckNameLabel: 'Slug · kebab-case, used as folder name',
      deckNamePlaceholder: 'e.g. transformers-pitch',
      deckIntentLabel: 'Intent · what the deck should explain, for whom',
      deckIntentPlaceholder: 'e.g. Explain attention to engineers, with diagrams and examples.',
      deckCritique: 'Run a second-pass critique (slower, finer)',
      deckErrName: 'A name is required.',
      deckErrSlug: 'Letters, digits, underscores, hyphens only.',
      deckErrIntent: 'Describe what this deck should explain.',
      deckErrSubmit: 'Submission failed',
      deckSubmitting: 'Submitting…',
      deckSubmit: 'Compile',
      deckCompiling: 'Compiling deck "{name}" (30–90s)…',
      deckDeleteConfirm: 'Delete deck "{name}"?\n\nIteration history is included; this cannot be undone.',

      taskFiles: '{n} file(s) compiled',
      taskFailUpload: 'Upload failed',
      taskAddFail: 'Failed to add',
      taskNetworkErr: 'Network error: {e}',
      taskUrlReqFail: 'URL request failed: {u}',
      taskUrlSubmitFail: 'URL submission failed: {u}',
      taskUrlSubmitted: 'Submitted: {label}, waiting to compile…',
      taskRemoveConfirm: 'Delete document "{name}"?\n\nThe wiki will be recompiled (concepts and entities will update, ≈30–60s).',
      taskRemoveSubmitted: 'Submitted: deleting "{name}", waiting for recompile…',
      taskRemoveFail: 'Failed to delete "{name}"',
      taskUnknown: 'Unknown error',
    },
    graph: {
      filterConcepts: 'Concepts',
      filterEntities: 'Entities',
      filterDocs: 'Documents',
      search: 'Search nodes…',
      loading: 'Loading the graph…',
      empty: 'No graph yet — add documents and let them compile.',
      tipShowAll: 'Show full graph (cose layout)',
      tipFit: 'Fit to viewport',
      tipZoomIn: 'Zoom in',
      tipZoomOut: 'Zoom out',
      tipRelayout: 'Re-layout (only highlighted, when a node is locked)',
      hint: 'right-click to open · double-click to refocus',
    },
    query: {
      sessionsEyebrow: 'Conversations',
      sessionsTitle: 'Sessions',
      collapse: 'Collapse',
      expand: 'Expand',
      newSession: '＋ New',
      noSessions: 'No conversations yet.',
      turn: 'turn',
      turns: 'turns',
      relJustNow: 'just now',
      relMinAgo: '{n}m ago',
      relHourAgo: '{n}h ago',
      relDayAgo: '{n}d ago',
      threadContinuing: 'Continuing',
      threadNew: 'New thread',
      threadUntitled: 'Untitled conversation',
      loadingSession: 'Loading session…',
      emptyAsk: 'Ask anything — your conversation will be kept on file.',
      emptyKb: 'Knowledge base · {name}',
      suggestionsLabel: 'Not sure where to start? Try one of these',
      suggestionsRefresh: 'Refresh',
      followUpLabel: 'Follow up',
      traceTools: 'Reasoning trace',
      traceGraph: 'In this answer',
      placeholder: 'Type a question — Enter to send, Shift+Enter for newline.',
      send: 'Send',
      thinkingConnecting: 'connecting',
      thinkingDefault: 'thinking',
      generating: 'composing the answer',
      generatingProgress: 'composing… {n} chars',
      toolRunning: 'running tool: {name}',
      abortHint: 'click to stop',
      aborted: '(stopped)',
      justDone: 'answer ready',
      errInference: 'Inference failed',
      errNetwork: 'Network error: {e}',
      deleteConfirm: 'Delete this conversation?',
    },
  },
}

const STORAGE_KEY = 'okb-locale'
type Locale = 'zh-CN' | 'en'

// 同步 detect：仅看 localStorage（用户曾手动切换过的话）+ 浏览器语言。
// 没有就先用 zh-CN 作启动占位，后续 detectLocaleFromServer() 会用后端 IP 判断结果覆盖。
function detectLocaleSync(): Locale {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'zh-CN' || stored === 'en') return stored
  const nav = navigator.language || ''
  return nav.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en'
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocaleSync(),
  fallbackLocale: 'en',
  messages,
})

/**
 * 用 /api/locale 让后端按 IP 自动判定中/英文。
 * 仅当用户**没有**手动选过语言（localStorage 为空）时生效，避免覆盖用户偏好。
 * 失败默认 fallback 到同步 detect 的结果，不抛错。
 */
export async function detectLocaleFromServer() {
  if (localStorage.getItem(STORAGE_KEY)) return // 用户已选过，尊重它
  try {
    const res = await fetch('/api/locale')
    if (!res.ok) return
    const data = await res.json() as { locale?: string }
    if (data.locale === 'zh-CN' || data.locale === 'en') {
      ;(i18n.global.locale as any).value = data.locale
    }
  } catch {
    // 静默失败：保留同步 detect 的结果
  }
}

export function setLocale(loc: Locale) {
  ;(i18n.global.locale as any).value = loc
  localStorage.setItem(STORAGE_KEY, loc)
}

export function currentLocale(): Locale {
  return (i18n.global.locale as any).value as Locale
}
