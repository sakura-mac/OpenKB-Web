// 跨 tab 切换保持 chat 状态：按 space.name 隔离，QueryView 卸载/重建后状态依然存在。
//
// 持久化的不只是消息历史，还包括「正在进行中的一轮推理」相关的瞬时状态：
//   - querying    : 是否在等 LLM
//   - thinkingMsg : 进度文案（"connecting" / "thinking 12s"）
//   - followUps   : 跟进推荐（done 后才到）
//   - abortCtrl   : 当前 fetch 的 AbortController（用于强制中断；目前不主动 abort）
//
// 这样用户在 stream 进行中切到「知识/图谱」再切回来，能看到答案继续在长，
// follow-ups 也能正常出现，而不是「assistant bubble 自己写但 UI 没反应」的诡异状态。
import { reactive } from 'vue'

export interface ChatMessage { role: 'user' | 'assistant'; content: string }

interface SpaceChatState {
  currentSid: string
  currentTitle: string
  messages: ChatMessage[]
  // —— 进行中的一轮（无活跃 turn 时全部清零）——
  querying: boolean
  thinkingMsg: string
  followUps: string[]
  /** 当前 fetch 的 AbortController；切空间/重置时可以用它打断流。 */
  abortCtrl: AbortController | null
  /** 标识本轮 turn 在 messages 里的下标（流式 delta 写入用），无活跃轮 = -1。 */
  activeAsstIdx: number
  /** 流式累积字数：done 之前 token 不渲染到 messages，只在 loading 框里显示进度。 */
  streamingChars: number
  /** 本轮（最新一轮）的「思考过程」追踪：
   *  - tools : 后端推的 tool 事件按顺序列表（如 read_file(...)）
   *  - links : 答案 done 后从文本提取的 wikilinks（[[category/slug]]）
   *  仅保留最新一轮：切换 session / 新一轮 doSend 都重置；
   *  历史消息不需要，避免持久化结构臃肿。 */
  trace: {
    tools: string[]
    links: Array<{ category: string; name: string }>
  }
  /** 按 assistant 消息下标存的逐轮思考过程。
   *  作用：让用户回看历史 assistant 消息时，每条都能看到自己那轮的工具时间轴 + 知识子图，
   *  而不是只有最新一条有图谱。
   *  - key   : messages 数组里 assistant 的 index
   *  - value : { tools, links } 同 trace 结构
   *  写入时机：doSend 累积 tool 事件 + done 时算 links；与 trace 同步更新（trace 仍指向「最新轮」用于工具时间轴的实时增长）。
   *  生命周期：openSession / newSession / 切 space 时清空；activeAsstIdx 之后的 idx 不应残留。
   *  没存进 traces 的历史轮（来自 chatLoadSession 的回填消息）会在模板侧降级为即时 extractWikilinks(m.content)。 */
  traces: Record<number, {
    tools: string[]
    links: Array<{ category: string; name: string }>
  }>
}

// 用 reactive 而非 ref<Record>：让嵌套字段也响应式。
const states = reactive<Record<string, SpaceChatState>>({})

function makeState(): SpaceChatState {
  return {
    currentSid: '',
    currentTitle: '',
    messages: [],
    querying: false,
    thinkingMsg: '',
    followUps: [],
    abortCtrl: null,
    activeAsstIdx: -1,
    streamingChars: 0,
    trace: { tools: [], links: [] },
    traces: {},
  }
}

export function useChatState(spaceName: string): SpaceChatState {
  if (!states[spaceName]) {
    states[spaceName] = makeState()
  }
  return states[spaceName]
}

export function resetChatState(spaceName: string) {
  const s = states[spaceName]
  if (s?.abortCtrl) {
    try { s.abortCtrl.abort() } catch { /* ignore */ }
  }
  states[spaceName] = makeState()
}
