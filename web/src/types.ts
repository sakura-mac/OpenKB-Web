export interface SpaceInfo {
  name: string
  path: string
  docs: number
  concepts: number
}

export interface DocInfo {
  name: string
  /** URL 抓取得到的人类可读标题，前端显示时优先于 name。 */
  display_name?: string
  /** URL 抓取的源 URL。 */
  source_url?: string
  size: number
  modified: number
}

export interface SpaceDetail {
  name: string
  path: string
  docs: DocInfo[]
  summaries: string[]
  concepts: string[]
  entities: string[]
  /** chat agent 在对话里临时写的对比/专题笔记，存在 wiki/explorations/ */
  explorations?: string[]
  /** raw-slug → 人类可读标题映射；让 summaries/concepts 列表也能渲染漂亮的标题。 */
  titles?: Record<string, string>
}

export interface TaskStatus {
  id: string
  space: string
  status: 'running' | 'done' | 'error'
  message: string
  files?: string[]
}

export interface GraphNode {
  id: string
  label: string
  type: 'doc' | 'concept' | 'entity'
}

export interface GraphEdge {
  source: string
  target: string
}

export interface GraphData {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface DeckInfo {
  name: string
  size: number
  modified: number
  has_file: boolean
}
