<template>
  <div class="code-view">
    <section class="code-hero">
      <div>
        <div class="eyebrow">CodeGraph · {{ space.indexed ? 'indexed' : 'not indexed' }}</div>
        <h2>{{ space.name }}</h2>
        <p>{{ space.path }}</p>
      </div>
      <button class="btn btn-sm" :disabled="syncing" @click="sync">
        {{ syncing ? '同步中…' : '同步索引' }}
      </button>
    </section>

    <section class="code-terminal">
      <div class="terminal-top">
        <span></span><span></span><span></span>
        <b>codegraph explore</b>
      </div>
      <textarea
        v-model="question"
        class="code-input"
        placeholder="问代码问题，例如：这个项目的请求链路如何到数据库？某个接口会影响哪些模块？"
        @keydown.meta.enter.prevent="ask"
        @keydown.ctrl.enter.prevent="ask"
      />
      <div class="code-actions">
        <button class="btn btn-primary" :disabled="asking || !question.trim()" @click="ask">
          {{ asking ? '分析中…' : '分析代码' }}
        </button>
        <span class="hint">⌘/Ctrl + Enter</span>
      </div>
    </section>

    <section v-if="error" class="code-error">{{ error }}</section>

    <section v-if="answer" class="code-answer">
      <div class="answer-head">CodeGraph Context</div>
      <pre>{{ answer }}</pre>
    </section>

    <section v-else class="code-empty">
      <div class="matrix">{ symbols · callers · routes · impact }</div>
      <p>CodeGraph 会直接读取本地代码图谱，返回相关符号、调用链和源码片段。</p>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { api } from '../api'
import type { CodeSpaceInfo } from '../types'

const props = defineProps<{ space: CodeSpaceInfo }>()
const emit = defineEmits<{ refresh: [] }>()

const question = ref('')
const answer = ref('')
const error = ref('')
const asking = ref(false)
const syncing = ref(false)

async function ask() {
  const q = question.value.trim()
  if (!q) return
  asking.value = true
  error.value = ''
  try {
    const res = await api.codeQuery(props.space.name, q)
    if (res.success) answer.value = res.answer || ''
    else error.value = res.error || 'CodeGraph 查询失败'
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    asking.value = false
  }
}

async function sync() {
  syncing.value = true
  error.value = ''
  try {
    const res = await api.codeSync(props.space.name)
    if (!res.success) error.value = res.error || '同步失败'
    emit('refresh')
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    setTimeout(() => { syncing.value = false; emit('refresh') }, 1400)
  }
}
</script>

<style scoped>
.code-view {
  display: grid;
  gap: 18px;
  max-width: 980px;
  margin: 0 auto;
}
.code-hero {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  align-items: flex-start;
  border: 1.5px solid var(--ink);
  background: #171411;
  color: #f5efe5;
  padding: 22px 24px;
  box-shadow: 8px 8px 0 rgba(0,0,0,.18);
}
.code-hero h2 {
  font-family: var(--font-display);
  font-size: 34px;
  margin: 4px 0 6px;
  font-style: italic;
}
.code-hero p { color: #bfb4a5; font-family: var(--font-mono); font-size: 12px; word-break: break-all; }
.code-terminal {
  border: 1.5px solid var(--ink);
  background: var(--paper-2);
}
.terminal-top {
  height: 34px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border-bottom: 1.5px solid var(--ink);
  font-family: var(--font-mono);
  font-size: 12px;
}
.terminal-top span { width: 9px; height: 9px; border-radius: 50%; background: var(--vermilion); display:inline-block; }
.terminal-top span:nth-child(2) { background: #d8a84a; }
.terminal-top span:nth-child(3) { background: #6f8a3b; }
.code-input {
  width: 100%;
  min-height: 132px;
  resize: vertical;
  border: 0;
  background: transparent;
  color: var(--ink);
  padding: 18px;
  font-family: var(--font-body);
  font-size: 16px;
  line-height: 1.7;
  outline: none;
}
.code-actions { display:flex; align-items:center; gap:12px; justify-content:flex-end; padding: 0 16px 16px; }
.hint { font-family: var(--font-mono); font-size: 11px; color: var(--ink-4); }
.code-error { border-left: 3px solid var(--vermilion); background: #fff2ed; padding: 12px 14px; color: var(--vermilion); white-space: pre-wrap; }
.code-answer { border: 1.5px solid var(--ink); background: #111; color: #eee; }
.answer-head { border-bottom: 1px solid #3a332d; padding: 10px 14px; color:#caa46a; font-family:var(--font-mono); font-size:12px; }
.code-answer pre { margin:0; padding:18px; white-space:pre-wrap; word-break:break-word; font-family:var(--font-mono); font-size:12.5px; line-height:1.65; }
.code-empty { text-align:center; padding: 60px 20px; color: var(--ink-3); }
.matrix { font-family: var(--font-mono); font-size: 13px; color: var(--vermilion); margin-bottom: 10px; }
</style>
