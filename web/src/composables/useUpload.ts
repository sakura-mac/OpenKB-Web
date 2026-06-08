import { ref } from 'vue'
import { api } from '../api'

export interface UploadTask {
  id: number
  fileCount: number
  status: 'uploading' | 'done' | 'error'
  message: string
}

const tasks = ref<UploadTask[]>([])
let nextId = 1

export function useUpload() {
  function startUpload(fileCount: number, customMessage?: string): number {
    const id = nextId++
    tasks.value.push({ id, fileCount, status: 'uploading', message: customMessage || `正在上传并编译 ${fileCount} 个文件...` })
    return id
  }

  function updateMessage(id: number, msg: string) {
    const task = tasks.value.find(t => t.id === id)
    if (task) task.message = msg
  }

  function finishUpload(id: number, success: boolean, msg: string) {
    const task = tasks.value.find(t => t.id === id)
    if (task) {
      task.status = success ? 'done' : 'error'
      task.message = msg
      setTimeout(() => {
        tasks.value = tasks.value.filter(t => t.id !== id)
      }, 4000)
    }
  }

  // 轮询后端异步任务，直到 done/error。onDone 回调用于刷新视图。
  function pollTask(uiId: number, taskId: string, onDone?: () => void) {
    let count = 0
    const tick = () => {
      if (count++ >= 600) { // 最多 ~20 分钟（deck --critique 可能很长）
        finishUpload(uiId, false, '编译超时，请稍后刷新查看')
        return
      }
      setTimeout(async () => {
        try {
          const st = await api.getTask(taskId)
          if (st.status === 'done') {
            finishUpload(uiId, true, st.message || '编译完成')
            onDone?.()
            return
          }
          if (st.status === 'error') {
            finishUpload(uiId, false, st.message || '编译失败')
            onDone?.()
            return
          }
          // running：更新进度文案后继续轮询
          if (st.message) updateMessage(uiId, st.message)
        } catch {
          /* 网络抖动，继续重试 */
        }
        tick()
      }, 2000)
    }
    tick()
  }

  return { tasks, startUpload, updateMessage, finishUpload, pollTask }
}
