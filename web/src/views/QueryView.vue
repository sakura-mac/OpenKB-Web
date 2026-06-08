<template>
  <div class="chat-wrap">
    <!--
      Sessions：默认折叠为细条（仅图标），点击展开。
      折叠态：只显示「展开」「+ 新建」两枚 icon 按钮，节省横向空间给主对话区
      展开态：完整列表（标题 / 轮数 / 时间 / 删除）
      用户偏好持久化在 localStorage('okb-sessions-open')
    -->
    <aside :class="['sessions', { collapsed: !sessionsOpen }]">
      <template v-if="sessionsOpen">
        <div class="sessions-head">
          <div>
            <div class="eyebrow">{{ t('query.sessionsEyebrow') }}</div>
            <div class="title">{{ t('query.sessionsTitle') }}</div>
          </div>
          <div class="sessions-head-actions">
            <button class="btn btn-sm new-btn" :title="t('query.newSession')" @click="newSession">{{ t('query.newSession') }}</button>
            <button class="icon-btn" :title="t('query.collapse')" @click="toggleSessions">‹</button>
          </div>
        </div>
        <div class="sessions-list">
          <div
            v-for="s in sessions" :key="s.id"
            :class="['session-item', { active: s.id === currentSid }]"
            @click="openSession(s.id)"
          >
            <div class="s-title">{{ s.title || `(${t('query.threadUntitled')})` }}</div>
            <div class="s-meta">
              <span class="s-meta-turns">{{ s.turn_count }} {{ s.turn_count === 1 ? t('query.turn') : t('query.turns') }}</span>
              <span class="s-meta-sep">·</span>
              <span class="s-meta-time">{{ relTime(s.updated_at) }}</span>
            </div>
            <button class="s-del" :title="t('common.delete')" @click.stop="deleteSession(s.id)">×</button>
          </div>
          <div v-if="sessions.length === 0" class="empty-hint" style="padding: 32px 16px">
            {{ t('query.noSessions') }}
          </div>
        </div>
      </template>

      <!--
        折叠态：极简 spine（书脊）。两个 icon（展开 / 新建），中间一根细线。
        不显示会话数——折叠态本来就为了视觉干净，多个数字反而干扰。
        用户想看会话数 = 直接展开侧栏。
      -->
      <div v-else class="sessions-rail">
        <button class="rail-btn rail-expand" :title="t('query.expand') + ' (' + sessions.length + ')'" @click="toggleSessions">
          <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true">
            <path d="M5 3 L9 7 L5 11" stroke="currentColor" stroke-width="1.4" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </button>
        <div class="rail-divider" aria-hidden="true"></div>
        <button class="rail-btn" :title="t('query.newSession')" @click="newSession">
          <svg width="13" height="13" viewBox="0 0 14 14" aria-hidden="true">
            <path d="M7 3 L7 11 M3 7 L11 7" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>
          </svg>
        </button>
      </div>
    </aside>

    <!-- Right: messages -->
    <main class="chat-main">
      <div class="chat-toolbar">
        <div>
          <div class="eyebrow">{{ currentSid ? t('query.threadContinuing') : t('query.threadNew') }}</div>
          <div class="chat-title">{{ currentTitle || t('query.threadUntitled') }}</div>
        </div>
        <span v-if="currentSid" class="sid-mark">{{ currentSid.slice(0, 12) }}</span>
      </div>

      <div ref="msgContainer" class="messages" @click="onMessageClick">
        <div v-if="loadingSession" class="empty-hint">
          <span class="loading-glyph">···</span> {{ t('query.loadingSession') }}
        </div>
        <div v-else-if="messages.length === 0 && !querying" class="chat-empty">
          <div class="ce-glyph">¶</div>
          <p class="ce-line">{{ t('query.emptyAsk') }}</p>
          <p class="ce-sub">{{ t('query.emptyKb', { name: space.name }) }}</p>

          <!--
            推荐问题卡片：4 张并排（窄屏自动换 2 列），点击直接发送。
            - 加载中显示 4 个骨架卡占位（避免空区跳变）
            - 「换一批」让后端再随机抽一组
          -->
          <div class="suggest-block">
            <div class="suggest-head">
              <span class="suggest-label">{{ t('query.suggestionsLabel') }}</span>
              <button
                class="suggest-refresh"
                :disabled="suggestLoading"
                :title="t('query.suggestionsRefresh')"
                @click="loadSuggestions(true)"
              >↻ {{ t('query.suggestionsRefresh') }}</button>
            </div>
            <div class="suggest-grid">
              <template v-if="suggestLoading && !suggestions.length">
                <div v-for="n in 4" :key="'sk' + n" class="suggest-card skeleton">
                  <span class="loading-glyph">···</span>
                </div>
              </template>
              <button
                v-for="(s, i) in suggestions" :key="i"
                class="suggest-card"
                @click="useSuggestion(s)"
                :title="s"
              >
                <span class="sg-num">{{ String(i + 1).padStart(2, '0') }}</span>
                <span class="sg-text">{{ s }}</span>
                <span class="sg-arrow">→</span>
              </button>
            </div>
          </div>
        </div>
        <div v-for="(m, i) in messages" :key="i" :class="['msg', m.role]">
          <div class="bubble" v-if="m.role === 'user'">{{ m.content }}</div>
          <div v-else class="assistant-wrap">
            <div class="msg-mark">A.</div>
            <!--
              生成中的「灰色小框」：用 3 跳点 + 流光进度条传达「正在工作」状态。
              不渲染流式 token（晕），但让用户清楚地感知后台在跑。
              整框可点击 → 中断当前生成（abortCtrl.abort()）。
              hover 时文案切到 "× 点击中断" 强提示可交互；正常态保持低存在感。
            -->
            <button
              v-if="!m.content && querying && i === messages.length - 1"
              type="button"
              class="bubble loading abortable"
              :title="t('query.abortHint')"
              @click="abortStream"
            >
              <span class="thinking-dots" aria-hidden="true">
                <span></span><span></span><span></span>
              </span>
              <span class="thinking-text thinking-text-default">{{ thinkingMsg || t('query.thinkingDefault') }}</span>
              <span class="thinking-text thinking-text-hover">× {{ t('query.abortHint') }}</span>
              <span class="thinking-shimmer" aria-hidden="true"></span>
            </button>
            <div v-else class="bubble-host">
              <!--
                推理过程折叠块：默认折叠（closed），点击展开看到这一轮 agent 调过的工具。
                数据从 getTools(i) 读（traces[i] 优先，历史回填轮没有就空数组不显示）。
                右栏「思考过程」时间轴也展示同一份数据，但右栏在桌面宽屏才有；
                这里 in-bubble 是窄屏 / 历史回看时的入口，保持复制 / 阅读体验干净（默认收起）。
              -->
              <details v-if="getTools(i).length" class="reason-fold">
                <summary class="reason-fold-summary">
                  <span class="rf-arrow" aria-hidden="true">▸</span>
                  <span class="rf-text">{{ t('query.traceTools') }} · {{ getTools(i).length }}</span>
                </summary>
                <ol class="reason-fold-list">
                  <li v-for="(t2, ti) in getTools(i)" :key="ti">
                    <span class="ti-arrow">→</span>
                    <span class="ti-text" :title="t2">{{ t2 }}</span>
                  </li>
                </ol>
              </details>
              <div class="bubble md-content" v-html="renderMd(m.content)"></div>
              <button
                v-if="m.content && !(querying && i === messages.length - 1)"
                class="copy-btn"
                :title="copiedIdx === i ? t('common.copied') : t('common.copy')"
                @click="copyAnswer(i)"
              >
                {{ copiedIdx === i ? t('common.copied') : t('common.copy') }}
              </button>
            </div>
            <!--
              右栏：思考过程 + 跟进推荐（共享 grid 第三列，桌面 sticky 跟随滚动）
              - 工具时间轴：每条 assistant 消息单独存自己那轮的 tools（traces[i]）
              - 知识子图：每条 assistant 消息单独算自己的 links（traces[i] 优先，否则即时从 content 抽）
              - follow-ups：仅挂最新一条 assistant（仅对当前轮有意义）
              整块仅在"有内容可展示"时渲染（querying / 工具 / 节点 / followUps 任一非空）；
              全空（含 KB 节点也匹配不到）则整块隐藏，避免无效占位。
            -->
            <div
              v-if="hasRightCol(i, m)"
              class="rightcol"
            >
              <!-- 工具调用时间轴：querying 中边跑边长，done 后保留 -->
              <div v-if="getTools(i).length" class="trace-block">
                <div class="trace-label">{{ t('query.traceTools') }}</div>
                <ol class="trace-list">
                  <li v-for="(t2, ti) in getTools(i)" :key="ti" class="trace-item">
                    <span class="ti-arrow">→</span>
                    <span class="ti-text" :title="t2">{{ t2 }}</span>
                  </li>
                </ol>
              </div>

              <!-- 本轮答案的"知识子图"：节点空则整块隐藏 -->
              <div
                v-if="!(querying && i === lastAssistantIdx) && m.content && getLinks(i, m.content).length"
                class="trace-block"
              >
                <div class="trace-label">{{ t('query.traceGraph') }}</div>
                <MiniGraph
                  :links="getLinks(i, m.content)"
                  @open="(p) => emit('openWiki', p)"
                />
              </div>

              <!-- 跟进推荐：仅挂最新一条 assistant（只对当前轮有意义） -->
              <div
                v-if="i === lastAssistantIdx && m.content && !querying && followUps.length"
                class="followup-row"
              >
                <span class="fu-label">{{ t('query.followUpLabel') }}</span>
                <button
                  v-for="(f, fi) in followUps" :key="fi"
                  class="followup-chip"
                  @click="useSuggestion(f)"
                  :title="f"
                >
                  <span class="fu-text">{{ f }}</span>
                  <span class="fu-arrow">→</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!--
        输入区上方的状态条：querying 时与上方 loading 气泡联动。
        作用：用户视线即使下移到输入框，也能立即看到「后台还在跑」的信号；
              生成完后短暂显示「✓ 答案已就绪」再淡出。
      -->
      <transition name="status-fade">
        <div v-if="querying" class="chat-status-bar working" aria-live="polite">
          <span class="csb-dots" aria-hidden="true">
            <span></span><span></span><span></span>
          </span>
          <span class="csb-text">{{ thinkingMsg || t('query.thinkingDefault') }}</span>
          <span class="csb-shimmer" aria-hidden="true"></span>
        </div>
        <div v-else-if="justFinished" class="chat-status-bar done" aria-live="polite">
          <span class="csb-check">✓</span>
          <span class="csb-text">{{ t('query.justDone') }}</span>
        </div>
      </transition>

      <div class="input-bar">
        <textarea
          v-model="question"
          rows="2"
          :placeholder="dynamicPlaceholder"
          @keydown="onKeydown"
          :disabled="querying"
        ></textarea>
        <button class="btn btn-primary send-btn" :disabled="querying || !question.trim()" @click="doSend">
          {{ querying ? '…' : t('query.send') }}
        </button>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, onMounted, onUnmounted, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { renderMarkdownWithWikilinks, handleCodeCopyClick } from '../markdown'
import type { SpaceDetail } from '../types'
import { useChatState } from '../composables/useChatState'
import MiniGraph from '../components/MiniGraph.vue'

const { t, locale } = useI18n()

interface SessionMeta { id: string; title: string; turn_count: number; updated_at: string }

const props = defineProps<{ space: SpaceDetail }>()

// 状态从 composable 读，跨 tab 切换/组件重建保留（按 space 隔离）
const chatState = useChatState(props.space.name)
const currentSid = computed({ get: () => chatState.currentSid, set: v => { chatState.currentSid = v } })
const currentTitle = computed({ get: () => chatState.currentTitle, set: v => { chatState.currentTitle = v } })
const messages = chatState.messages // 直接引用底层数组，push/splice 都是响应式

const sessions = ref<SessionMeta[]>([])
const question = ref('')
const msgContainer = ref<HTMLElement>()
const copiedIdx = ref<number>(-1)

// 会话栏折叠状态：默认折叠（节省横向空间），用户偏好持久化。
// 用 localStorage 而非 chatState 是因为这是「全局 UI 偏好」，
// 与具体哪个 space 无关。
const SESSIONS_KEY = 'okb-sessions-open'
const sessionsOpen = ref<boolean>(localStorage.getItem(SESSIONS_KEY) === '1')
function toggleSessions() {
  sessionsOpen.value = !sessionsOpen.value
  localStorage.setItem(SESSIONS_KEY, sessionsOpen.value ? '1' : '0')
}

// 这些「正在进行中的一轮」的状态都跨 mount 持久化在 chatState 里：
// 切走再切回时不会出现「assistant bubble 自己写但 UI 状态错乱」。
// 用 computed get/set 把 .value 语义保下来，原 doSend 代码不用改。
const querying = computed({ get: () => chatState.querying, set: v => { chatState.querying = v } })
const thinkingMsg = computed({ get: () => chatState.thinkingMsg, set: v => { chatState.thinkingMsg = v } })
const followUps = computed({ get: () => chatState.followUps, set: v => { chatState.followUps = v } })

const lastAssistantIdx = computed(() => {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'assistant') return i
  }
  return -1
})

// 「答案刚就绪」短暂提示，done 后显示 1.5s 然后自动隐藏；
// 用 ref 而不是 chatState 里持久化——切走切回不需要重现这个一次性提示
const justFinished = ref(false)
let justFinishedTimer: number | null = null
function flashJustFinished() {
  justFinished.value = true
  if (justFinishedTimer) window.clearTimeout(justFinishedTimer)
  justFinishedTimer = window.setTimeout(() => {
    justFinished.value = false
    justFinishedTimer = null
  }, 1500)
}

/**
 * 抽出本轮答案涉及的"知识子图"节点。
 *
 * 策略（合并去重，最多 12 个）：
 *  1) 显式 wikilinks：`[[category/slug]]` / `[[category/slug|alias]]`
 *     —— LLM 严格按 wikilink 语法时直接命中
 *  2) Markdown 链接占位（renderMarkdownWithWikilinks 也认这套）：
 *     `[文本](category/slug)` 形式，比双方括号更常见
 *  3) KB 节点名兜底匹配：
 *     拿 props.space.concepts/entities/summaries 的 slug 列表 + titles 映射，
 *     在答案文本里做子串匹配；命中即纳入。这是最关键的兜底——绝大多数
 *     LLM 回答既不写 [[]] 也不写 markdown 链接，而是直接写实体名。
 *
 * 为什么之前总是空：之前只有 (1)，而 LLM 一般不输出 [[]] 语法。
 */
function extractWikilinks(text: string): Array<{ category: string; name: string }> {
  const out: Array<{ category: string; name: string }> = []
  const seen = new Set<string>()
  const push = (category: string, name: string) => {
    const key = category + '/' + name
    if (seen.has(key)) return
    seen.add(key)
    out.push({ category, name })
  }

  // (1) [[category/slug]]
  const reWiki = /\[\[([^\]]+)\]\]/g
  let m: RegExpExecArray | null
  while ((m = reWiki.exec(text)) !== null) {
    if (out.length >= 12) break
    let raw = m[1].trim()
    const pipe = raw.indexOf('|')
    if (pipe >= 0) raw = raw.slice(0, pipe).trim()
    const slash = raw.indexOf('/')
    if (slash < 0) {
      push('concepts', raw)
    } else {
      push(raw.slice(0, slash), raw.slice(slash + 1))
    }
  }

  // (2) [文本](concepts/foo) / [文本](entities/bar) / [文本](summaries/baz) / [文本](explorations/xx)
  const reMd = /\]\((concepts|entities|summaries|explorations)\/([^)\s#]+)\)/g
  while ((m = reMd.exec(text)) !== null) {
    if (out.length >= 12) break
    push(m[1], m[2])
  }

  // (3) KB 节点名兜底匹配：用本空间已知的节点 slug + 显示标题在文本里做子串扫描
  if (out.length < 12) {
    const lower = text.toLowerCase()
    const sp = props.space
    const titlesMap = sp.titles || {}
    const cats: Array<{ category: string; list: string[] }> = [
      { category: 'concepts', list: sp.concepts || [] },
      { category: 'entities', list: sp.entities || [] },
      { category: 'summaries', list: sp.summaries || [] },
      { category: 'explorations', list: sp.explorations || [] },
    ]
    for (const { category, list } of cats) {
      for (const slug of list) {
        if (out.length >= 12) break
        if (!slug) continue
        // 候选名：slug 本身 + 标题（标题往往是中文，而 slug 是 pinyin/kebab）
        const candidates: string[] = [slug]
        const fullKey = `${category}/${slug}`
        if (titlesMap[fullKey]) candidates.push(titlesMap[fullKey])
        if (titlesMap[slug]) candidates.push(titlesMap[slug])
        // 把 slug 的 - / _ 还原成空格后也试一次（"react-hook" → "react hook"）
        const normSlug = slug.replace(/[-_]+/g, ' ').trim()
        if (normSlug && normSlug !== slug) candidates.push(normSlug)
        for (const cand of candidates) {
          if (!cand || cand.length < 2) continue
          if (lower.includes(cand.toLowerCase())) {
            push(category, slug)
            break
          }
        }
      }
      if (out.length >= 12) break
    }
  }

  return out
}

/**
 * 模板辅助：取第 i 条 assistant 消息的工具时间轴。
 * 优先 traces[i]（doSend 流程注册的）；历史回填消息没注册 → 返回空数组。
 */
function getTools(i: number): string[] {
  return chatState.traces[i]?.tools || []
}

/**
 * 模板辅助：取第 i 条 assistant 消息的知识子图节点。
 * 优先 traces[i].links；缺失（如历史 session 回填）则即时从 m.content 抽。
 * 即时抽用 KB 节点名兜底匹配 → 历史轮也能看到图谱。
 *
 * 用 _linksCache 把"按下标的即时抽取结果"缓存起来，避免每次模板重渲染都跑一遍正则 + 子串扫描。
 * cache key 用 `${i}::${content.length}::首尾摘要`，content 改变即失效。
 */
const _linksCache = new Map<string, Array<{ category: string; name: string }>>()
function getLinks(i: number, content: string): Array<{ category: string; name: string }> {
  const stored = chatState.traces[i]?.links
  if (stored && stored.length) return stored
  if (!content) return []
  // 缓存：内容指纹 + 当前 space（切 space 时整页 unmount cache 自然清，但保险用 space 名混进 key）
  const key = `${props.space.name}::${i}::${content.length}::${content.slice(0, 24)}::${content.slice(-24)}`
  const hit = _linksCache.get(key)
  if (hit) return hit
  const computed = extractWikilinks(content)
  _linksCache.set(key, computed)
  return computed
}

/**
 * 模板辅助：判断第 i 条 assistant 消息是否需要展示右栏。
 * 任一非空即显示：querying（最新轮）/ 工具 / 节点 / followUps。
 * 全空 → 整块隐藏（用户要求"空面板就别显示"）。
 */
function hasRightCol(i: number, m: { content: string }): boolean {
  const isLast = i === lastAssistantIdx.value
  if (isLast && querying.value) return true
  if (getTools(i).length) return true
  if (m.content && getLinks(i, m.content).length) return true
  if (isLast && followUps.value.length) return true
  return false
}

async function copyAnswer(i: number) {
  const m = messages[i]
  if (!m) return
  try {
    await navigator.clipboard.writeText(m.content)
  } catch {
    // 降级：textarea + execCommand
    const ta = document.createElement('textarea')
    ta.value = m.content
    ta.style.position = 'fixed'; ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    try { document.execCommand('copy') } catch { /* ignore */ }
    document.body.removeChild(ta)
  }
  copiedIdx.value = i
  setTimeout(() => { if (copiedIdx.value === i) copiedIdx.value = -1 }, 1500)
}

function renderMd(s: string) {
  return renderMarkdownWithWikilinks(s)
}

const emit = defineEmits<{ openWiki: [{ category: string; name: string }] }>()

function onMessageClick(e: MouseEvent) {
  // 优先处理代码块复制按钮（任何 v-html 渲染的 markdown 区都共用这套）
  if (handleCodeCopyClick(e)) return
  const t = e.target as HTMLElement
  if (t.classList.contains('wikilink')) {
    e.preventDefault()
    const category = t.dataset.category || 'concepts'
    const name = decodeURIComponent(t.dataset.name || '')
    if (name) emit('openWiki', { category, name })
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    doSend()
  }
}

async function scrollToBottom() {
  await nextTick()
  if (msgContainer.value) msgContainer.value.scrollTop = msgContainer.value.scrollHeight
}

/**
 * 答案 done 后的滚动定位：让用户看到的不是答案的尾巴，
 * 而是「自己的提问 + 答案的开头」。
 *
 * 实现：把 assistant 气泡前面那条 user 消息滚到视口顶（带 16px padding）。
 * 找不到对应 user 消息时退化为：把 assistant 气泡顶对齐视口顶。
 */
/**
 * 把第 asstIdx 条 assistant 之前的 user 消息滚到 messages 容器的最顶上。
 * 用户视觉：屏幕第一行 = "我刚问的问题" → 紧接着是答案开头。
 *
 * 实现细节（踩过坑）：
 *  - 单 nextTick 不够：marked + hljs 高亮 + mini-graph 渲染会引发多轮 reflow。
 *    用 double nextTick + rAF 等到 layout 真正稳定。
 *  - 不用 element.offsetTop：offsetTop 是相对 offsetParent，复杂 grid + sticky
 *    嵌套下 offsetParent 不一定是 msgContainer。
 *    用 getBoundingClientRect() 算两者 viewport 偏差更准：
 *       新 scrollTop = 当前 scrollTop + (target.top - container.top) - 顶部留白
 *  - 顶部留白用 0：让 user 消息完全贴顶；之前用 16px 反而让用户感觉答案"位置不对"。
 *  - behavior: 'auto' 而非 'smooth'：smooth 会持续若干帧，期间如果用户滚动会冲突；
 *    一次性瞬间到位更可靠（用户也不会觉得"被拽走"，因为这本来就是答案就绪的视觉切换）。
 */
async function scrollToAnswerStart(asstIdx: number) {
  await nextTick()
  await nextTick()
  await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
  const root = msgContainer.value
  if (!root) return
  const items = root.querySelectorAll<HTMLElement>('.msg')
  // 优先把 user（asstIdx - 1）滚到顶；没有就用 assistant 自己
  const target = items[asstIdx - 1] || items[asstIdx]
  if (!target) return
  const containerRect = root.getBoundingClientRect()
  const targetRect = target.getBoundingClientRect()
  const delta = targetRect.top - containerRect.top
  // 留 0 px 顶部空间——让用户提问完全贴顶
  const next = root.scrollTop + delta
  root.scrollTo({ top: Math.max(0, next), behavior: 'auto' })
}

async function loadSessions() {
  const res = await api.chatSessions(props.space.name)
  sessions.value = res.ok && res.sessions ? res.sessions : []
}

const loadingSession = ref(false)

async function openSession(sid: string) {
  if (sid === currentSid.value) return
  if (loadingSession.value) return
  loadingSession.value = true
  // 切换前先清空，避免显示旧消息
  currentSid.value = sid
  messages.splice(0)
  currentTitle.value = ''
  followUps.value = []
  // 清空逐轮思考过程（旧 session 的 traces 不再适用，会被新消息回填后即时算）
  chatState.traces = {}
  chatState.trace = { tools: [], links: [] }
  _linksCache.clear()
  try {
    const res = await api.chatLoadSession(props.space.name, sid)
    // 防并发：用户在等待期间又点了别的 session
    if (sid !== currentSid.value) return
    if (res.ok && res.messages) {
      res.messages.forEach(m => messages.push(m as { role: 'user' | 'assistant'; content: string }))
      currentTitle.value = res.title || ''
    }
  } finally {
    loadingSession.value = false
    await scrollToBottom()
  }
}

function newSession() {
  currentSid.value = ''
  currentTitle.value = ''
  messages.splice(0)
  followUps.value = []
  chatState.traces = {}
  chatState.trace = { tools: [], links: [] }
  _linksCache.clear()
}

async function deleteSession(sid: string) {
  if (!confirm(t('query.deleteConfirm'))) return
  await api.chatDeleteSession(props.space.name, sid)
  if (sid === currentSid.value) newSession()
  await loadSessions()
}

function relTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso).getTime()
  if (isNaN(d)) return ''
  const diff = (Date.now() - d) / 1000
  if (diff < 60) return t('query.relJustNow')
  if (diff < 3600) return t('query.relMinAgo', { n: Math.floor(diff / 60) })
  if (diff < 86400) return t('query.relHourAgo', { n: Math.floor(diff / 3600) })
  return t('query.relDayAgo', { n: Math.floor(diff / 86400) })
}

/**
 * 中断当前流式生成。
 * 用户点击灰色 loading 框时调用：abort fetch → reader.read() 抛 AbortError →
 * catch 分支感知 aborted（通过 e.name）后做"友好收尾"：保留已累积的 answerText
 * + 追加（已中断）尾标，而不是当成错误展示。
 *
 * 也兼顾了竞态：如果中断信号到达时已经没有 abortCtrl（done 已收到），就静默忽略。
 */
function abortStream() {
  const ctrl = chatState.abortCtrl
  if (!ctrl) return
  try { ctrl.abort() } catch { /* ignore */ }
}

async function doSend() {
  const q = question.value.trim()
  if (!q || querying.value) return

  // 1) 立即把用户消息加进 UI；同时清空旧的 follow-ups（就是当前问题的来源）
  messages.push({ role: 'user', content: q })
  question.value = ''
  querying.value = true
  thinkingMsg.value = t('query.thinkingConnecting')
  followUps.value = []
  await scrollToBottom()

  // 2) 预先放一个空的助手气泡。它的内容在 done 之前一直为空，
  //    模板的 loading 分支会显示一个灰色小框作为「思考/正在生成」状态。
  const assistantIdx = messages.length
  messages.push({ role: 'assistant', content: '' })
  // 推完 loading 气泡再滚一次：
  // 上一次 scrollToBottom 只看见 user 消息（loading 框还没 mount），
  // 这次确保 user + loading 灰框都在视口里——用户回车后立刻"看见自己的提问 +
  // 后台正在思考"的连贯反馈，而不是回车后页面纹丝不动。
  await scrollToBottom()
  // 标记本轮 turn 的 assistant 在 messages 里的下标。
  // follow-ups 事件迟到时会用它 vs 当前 chatState.activeAsstIdx 比较，
  // 如果不一致说明已被下一轮覆盖（用户连续发问），就丢弃这次 follow-ups。
  chatState.activeAsstIdx = assistantIdx
  // 清空上一轮的思考过程追踪——本轮重新累积
  chatState.trace = { tools: [], links: [] }
  // 同时把本轮 trace 注册进 traces[assistantIdx]，让模板按消息下标读到自己那轮的内容。
  // 引用同一对象 → tools.push 会同步增长，无需额外 sync。
  chatState.traces[assistantIdx] = chatState.trace

  // ─── 关键设计：不实时渲染 token ───────────────────────────────
  // LLM token 流如果按 delta 来一字一字 push 进 messages.content + 触发 marked + v-html，
  // 高速时整屏跳跃式重排，视觉非常晕。改成：
  //   - 流过程中只把 token 累到本地 answerText，用 chatState.streamingChars 计字数让
  //     loading 框显示进度（"已生成 N 字"+灰底脉冲），但 messages[idx].content 始终是 ''
  //   - 收到 done 时一次性把整段 answerText 写进 messages[idx].content，
  //     marked + 高亮一次性渲染出来，给用户「答案已就绪」的明确分界感
  //   - 工具调用（tool 事件）也只更新进度文案，不渲染到气泡里
  // 副作用：用户看不到逐字增长的过程；但首屏稳定、不晕、思考边界清晰，
  // 这正是用户要的"灰色小框 + 最后一大块直接回复"的体验。
  let answerText = ''
  let charCount = 0
  // 中断控制：用 AbortController 接 fetch；用户点灰色 loading 框时调用 abort()。
  // chatState.abortCtrl 存进 store 是为了：跨 mount 切走再切回，仍然能让别的组件触发中断（虽然目前只有当前组件用）。
  // aborted 标记给 catch/finally 逻辑用：true 时不当作错误显示，而是在已累积文本后追加 (已中断)。
  const abortCtrl = new AbortController()
  chatState.abortCtrl = abortCtrl
  let aborted = false
  // ──────────────────────────────────────────────────────────────

  try {
    const resp = await fetch('/api/chat/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      signal: abortCtrl.signal,
      body: JSON.stringify({
        space: props.space.name,
        message: q,
        session_id: currentSid.value || '',
        lang: locale.value,
      }),
    })
    if (!resp.ok || !resp.body) {
      throw new Error(`HTTP ${resp.status}`)
    }
    thinkingMsg.value = ''
    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buf = ''
    /**
     * 处理 buf 里所有完整的 SSE event（以 "\n\n" 分隔）。
     * 抽成函数是因为流读完后还要再调一次 flush 残留——
     * 如果最后一个 event 没收到 "\n\n" 就 EOF 了，必须把残留也当一个 event 解析，
     * 不然就丢了 done 事件 → 答案看上去"不完整"。
     */
    const processBuf = (final = false) => {
      let idx
      while ((idx = buf.indexOf('\n\n')) >= 0) {
        const chunk = buf.slice(0, idx)
        buf = buf.slice(idx + 2)
        handleSSEChunk(chunk)
      }
      if (final && buf.trim()) {
        // EOF 时把残留（可能没收到尾部 \n\n 的最后一个 event）也喂进去
        handleSSEChunk(buf)
        buf = ''
      }
    }
    const handleSSEChunk = (chunk: string) => {
      // 提取 data: 行（可能多行）
      const dataLines = chunk.split('\n').filter(l => l.startsWith('data: ')).map(l => l.slice(6))
      if (!dataLines.length) return
      const raw = dataLines.join('\n')
      let ev: any
      try { ev = JSON.parse(raw) } catch { return }
      // fire-and-forget：handleSSEEvent 内部的 await 只用于滚动 UI，不阻塞下一 event
      handleSSEEvent(ev)
    }
    /**
     * 处理单个 SSE event 对象。
     * 抽出来配合 buf 双重 flush（每次 read 后 + EOF 残留），避免最后一个 event
     * 没收到 `\n\n` 收尾就被丢弃 → 答案"不完整"的根因之一。
     */
    const handleSSEEvent = async (ev: any) => {
      if (ev.event === 'start') {
        if (!currentSid.value && ev.session_id) currentSid.value = ev.session_id
        // 第一个 token 还没来：进度文案改"正在生成"
        thinkingMsg.value = t('query.generating')
      } else if (ev.event === 'delta') {
        // 累积但不渲染：只更新进度计数 + 让灰色小框知道有内容在长
        const piece = ev.text || ''
        if (piece) {
          answerText += piece
          charCount += piece.length
          chatState.streamingChars = charCount
          thinkingMsg.value = t('query.generatingProgress', { n: charCount })
        }
      } else if (ev.event === 'tool') {
        // 工具调用：进 chatState.trace.tools，让 <details> 折叠块和右栏面板都能读
        const note = `${ev.name}(${(ev.args || '').slice(0, 60)})`
        if (chatState.activeAsstIdx === assistantIdx) {
          chatState.trace.tools.push(note)
        }
        thinkingMsg.value = t('query.toolRunning', { name: ev.name || '?' })
      } else if (ev.event === 'done') {
        if (!currentSid.value && ev.session_id) currentSid.value = ev.session_id
        if (ev.title) currentTitle.value = ev.title
        // done 兜底：用 done.answer 作权威，写一次完整的进 messages
        if (typeof ev.answer === 'string' && ev.answer.length > answerText.length) {
          answerText = ev.answer
        }
        // 一次性把累积的整段 answer 渲染到气泡里——给用户「答案已就绪」的清晰分界
        if (chatState.activeAsstIdx === assistantIdx) {
          // 注意：不再把工具调用拼到 content 前面——
          // 推理过程已由 bubble-host 顶部的 <details> 折叠块独立渲染
          // （数据从 traces[i] 读），保持气泡内容只是纯答案，复制 / 阅读更干净。
          messages[assistantIdx] = {
            role: 'assistant',
            content: answerText,
          }
          querying.value = false
          thinkingMsg.value = ''
          chatState.streamingChars = 0
          // 抽答案中的 wikilinks 作为「本轮答案的知识图谱」节点源
          chatState.trace.links = extractWikilinks(answerText)
          flashJustFinished()
        }
        // 滚动定位：让用户看到「自己的提问 + 答案的开头」
        await scrollToAnswerStart(assistantIdx)
        loadSessions()
      } else if (ev.event === 'follow_ups') {
        if (Array.isArray(ev.follow_ups) && chatState.activeAsstIdx === assistantIdx) {
          followUps.value = ev.follow_ups.filter((s: any) => typeof s === 'string' && s.trim())
          // 注意：这里**不能** scrollToBottom：done 事件刚把视口定到"提问 + 答案开头"，
          // follow_ups 几秒后异步到达，如果再滚底会把答案开头推出视口（用户看到页面"自己往下滑"）。
          // follow-ups 在右栏底部，用户读完答案自然下滑就能看到。
        }
      } else if (ev.event === 'error') {
        const errLine = `\n\n> ✦ ${ev.error || t('query.errInference')}`
        answerText += errLine
        if (chatState.activeAsstIdx === assistantIdx) {
          messages[assistantIdx] = { role: 'assistant', content: answerText }
        }
      }
    }
    while (true) {
      const { value, done } = await reader.read()
      if (done) {
        // EOF：先 flush decoder 残留多字节（utf-8 可能有半个汉字停在内部），再 process 残留 event
        buf += decoder.decode()
        processBuf(true)
        break
      }
      buf += decoder.decode(value, { stream: true })
      processBuf(false)
    }
  } catch (e: any) {
    // AbortError = 用户主动点中断。这是预期路径，不当错误展示：
    // 已累积的 answerText 原样保留 + 末尾追加 (已中断) 标记；空答案则只显示标记。
    if (e?.name === 'AbortError' || abortCtrl.signal.aborted) {
      aborted = true
      const note = `\n\n> _${t('query.aborted')}_`
      answerText = (answerText || '') + note
      if (chatState.activeAsstIdx === assistantIdx) {
        // 同 done 路径：不再把 tools 拼进 content，由 <details> 折叠块独立渲染
        messages[assistantIdx] = {
          role: 'assistant',
          content: answerText,
        }
      }
    } else {
      // 网络/解析错误：把已累积的 answerText（可能为空）+ 错误说明一次写到气泡
      const errLine = `\n\n> ✦ ${t('query.errNetwork', { e: e?.message || e })}`
      answerText = (answerText || '') + errLine
      if (chatState.activeAsstIdx === assistantIdx) {
        messages[assistantIdx] = { role: 'assistant', content: answerText }
      }
    }
  } finally {
    // 释放 abortCtrl 引用：本轮已结束（无论 done/中断/错误），下一轮 doSend 会建新的
    if (chatState.abortCtrl === abortCtrl) {
      chatState.abortCtrl = null
    }
    // 兜底：done 没收到（流提前断 / 用户中断）时也要 flush + 解锁
    let didFallbackFlush = false
    if (chatState.activeAsstIdx === assistantIdx) {
      // 如果 messages 还是空字符串说明 done 没来过——把累积的 answerText 写进去（即使没完整）
      if (messages[assistantIdx] && messages[assistantIdx].content === '' && answerText) {
        // 同 done 路径：不再把 tools 拼进 content，由 <details> 折叠块独立渲染
        messages[assistantIdx] = {
          role: 'assistant',
          content: answerText,
        }
        didFallbackFlush = true
      }
      if (querying.value) {
        querying.value = false
        thinkingMsg.value = ''
        chatState.streamingChars = 0
        loadSessions()
        didFallbackFlush = true
      }
    }
    // 滚动策略：done 正常路径已经 scrollToAnswerStart 到「提问 + 答案开头」，
    // finally 不能再 scrollToBottom 把答案开头推出视口。
    // 仅在异常/中断/done 没收到这种 fallback 路径下，才补一次 scrollToAnswerStart
    // 让用户至少看到「自己的提问 + 残缺答案的开头 + (已中断)/错误标记」。
    if (didFallbackFlush) {
      await scrollToAnswerStart(assistantIdx)
    }
    void aborted // 只是给 catch 用作语义标记，不在 finally 中读
  }
}

onMounted(loadSessions)

// ============================================================
// 推荐问题（首屏交互优化）
// ============================================================
//
// 行为：
//  1. 进入空 chat 状态时拉一组 4 条「基于本空间内容生成」的推荐问题
//  2. 输入框 placeholder 也用其中一条作动态提示（5s 轮换），
//     比静态 "输入问题…" 文案更能让用户意识到「这里是个问答助手」
//  3. 点击卡片 = 把问题填进 question 并立即调 doSend()，省一步
//
// 缓存：suggestions 不缓存到 localStorage——每次 mount 拉新的，
//   因为后端会随机抽样，刷新一次能换批意外发现的角度。
// ============================================================

const suggestions = ref<string[]>([])
const suggestLoading = ref(false)
const placeholderIdx = ref(0)
let placeholderTimer: number | null = null

// 进入空对话 → 拉 suggestions；切到非空对话 → 暂停轮播省 CPU
const isEmpty = computed(() => messages.length === 0 && !currentSid.value)

const dynamicPlaceholder = computed(() => {
  const fallback = t('query.placeholder')
  // 仅在「空对话状态」展示推荐问题，避免：
  //   - 已经发过一轮后 placeholder 还停在某条推荐（用户误以为输入框里有字按回车没反应）
  //   - 切换 session 后旧 suggestions 残留
  if (!isEmpty.value) return fallback
  if (suggestions.value.length === 0) return fallback
  const s = suggestions.value[placeholderIdx.value % suggestions.value.length]
  return s ? `${s}    ⏎` : fallback
})

async function loadSuggestions(force = false) {
  if (suggestLoading.value && !force) return
  suggestLoading.value = true
  try {
    const res = await api.suggestions(props.space.name, locale.value)
    suggestions.value = res.suggestions || []
    placeholderIdx.value = 0
  } catch {
    suggestions.value = []
  } finally {
    suggestLoading.value = false
  }
}

function startPlaceholderRotation() {
  stopPlaceholderRotation()
  placeholderTimer = window.setInterval(() => {
    if (suggestions.value.length > 0) {
      placeholderIdx.value = (placeholderIdx.value + 1) % suggestions.value.length
    }
  }, 5000)
}
function stopPlaceholderRotation() {
  if (placeholderTimer) {
    window.clearInterval(placeholderTimer)
    placeholderTimer = null
  }
}

// 把卡片里的问题塞进输入框立刻发送（保持 doSend 是单一入口）
function useSuggestion(text: string) {
  if (querying.value) return
  question.value = text
  doSend()
}

watch(isEmpty, (empty) => {
  if (empty) {
    loadSuggestions()
    startPlaceholderRotation()
  } else {
    stopPlaceholderRotation()
  }
}, { immediate: true })

// 切换 space（onMounted 触发不到，但保险起见）
watch(() => props.space.name, () => {
  if (isEmpty.value) loadSuggestions()
})

// locale 变了，文案也要换语言
watch(locale, () => {
  if (isEmpty.value) loadSuggestions()
})

onUnmounted(() => {
  stopPlaceholderRotation()
  if (justFinishedTimer) {
    window.clearTimeout(justFinishedTimer)
    justFinishedTimer = null
  }
})
</script>

<style scoped>
.chat-wrap {
  display: flex;
  gap: 0;
  height: calc(100vh - 200px);
  min-height: 540px;
  border: 1.5px solid var(--ink);
  background: var(--paper);
}

/* ----- Sessions sidebar (notebook spine) ----------------------- */
.sessions {
  width: 260px;
  flex-shrink: 0;
  border-right: 1px solid var(--paper-edge);
  background: var(--paper-2);
  display: flex;
  flex-direction: column;
  transition: width 240ms cubic-bezier(0.2, 0.7, 0.2, 1);
}
.sessions.collapsed {
  width: 36px;
}

/* 折叠态：极简 spine。
   - 整列居中，仅两个 SVG icon 按钮 + 中间一根连贯细线，
     线把两个 icon "缝"在一起，像合上的笔记本侧脊
   - 背景与展开态一致 paper-2，与主对话区有一道竖向边界感
   - 不放数字、不放横线条，避免折叠态变得复杂 */
.sessions-rail {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 14px 0;
  height: 100%;
}
.rail-btn {
  appearance: none;
  background: transparent;
  border: 0;
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--ink-3);
  cursor: pointer;
  transition: color 130ms ease, transform 130ms ease;
  flex-shrink: 0;
}
.rail-btn:hover { color: var(--vermilion); }
.rail-btn:hover svg { transform: scale(1.08); }
.rail-btn svg { transition: transform 150ms ease; }

/* 中间连接线：把两个 icon "缝起来"，制造书脊感 */
.rail-divider {
  width: 1px;
  flex: 1;
  min-height: 60px;
  background: var(--paper-edge);
  margin: 10px 0;
}

/* 展开态 head：头部加一个收起 icon */
.sessions-head-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}
.icon-btn {
  appearance: none;
  background: transparent;
  border: 0;
  width: 24px;
  height: 24px;
  font-family: var(--font-display);
  font-size: 16px;
  line-height: 1;
  color: var(--ink-4);
  cursor: pointer;
  transition: color 130ms ease;
}
.icon-btn:hover { color: var(--vermilion); }
.sessions-head {
  padding: 16px 18px 14px;
  border-bottom: 1px solid var(--paper-edge);
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 10px;
}
.sessions-head .title {
  font-family: var(--font-display);
  font-size: 18px;
  font-weight: 500;
  letter-spacing: -0.01em;
  margin-top: 2px;
  font-variation-settings: "opsz" 24, "SOFT" 30;
}
.new-btn {
  font-size: 11px;
  padding: 4px 10px;
  white-space: nowrap;
}

.sessions-list {
  flex: 1;
  overflow-y: auto;
  padding: 6px 0;
}
.session-item {
  position: relative;
  padding: 12px 36px 12px 18px;
  cursor: pointer;
  border-bottom: 1px solid var(--paper-edge);
  transition: padding-left 150ms ease, background 150ms ease;
}
.session-item:hover { background: rgba(255, 252, 244, 0.6); padding-left: 22px; }
.session-item.active {
  background: var(--ink);
  color: var(--paper);
  border-bottom-color: var(--ink);
}
.s-title {
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 500;
  letter-spacing: -0.005em;
  line-height: 1.3;
  font-variation-settings: "opsz" 18, "SOFT" 30;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.s-meta {
  margin-top: 4px;
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.04em;
  color: var(--ink-4);
  display: flex;
  gap: 6px;
  align-items: baseline;
}
.session-item.active .s-meta { color: rgba(245, 240, 230, 0.5); }
.s-meta-sep { opacity: 0.5; }
.s-del {
  position: absolute;
  right: 8px;
  top: 12px;
  background: none;
  border: none;
  color: var(--ink-4);
  cursor: pointer;
  font-size: 14px;
  line-height: 1;
  padding: 4px;
  opacity: 0;
  transition: opacity 130ms ease, color 130ms ease;
}
.session-item:hover .s-del { opacity: 1; }
.s-del:hover { color: var(--vermilion); }
.session-item.active .s-del { color: rgba(245, 240, 230, 0.5); }
.session-item.active .s-del:hover { color: var(--vermilion); }

/* ----- Right side: conversation ------------------------------- */
.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 0 28px;
}
.chat-toolbar {
  padding: 18px 0 14px;
  border-bottom: 1px solid var(--paper-edge);
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
}
.chat-title {
  font-family: var(--font-display);
  font-size: 22px;
  font-weight: 500;
  letter-spacing: -0.01em;
  margin-top: 4px;
  font-variation-settings: "opsz" 30, "SOFT" 30;
}
.sid-mark {
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--ink-4);
  letter-spacing: 0.05em;
}

.messages {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 26px;
  padding: 24px 0;
}

/* empty state */
.chat-empty {
  margin: auto;
  text-align: center;
}
.ce-glyph {
  font-family: var(--font-display);
  font-size: 80px;
  color: var(--paper-edge);
  line-height: 1;
  margin-bottom: 12px;
  font-variation-settings: "opsz" 144;
}
.ce-line {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 18px;
  color: var(--ink-2);
  font-variation-settings: "opsz" 24, "SOFT" 80;
  margin-bottom: 6px;
}
.ce-sub {
  font-family: var(--font-mono);
  font-size: 11px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--ink-4);
}

/* ----- Message bubbles ---------------------------------------- */
.msg { display: flex; }
.msg.user { justify-content: flex-end; }
.msg.assistant { justify-content: flex-start; }

.msg.user .bubble {
  max-width: 78%;
  padding: 12px 18px;
  background: var(--ink);
  color: var(--paper);
  font-family: var(--font-body);
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  /* asymmetric: user is "stamped" on the right */
  border-radius: 0;
}

.assistant-wrap {
  position: relative;
  max-width: 100%;
  display: grid;
  /* 默认两列：A. 标记 + 气泡（窄屏走这套，followup-row 自动到 grid-column 2 走第二行底部横排） */
  grid-template-columns: 28px 1fr;
  gap: 10px;
  align-items: start;
}
/* 桌面宽屏：让出第三列给 follow-ups 纵向排，气泡占第二列 */
@media (min-width: 980px) {
  .assistant-wrap {
    grid-template-columns: 28px minmax(0, 1fr) minmax(180px, 260px);
  }
}

/* bubble-host：包气泡 + 复制按钮的容器（让 copy-btn 用绝对定位时
   不会跑到 assistant-wrap 的边角去；现在 wrap 因为 follow-ups 有时很高，
   原来 right:4px;bottom:-22px 会被推到对话最下方） */
.bubble-host {
  position: relative;
  min-width: 0;
}

/* 推理过程折叠块：HTML <details> 原生折叠，默认 closed。
   贴在气泡顶部，墨色细框 + mono 标签。展开后才展示工具调用列表。
   不抢答案视线（默认很小很灰），但用户想看 agent 怎么思考的随时可点开。 */
.reason-fold {
  margin-bottom: 8px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-4);
  border-left: 1px solid var(--paper-edge);
  padding-left: 10px;
}
.reason-fold[open] {
  border-left-color: var(--vermilion);
}
.reason-fold-summary {
  list-style: none;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
  user-select: none;
  outline: none;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  transition: color 130ms ease;
}
.reason-fold-summary::-webkit-details-marker { display: none; }
.reason-fold-summary:hover { color: var(--ink-2); }
.rf-arrow {
  display: inline-block;
  font-size: 10px;
  color: var(--vermilion);
  transition: transform 150ms ease;
}
.reason-fold[open] .rf-arrow { transform: rotate(90deg); }
.rf-text { font-size: 10px; }
.reason-fold-list {
  list-style: none;
  padding: 6px 0 4px;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.reason-fold-list li {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 11px;
  color: var(--ink-3);
}
.reason-fold-list .ti-arrow {
  color: var(--vermilion);
  font-family: var(--font-display);
  flex-shrink: 0;
}
.reason-fold-list .ti-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}
.msg-mark {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 18px;
  color: var(--vermilion);
  line-height: 1.4;
  font-variation-settings: "opsz" 24;
  padding-top: 1px;
  text-align: right;
}
.assistant-wrap .bubble {
  background: rgba(255, 252, 244, 0.4);
  border-left: 1.5px solid var(--paper-edge);
  padding: 4px 16px;
  font-family: var(--font-body);
  font-size: 14px;
  line-height: 1.7;
  color: var(--ink);
  word-break: break-word;
}
/* loading 状态：灰底框 + 3 跳点 + 底部流光，多层动画堆叠出「正在工作」感。
   设计意图：进行中需要明确视觉占位，但不让用户感到繁忙抖动。
   - 底色：var(--paper-2) 比常规气泡略深一档（与 wiki tag 一脉相承的浅墨）
   - 左侧 1.5px 朱红条做强调，表明「这是当前活动项」
   - 底部 1px 流光条横向扫，制造「时间线在推进」的感觉
   - 3 跳点（thinking-dots）依次起伏，传达「在反复思考」
   - 整框微脉冲 0.92↔1 加强心跳感
*/
.assistant-wrap .bubble.loading {
  position: relative;
  font-family: var(--font-display);
  font-style: italic;
  font-size: 14px;
  color: var(--ink-3);
  font-variation-settings: "opsz" 24, "SOFT" 100;
  background: var(--paper-2);
  border-left: 1.5px solid var(--vermilion);
  padding: 12px 22px 14px 18px;
  display: inline-flex;
  align-items: center;
  gap: 12px;
  overflow: hidden;
  animation: loadingPulse 1.6s ease-in-out infinite;
  min-width: 200px;
}
@keyframes loadingPulse {
  0%, 100% { opacity: 0.92; }
  50%      { opacity: 1; }
}

/* 中断按钮态：现在 .bubble.loading 是 <button>，重置浏览器默认 +
   hover 切到「× 点击中断」文案、整框泛朱红边、停掉脉冲（强化"动作焦点"语义） */
.assistant-wrap button.bubble.loading {
  appearance: none;
  border-top: 0;
  border-right: 0;
  border-bottom: 0;
  text-align: left;
  font: inherit;
  /* 默认态：仍用上面定义的样式；按钮额外要的就是 cursor */
  cursor: pointer;
  transition: border-color 160ms ease, background 160ms ease, transform 130ms ease;
}
.assistant-wrap button.bubble.loading:hover {
  background: var(--paper);
  border-left-color: var(--vermilion);
  border-left-width: 2.5px;
  /* hover 时停掉脉冲——告诉用户"焦点已锁定，准备执行动作" */
  animation-play-state: paused;
  opacity: 1;
  transform: translateX(-1px);
}
.assistant-wrap button.bubble.loading:active {
  transform: translateX(0) scale(0.98);
}
/* hover 时切换两套 thinking-text：默认隐藏 hover 文案，hover 时反过来 */
.thinking-text-hover { display: none; color: var(--vermilion); }
.assistant-wrap button.bubble.loading:hover .thinking-text-default { display: none; }
.assistant-wrap button.bubble.loading:hover .thinking-text-hover { display: inline; font-style: normal; letter-spacing: 0.04em; }
/* hover 时朱红跳点：强化"危险/中断"色调 */
.assistant-wrap button.bubble.loading:hover .thinking-dots span {
  background: var(--vermilion);
}

/* 3 跳点：经典 chat 思考动画 */
.thinking-dots {
  display: inline-flex;
  align-items: flex-end;
  gap: 4px;
  height: 14px;
}
.thinking-dots span {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--vermilion);
  display: inline-block;
  animation: thinkingDotJump 1.1s ease-in-out infinite;
}
.thinking-dots span:nth-child(2) { animation-delay: 0.18s; background: var(--ink-2); }
.thinking-dots span:nth-child(3) { animation-delay: 0.36s; background: var(--ink-3); }
@keyframes thinkingDotJump {
  0%, 60%, 100% { transform: translateY(0);    opacity: 0.4; }
  30%           { transform: translateY(-6px); opacity: 1; }
}

.thinking-text {
  letter-spacing: 0.01em;
  /* 文案颜色稍深、保持斜体的纸感 */
}

/* 底部 1px 流光：横向扫动表示"时间线在推进" */
.thinking-shimmer {
  position: absolute;
  left: 0;
  bottom: 0;
  height: 1px;
  width: 40%;
  background: linear-gradient(
    90deg,
    transparent 0%,
    var(--vermilion) 50%,
    transparent 100%
  );
  animation: shimmerSlide 1.6s linear infinite;
}
@keyframes shimmerSlide {
  0%   { transform: translateX(-100%); }
  100% { transform: translateX(350%); }
}

/* 复制按钮：贴在 bubble-host 右下角内侧（hover 才显示）。
   位置不再依赖 assistant-wrap 的总高度，跟随气泡走。 */
.copy-btn {
  position: absolute;
  right: 6px;
  bottom: 4px;
  appearance: none;
  background: rgba(245, 240, 230, 0.85);
  border: 0;
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-4);
  cursor: pointer;
  opacity: 0;
  transition: opacity 130ms ease, color 130ms ease, background 130ms ease;
  padding: 3px 7px;
  pointer-events: none;
}
.bubble-host:hover .copy-btn { opacity: 1; pointer-events: auto; }
.copy-btn:hover { color: var(--vermilion); background: var(--paper); }

/* loading glyph (shared with WikiView) */
.loading-glyph {
  display: inline-block;
  font-family: var(--font-mono);
  color: var(--vermilion);
  letter-spacing: 0;
  margin-right: 6px;
  animation: typewriter 0.9s steps(4) infinite;
}
@keyframes typewriter {
  0%   { opacity: 0.2; }
  50%  { opacity: 1;   }
  100% { opacity: 0.2; }
}

/* =================================================================
 * 输入区上方的实时状态条 — 与上方 loading 气泡联动
 *
 * 出现在 input-bar 上方；querying 时跑同款 3 跳点 + 流光，done 后
 * 短暂闪一行 "✓ 答案已就绪" 再淡出。
 * 用户视线下移到输入框时也能感知后台进行/完成状态。
 * ================================================================= */
.chat-status-bar {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 14px;
  margin: 0;
  font-family: var(--font-mono);
  font-size: 11px;
  letter-spacing: 0.04em;
  color: var(--ink-3);
  border-top: 1px solid var(--paper-edge);
  overflow: hidden;
  background: var(--paper-2);
}
.chat-status-bar.working {
  background: linear-gradient(180deg, var(--paper-2), var(--paper));
}
.chat-status-bar.done {
  color: var(--moss);
  background: linear-gradient(180deg, rgba(91, 111, 63, 0.08), transparent);
}

.csb-dots {
  display: inline-flex;
  align-items: flex-end;
  gap: 3px;
  height: 10px;
}
.csb-dots span {
  width: 4px; height: 4px; border-radius: 50%;
  background: var(--vermilion);
  animation: thinkingDotJump 1.1s ease-in-out infinite;
}
.csb-dots span:nth-child(2) { animation-delay: 0.18s; background: var(--ink-2); }
.csb-dots span:nth-child(3) { animation-delay: 0.36s; background: var(--ink-3); }

.csb-text {
  flex: 1;
  font-style: italic;
  font-family: var(--font-display);
  font-variation-settings: "opsz" 14, "SOFT" 80;
  font-size: 12px;
  letter-spacing: 0;
}
.csb-check {
  font-family: var(--font-display);
  font-size: 14px;
  color: var(--moss);
}

.csb-shimmer {
  position: absolute;
  left: 0; bottom: 0;
  height: 1px; width: 30%;
  background: linear-gradient(90deg, transparent, var(--vermilion), transparent);
  animation: shimmerSlide 1.6s linear infinite;
}

/* 进出场动画 */
.status-fade-enter-active, .status-fade-leave-active {
  transition: opacity 250ms ease, transform 250ms ease, max-height 250ms ease;
  max-height: 60px;
}
.status-fade-enter-from, .status-fade-leave-to {
  opacity: 0;
  transform: translateY(4px);
  max-height: 0;
}

/* ----- Input ---------------------------------------------------- */
.input-bar {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  padding: 16px 0 18px;
  border-top: 1.5px solid var(--ink);
  background:
    linear-gradient(180deg, transparent, rgba(245, 240, 230, 0.6));
}
.input-bar textarea {
  flex: 1;
  resize: none;
  font-family: var(--font-body);
  font-size: 14px;
  line-height: 1.55;
  border: 1px solid var(--paper-edge);
  background: rgba(255, 252, 244, 0.5);
  padding: 10px 14px;
  min-height: 56px;
}
.input-bar textarea:focus { border-color: var(--ink); background: var(--paper); }
.send-btn {
  white-space: nowrap;
  height: fit-content;
  padding: 9px 22px;
}

/* override the dotted wikilink style inside chat bubbles to match the rest */
.bubble :deep(.wikilink) {
  color: var(--indigo);
  cursor: pointer;
  text-decoration: none;
  border-bottom: 1px dotted var(--indigo);
  padding-bottom: 1px;
}
.bubble :deep(.wikilink:hover) {
  background: var(--indigo);
  color: var(--paper);
}

/* =================================================================
 * Suggestion cards — first-screen "where do I start?" affordance
 *
 * 视觉：四张并排的 index card（便签卡），编辑部抽屉里抽出来的题目卡。
 * 行为：hover 上抬 + 朱红描边；点击直接发送。
 * ================================================================= */
.suggest-block {
  margin-top: 36px;
  text-align: left;
  max-width: 720px;
  width: 100%;
  margin-left: auto;
  margin-right: auto;
}
.suggest-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 12px;
  padding: 0 4px;
}
.suggest-label {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 13px;
  color: var(--ink-3);
  font-variation-settings: "opsz" 18, "SOFT" 80;
}
.suggest-refresh {
  appearance: none;
  background: none;
  border: 0;
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--ink-4);
  cursor: pointer;
  padding: 4px 6px;
  transition: color 130ms ease;
}
.suggest-refresh:hover { color: var(--vermilion); }
.suggest-refresh:disabled { opacity: 0.4; cursor: not-allowed; }

.suggest-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
}
@media (min-width: 980px) {
  .suggest-grid { grid-template-columns: repeat(4, 1fr); }
}

.suggest-card {
  appearance: none;
  text-align: left;
  background: var(--paper);
  border: 1px solid var(--paper-edge);
  padding: 14px 14px 12px;
  cursor: pointer;
  font-family: var(--font-body);
  font-size: 13px;
  color: var(--ink-2);
  line-height: 1.5;
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 100px;
  position: relative;
  transition:
    transform 180ms cubic-bezier(0.2, 0.7, 0.2, 1),
    border-color 180ms ease,
    box-shadow 180ms ease,
    color 180ms ease;
}
.suggest-card:hover {
  transform: translateY(-2px);
  border-color: var(--ink);
  color: var(--ink);
  box-shadow: 3px 3px 0 0 var(--vermilion);
}
.suggest-card:active { transform: translateY(0); box-shadow: 1px 1px 0 0 var(--vermilion); }

.sg-num {
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
  color: var(--ink-4);
}
.suggest-card:hover .sg-num { color: var(--vermilion); }
.sg-text {
  flex: 1;
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 500;
  letter-spacing: -0.005em;
  color: inherit;
  font-variation-settings: "opsz" 18, "SOFT" 30;
  /* 多行省略：避免长问题撑爆卡片 */
  display: -webkit-box;
  -webkit-line-clamp: 4;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.sg-arrow {
  font-family: var(--font-display);
  font-size: 14px;
  color: var(--ink-4);
  align-self: flex-end;
  transition: color 180ms ease, transform 180ms ease;
}
.suggest-card:hover .sg-arrow {
  color: var(--vermilion);
  transform: translateX(3px);
}

.suggest-card.skeleton {
  cursor: default;
  pointer-events: none;
  align-items: center;
  justify-content: center;
  color: var(--ink-4);
  background:
    linear-gradient(120deg,
      var(--paper) 30%,
      var(--paper-2) 50%,
      var(--paper) 70%);
  background-size: 200% 100%;
  animation: skeletonShimmer 1.6s linear infinite;
}
@keyframes skeletonShimmer {
  0%   { background-position: 100% 0; }
  100% { background-position: -100% 0; }
}

/* =================================================================
 * 右栏 .rightcol — 思考过程 + 跟进推荐共享空间
 *
 * 桌面 (>=980px)：占 assistant-wrap 的第三列，sticky 跟随滚动。
 * 窄屏：退回到第二列下方（跟原 followup-row 的窄屏行为一致）。
 * 内部纵向排：trace-block (tools 时间轴) → trace-block (mini-graph) → followup-row
 * ================================================================= */
.rightcol {
  /* 默认（窄屏）：跳过左 28px "A." 标记列，对齐 bubble */
  grid-column: 2 / 3;
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-top: 14px;
  padding-left: 16px;
}
@media (min-width: 980px) {
  .rightcol {
    grid-column: 3 / 4;
    grid-row: 1 / 2;
    margin-top: 0;
    padding-left: 0;
    align-self: start;
    padding-top: 4px;

    /*
     * sticky 跟随滚动，但不再做内部 overflow——
     * 之前 max-height + overflow-y:auto 让滚轮事件被吞，导致用户
     * 在右栏区滑动时主消息流不动 + 长内容下方的 follow-up 难触达。
     * 现在让右栏自然撑高：
     *   - 内容矮（小于视口）：sticky 粘在 top，用户主滚动时它跟着停留
     *   - 内容高（超过视口）：sticky 失效退化为正常块，主滚动条一路看到底
     * 这是 ChatGPT/Claude 那种侧栏的成熟做法：宁可放弃严格的"始终可见"，
     * 也要保住"什么都能滚到"的可触达性。
     */
    position: sticky;
    top: 12px;
  }
}

/* trace-block：tools 时间轴 / mini-graph 都用同一外壳（label + 内容） */
.trace-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.trace-label {
  font-family: var(--font-mono);
  font-size: 9px;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ink-4);
  padding-left: 2px;
}

/* tools 时间轴：每一项 → 一条 read_file(...) 调用 */
.trace-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
  border-left: 1px solid var(--paper-edge);
  padding-left: 10px;
}
.trace-item {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-3);
  /* 入场动画：每条新工具调用淡入 */
  animation: traceFadeIn 220ms ease;
}
@keyframes traceFadeIn {
  from { opacity: 0; transform: translateX(-4px); }
  to   { opacity: 1; transform: translateX(0); }
}
.ti-arrow {
  color: var(--vermilion);
  font-family: var(--font-display);
  flex-shrink: 0;
}
.ti-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}

/* =================================================================
 * Follow-up chips
 * .rightcol 已负责 sticky / 列位置；followup-row 退化为简单 flex 容器
 * ================================================================= */
.followup-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: baseline;
}
@media (min-width: 980px) {
  .followup-row {
    flex-direction: column;
    align-items: stretch;
  }
  .followup-row .fu-label {
    margin-bottom: 2px;
    margin-right: 0;
    padding-left: 2px;
  }
  .followup-row .followup-chip {
    width: 100%;
    justify-content: space-between;
  }
  .followup-row .fu-text {
    white-space: normal;
    max-width: none;
    line-height: 1.4;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    text-align: left;
  }
}
.fu-label {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 12px;
  color: var(--ink-4);
  font-variation-settings: "opsz" 14, "SOFT" 80;
  margin-right: 2px;
  white-space: nowrap;
}
.followup-chip {
  appearance: none;
  background: var(--paper);
  border: 1px solid var(--paper-edge);
  padding: 6px 12px;
  cursor: pointer;
  font-family: var(--font-body);
  font-size: 12.5px;
  color: var(--ink-2);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  transition:
    border-color 150ms ease,
    color 150ms ease,
    transform 150ms ease,
    box-shadow 150ms ease;
  /* 入场动画：略微淡入上滑，告诉用户这是新出现的 */
  animation: fuPop 280ms cubic-bezier(0.2, 0.7, 0.2, 1) both;
}
@keyframes fuPop {
  from { opacity: 0; transform: translateY(4px); }
  to   { opacity: 1; transform: translateY(0); }
}
/* 错峰显示，更自然 */
.followup-chip:nth-child(2) { animation-delay: 60ms; }
.followup-chip:nth-child(3) { animation-delay: 120ms; }
.followup-chip:nth-child(4) { animation-delay: 180ms; }

.followup-chip:hover {
  border-color: var(--ink);
  color: var(--ink);
  transform: translateY(-1px);
  box-shadow: 2px 2px 0 0 var(--vermilion);
}
.followup-chip:active {
  transform: translateY(0);
  box-shadow: 1px 1px 0 0 var(--vermilion);
}
.fu-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 320px;
}
.fu-arrow {
  font-family: var(--font-display);
  color: var(--ink-4);
  font-size: 12px;
  transition: color 150ms ease, transform 150ms ease;
}
.followup-chip:hover .fu-arrow {
  color: var(--vermilion);
  transform: translateX(2px);
}

</style>
