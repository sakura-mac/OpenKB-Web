<template>
  <div class="docs">
    <section class="ds-section">
      <header class="ds-header">
        <div>
          <div class="eyebrow">{{ t('docs.sectionA') }}</div>
          <h2 class="display ds-title">{{ t('docs.titleSources') }}</h2>
        </div>
        <div class="ds-actions">
          <button class="btn btn-sm" @click="showUrl = true">{{ t('docs.btnFromUrl') }}</button>
          <button class="btn btn-sm" @click="openBrowser()">{{ t('docs.btnFromServer') }}</button>
          <button class="btn btn-sm btn-primary" @click="showUpload = true">{{ t('docs.btnUpload') }}</button>
        </div>
      </header>

      <p v-if="space.docs.length === 0" class="empty-hint">
        {{ t('docs.emptyDocs') }}
      </p>
      <ol v-else class="doc-list">
        <li v-for="(d, i) in space.docs" :key="d.name" class="doc-row">
          <span class="dr-num">{{ String(i + 1).padStart(2, '0') }}</span>
          <span class="dr-name" :title="d.source_url || d.name">
            {{ d.display_name || d.name }}
            <a v-if="d.source_url" class="dr-src" :href="d.source_url" target="_blank" @click.stop>↗</a>
          </span>
          <span class="dr-size">{{ formatSize(d.size) }}</span>
          <button class="btn btn-ghost btn-sm dr-delete" @click="doRemove(d.name)">{{ t('common.delete') }}</button>
        </li>
      </ol>
    </section>

    <section class="ds-section">
      <header class="ds-header">
        <div>
          <div class="eyebrow">{{ t('docs.sectionB') }}</div>
          <h2 class="display ds-title">{{ t('docs.titleDecks') }}</h2>
        </div>
        <button class="btn btn-sm btn-primary" @click="showDeck = true">{{ t('docs.btnCompileDeck') }}</button>
      </header>

      <p v-if="decks.length === 0" class="empty-hint">{{ t('docs.emptyDecks') }}</p>
      <ol v-else class="doc-list">
        <li v-for="(d, i) in decks" :key="d.name" class="doc-row">
          <span class="dr-num">{{ String(i + 1).padStart(2, '0') }}</span>
          <span class="dr-name">{{ d.name }}</span>
          <span class="dr-size">{{ d.has_file ? formatSize(d.size) : t('docs.compiling') }}</span>
          <span v-if="d.has_file" class="dr-actions">
            <a class="btn btn-ghost btn-sm" :href="api.deckUrl(space.name, d.name)" target="_blank">{{ t('common.open') }}</a>
            <a class="btn btn-ghost btn-sm" :href="api.deckUrl(space.name, d.name, true)">{{ t('common.download') }}</a>
            <button class="btn btn-ghost btn-sm" @click="doDeleteDeck(d.name)">{{ t('common.delete') }}</button>
          </span>
        </li>
      </ol>
    </section>

    <!-- Upload modal -->
    <div v-if="showUpload" class="modal-overlay" @click.self="showUpload = false">
      <div class="modal" style="width:540px">
        <div class="eyebrow" style="margin-bottom:6px">{{ t('docs.uploadFormNo') }}</div>
        <h3>{{ t('docs.uploadTitle') }}</h3>

        <div
          class="upload-zone"
          :class="{ dragover: dragOver }"
          @dragover.prevent="dragOver = true"
          @dragleave="dragOver = false"
          @drop.prevent="onDrop"
        >
          <div class="uz-glyph">¶</div>
          <p class="uz-line">
            {{ t('docs.uploadDropLine') }}
            <label for="fileInput" class="uz-link">{{ t('docs.uploadBrowseLink') }}</label>
          </p>
          <p class="uz-sub">{{ t('docs.uploadHint') }}</p>
          <input id="fileInput" type="file" multiple style="display:none" @change="onFileSelect" />
        </div>

        <ul v-if="files.length" class="file-list">
          <li v-for="(f, i) in files" :key="i" class="file-item">
            <span class="fi-name">{{ f.name }}</span>
            <span class="fi-meta">{{ formatSize(f.size) }}</span>
            <button class="fi-x" @click="files.splice(i, 1)">{{ t('common.remove') }}</button>
          </li>
        </ul>

        <p class="form-note">{{ t('docs.uploadNote') }}</p>

        <div class="modal-actions">
          <button class="btn btn-ghost" @click="showUpload = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="!files.length" @click="doUpload">
            {{ t('docs.uploadSubmit') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Server browser -->
    <div v-if="showBrowser" class="modal-overlay" @click.self="showBrowser = false">
      <div class="modal" style="width:620px">
        <div class="eyebrow" style="margin-bottom:6px">{{ t('docs.browserFormNo') }}</div>
        <h3>{{ t('docs.browserTitle') }}</h3>

        <div class="browser-toolbar">
          <button class="btn btn-ghost btn-sm" @click="browseParent()">{{ t('common.up') }}</button>
          <span class="browser-path" :title="browserPath">{{ browserPath }}</span>
        </div>
        <div class="browser-list">
          <div
            v-for="item in browserItems" :key="item.name"
            :class="['browser-item', { 'is-dir': item.is_dir, 'is-selected': !item.is_dir && selectedPaths.has(browserPath + '/' + item.name) }]"
            @click="item.is_dir ? browseTo(browserPath + '/' + item.name) : toggleSelect(item)"
          >
            <span class="bi-mark">{{ item.is_dir ? '/' : '·' }}</span>
            <span class="bi-name">{{ item.name }}</span>
            <span v-if="!item.is_dir" class="bi-size">{{ formatSize(item.size) }}</span>
            <span v-if="!item.is_dir && selectedPaths.has(browserPath + '/' + item.name)" class="bi-check">✓</span>
          </div>
          <div v-if="browserItems.length === 0" class="empty-hint" style="padding:18px 0">{{ t('common.empty') }}</div>
        </div>

        <div class="modal-actions">
          <span class="form-note" style="flex:1; margin: 0">{{ t('docs.browserSelected', { n: selectedPaths.size }) }}</span>
          <button class="btn btn-ghost" @click="showBrowser = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="selectedPaths.size === 0" @click="addFromServer">{{ t('docs.browserAdd') }}</button>
        </div>
      </div>
    </div>

    <!-- URL modal -->
    <div v-if="showUrl" class="modal-overlay" @click.self="showUrl = false">
      <div class="modal" style="width:540px">
        <div class="eyebrow" style="margin-bottom:6px">{{ t('docs.urlFormNo') }}</div>
        <h3>{{ t('docs.urlTitle') }}</h3>

        <div class="form-group">
          <label>{{ t('docs.urlLabel') }}</label>
          <textarea
            v-model="urlInput" rows="5"
            :placeholder="t('docs.urlPlaceholder')"
          ></textarea>
        </div>
        <p class="form-note">{{ t('docs.urlNote') }}</p>

        <div class="modal-actions">
          <button class="btn btn-ghost" @click="showUrl = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="!urlInput.trim()" @click="addFromUrls">{{ t('common.submit') }}</button>
        </div>
      </div>
    </div>

    <!-- Deck modal -->
    <div v-if="showDeck" class="modal-overlay" @click.self="showDeck = false">
      <div class="modal" style="width:560px">
        <div class="eyebrow" style="margin-bottom:6px">{{ t('docs.deckFormNo') }}</div>
        <h3>{{ t('docs.deckTitle') }}</h3>

        <div class="form-group">
          <label>{{ t('docs.deckNameLabel') }}</label>
          <input v-model="deckName" type="text" :placeholder="t('docs.deckNamePlaceholder')" />
        </div>

        <div class="form-group">
          <label>{{ t('docs.deckIntentLabel') }}</label>
          <textarea
            v-model="deckIntent" rows="3"
            :placeholder="t('docs.deckIntentPlaceholder')"
          ></textarea>
        </div>

        <label class="checkbox-row">
          <input v-model="deckCritique" type="checkbox" />
          <span>{{ t('docs.deckCritique') }}</span>
        </label>

        <p v-if="deckError" class="error-text">{{ deckError }}</p>

        <div class="modal-actions">
          <button class="btn btn-ghost" @click="showDeck = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="creating" @click="doCreateDeck">
            {{ creating ? t('docs.deckSubmitting') : t('docs.deckSubmit') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../api'
import { useUpload } from '../composables/useUpload'
import type { SpaceDetail, DeckInfo } from '../types'

const { t } = useI18n()

const props = defineProps<{ space: SpaceDetail }>()
const emit = defineEmits<{ refresh: [] }>()

const showUpload = ref(false)
const files = ref<File[]>([])
const dragOver = ref(false)
const { startUpload, finishUpload, pollTask } = useUpload()

// Deck
const showDeck = ref(false)
const deckName = ref('')
const deckIntent = ref('')
const deckCritique = ref(false)
const deckError = ref('')
const creating = ref(false)
const decks = ref<DeckInfo[]>([])

async function loadDecks() {
  try {
    decks.value = await api.listDecks(props.space.name)
  } catch {
    decks.value = []
  }
}
onMounted(loadDecks)

async function doCreateDeck() {
  deckError.value = ''
  const name = deckName.value.trim()
  const intent = deckIntent.value.trim()
  if (!name) { deckError.value = t('docs.deckErrName'); return }
  if (!/^[a-zA-Z0-9_-]+$/.test(name)) { deckError.value = t('docs.deckErrSlug'); return }
  if (!intent) { deckError.value = t('docs.deckErrIntent'); return }

  creating.value = true
  const res = await api.createDeck(props.space.name, name, intent, deckCritique.value)
  creating.value = false

  if (!res.success || !res.task_id) {
    deckError.value = res.error || t('docs.deckErrSubmit')
    return
  }

  showDeck.value = false
  deckName.value = ''
  deckIntent.value = ''
  deckCritique.value = false

  const id = startUpload(1, t('docs.deckCompiling', { name }))
  pollTask(id, res.task_id, () => loadDecks())
}

async function doDeleteDeck(name: string) {
  if (!confirm(t('docs.deckDeleteConfirm', { name }))) return
  await api.deleteDeck(props.space.name, name)
  loadDecks()
}

function onDrop(e: DragEvent) {
  dragOver.value = false
  if (e.dataTransfer?.files) {
    for (const f of e.dataTransfer.files) files.value.push(f)
  }
}

function onFileSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files) {
    for (const f of input.files) files.value.push(f)
  }
}

async function doUpload() {
  const toUpload = [...files.value]
  showUpload.value = false
  files.value = []

  const id = startUpload(toUpload.length)
  try {
    const res = await api.uploadFiles(props.space.name, toUpload)
    if (res.success && res.task_id) {
      pollTask(id, res.task_id, () => emit('refresh'))
    } else {
      finishUpload(id, false, res.error || t('docs.taskFailUpload'))
    }
  } catch (e: any) {
    finishUpload(id, false, t('docs.taskNetworkErr', { e: e?.message || e }))
  }
}

async function doRemove(name: string) {
  if (!confirm(t('docs.taskRemoveConfirm', { name }))) return
  const slug = name.replace(/\.[^.]+$/, '')
  try {
    const res = await api.removeDoc(props.space.name, slug)
    if (!res.success || !res.task_id) {
      const id = startUpload(1, t('docs.taskRemoveFail', { name }))
      finishUpload(id, false, res.error || t('docs.taskUnknown'))
      return
    }
    const uiId = startUpload(1, t('docs.taskRemoveSubmitted', { name }))
    pollTask(uiId, res.task_id, () => emit('refresh'))
  } catch (e: any) {
    const id = startUpload(1, t('docs.taskRemoveFail', { name }))
    finishUpload(id, false, e?.message || t('docs.taskNetworkErr', { e: '' }))
  }
}

function formatSize(bytes: number) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

// ----- File Browser -----
const showBrowser = ref(false)
const browserPath = ref('')
const browserItems = ref<{ name: string; is_dir: boolean; size: number }[]>([])
const selectedPaths = ref<Set<string>>(new Set())

async function openBrowser() {
  showBrowser.value = true
  selectedPaths.value = new Set()
  await browseTo('')
}
async function browseTo(path: string) {
  const res = await api.browse(path)
  if (res.path) {
    browserPath.value = res.path
    browserItems.value = res.items || []
  }
}
function browseParent() {
  const parts = browserPath.value.split('/')
  parts.pop()
  browseTo(parts.join('/') || '/')
}
function toggleSelect(item: { name: string }) {
  const full = browserPath.value + '/' + item.name
  if (selectedPaths.value.has(full)) selectedPaths.value.delete(full)
  else selectedPaths.value.add(full)
  selectedPaths.value = new Set(selectedPaths.value)
}
async function addFromServer() {
  const paths = [...selectedPaths.value]
  showBrowser.value = false

  const id = startUpload(paths.length)
  let successCount = 0
  let errors: string[] = []

  for (const p of paths) {
    const res = await api.addDoc(props.space.name, p)
    if (res.success) successCount++
    else errors.push(res.error || p)
  }

  if (successCount > 0) {
    finishUpload(id, true, t('docs.taskFiles', { n: successCount }))
    emit('refresh')
  } else {
    finishUpload(id, false, errors[0] || t('docs.taskAddFail'))
  }
}

// ----- URL ingest -----
const showUrl = ref(false)
const urlInput = ref('')

async function addFromUrls() {
  const urls = urlInput.value.split('\n').map(s => s.trim()).filter(s => s.length > 0)
  if (!urls.length) return

  const bad = urls.find(u => !/^https?:\/\//i.test(u))
  if (bad) {
    alert(t('docs.urlBadProtocol', { u: bad }))
    return
  }

  showUrl.value = false
  urlInput.value = ''

  for (const u of urls) {
    try {
      const res = await api.addDoc(props.space.name, u) as any
      if (!res.success || !res.task_id) {
        const id = startUpload(1, t('docs.taskUrlReqFail', { u }))
        finishUpload(id, false, res.error || t('docs.taskUnknown'))
        continue
      }
      const uiId = startUpload(1, t('docs.taskUrlSubmitted', { label: shortLabel(u) }))
      pollTask(uiId, res.task_id, () => emit('refresh'))
    } catch (e: any) {
      const id = startUpload(1, t('docs.taskUrlSubmitFail', { u }))
      finishUpload(id, false, e?.message || t('docs.taskNetworkErr', { e: '' }))
    }
  }
}

function shortLabel(u: string): string {
  try {
    const url = new URL(u)
    const host = url.hostname.replace(/^www\./, '')
    const path = url.pathname.length > 30 ? url.pathname.slice(0, 30) + '...' : url.pathname
    return host + path
  } catch {
    return u.length > 50 ? u.slice(0, 50) + '...' : u
  }
}
</script>

<style scoped>
.docs { max-width: 980px; }

/* ----- Section header ------------------------------------------ */
.ds-section { margin-bottom: 56px; }
.ds-section:last-child { margin-bottom: 0; }
.ds-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 18px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--paper-edge);
}
.ds-title {
  font-size: 28px;
  font-weight: 500;
  margin-top: 4px;
  font-variation-settings: "opsz" 36, "SOFT" 30;
}
.ds-actions { display: flex; gap: 8px; align-items: center; }

/* ----- Document/deck list -------------------------------------- */
.doc-list { list-style: none; padding: 0; margin: 0; }
.doc-row {
  display: grid;
  grid-template-columns: 32px 1fr auto auto;
  gap: 18px;
  align-items: baseline;
  padding: 14px 4px 14px 0;
  border-bottom: 1px solid var(--paper-edge);
  transition: padding-left 150ms ease;
}
.doc-row:hover { padding-left: 8px; }
.doc-row:hover .dr-name { color: var(--vermilion); }
.dr-num {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-4);
  letter-spacing: 0.05em;
}
.dr-name {
  font-family: var(--font-display);
  font-size: 16px;
  letter-spacing: -0.005em;
  color: var(--ink);
  font-variation-settings: "opsz" 24, "SOFT" 20;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: color 150ms ease;
}
.dr-src {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-4);
  margin-left: 6px;
  text-decoration: none;
  vertical-align: super;
  transition: color 150ms ease;
}
.dr-src:hover { color: var(--vermilion); }
.dr-size {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-4);
  letter-spacing: 0.04em;
  white-space: nowrap;
}
.dr-actions { display: flex; gap: 4px; }
.dr-delete { color: var(--ink-4); }
.dr-delete:hover { color: var(--vermilion); }

/* ----- Upload zone --------------------------------------------- */
.upload-zone {
  margin-top: 16px;
  border: 1px dashed var(--ink-4);
  padding: 36px 24px;
  text-align: center;
  cursor: pointer;
  transition: border-color 200ms ease, background 200ms ease;
}
.upload-zone:hover, .upload-zone.dragover {
  border-color: var(--vermilion);
  background: rgba(200, 48, 46, 0.04);
}
.uz-glyph {
  font-family: var(--font-display);
  font-size: 56px;
  color: var(--paper-edge);
  line-height: 1;
  margin-bottom: 8px;
  font-variation-settings: "opsz" 144;
}
.upload-zone:hover .uz-glyph { color: var(--vermilion); }
.uz-line {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 17px;
  color: var(--ink-2);
  font-variation-settings: "opsz" 24, "SOFT" 80;
}
.uz-link {
  color: var(--ink);
  cursor: pointer;
  border-bottom: 1px solid var(--ink);
  font-style: normal;
  font-family: var(--font-body);
  font-size: 14px;
}
.uz-link:hover { color: var(--vermilion); border-color: var(--vermilion); }
.uz-sub {
  margin-top: 6px;
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--ink-4);
}

/* ----- File list (selected to upload) -------------------------- */
.file-list {
  list-style: none;
  padding: 0;
  margin: 16px 0 0;
  border-top: 1px solid var(--paper-edge);
}
.file-item {
  display: grid;
  grid-template-columns: 1fr auto auto;
  gap: 16px;
  align-items: baseline;
  padding: 8px 0;
  border-bottom: 1px solid var(--paper-edge);
  font-family: var(--font-mono);
  font-size: 12px;
}
.fi-name { color: var(--ink-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.fi-meta { color: var(--ink-4); }
.fi-x {
  appearance: none;
  background: none;
  border: 0;
  color: var(--vermilion);
  font-family: var(--font-body);
  font-size: 11px;
  cursor: pointer;
  text-decoration: underline;
}

/* ----- Browser inside modals ----------------------------------- */
.browser-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 8px 0;
}
.browser-path {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--ink-3);
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.browser-list {
  max-height: 320px;
  overflow-y: auto;
  border-top: 1px solid var(--paper-edge);
  border-bottom: 1px solid var(--paper-edge);
}
.browser-item {
  display: grid;
  grid-template-columns: 16px 1fr auto auto;
  gap: 12px;
  align-items: baseline;
  padding: 8px 4px;
  cursor: pointer;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--ink-2);
  border-bottom: 1px solid var(--paper-edge);
  transition: padding-left 120ms ease, color 120ms ease;
}
.browser-item:last-child { border-bottom: 0; }
.browser-item:hover { padding-left: 8px; color: var(--ink); }
.browser-item.is-dir { color: var(--ink); font-weight: 500; }
.browser-item.is-selected { color: var(--vermilion); }
.bi-mark {
  color: var(--ink-4);
  font-family: var(--font-display);
  text-align: center;
}
.browser-item.is-dir .bi-mark { color: var(--ink); }
.bi-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bi-size { color: var(--ink-4); font-size: 10px; }
.bi-check { color: var(--vermilion); font-weight: 600; }

/* ----- Form helpers -------------------------------------------- */
.form-note {
  font-family: var(--font-display);
  font-style: italic;
  font-size: 12px;
  color: var(--ink-3);
  margin-top: 12px;
  font-variation-settings: "opsz" 14;
}
.checkbox-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 6px 0 4px;
  font-family: var(--font-body);
  font-size: 13px;
  color: var(--ink-2);
  cursor: pointer;
}
.checkbox-row input[type="checkbox"] { accent-color: var(--vermilion); }
</style>
