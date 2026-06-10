<script setup lang="ts">
import { computed, ref } from 'vue'
import { Plus, Search, Trash2 } from '@lucide/vue'
import { VueDraggable } from 'vue-draggable-plus'
import { useAppStore } from '../stores/app'
import RuleEditor from './RuleEditor.vue'
import type { RuleConfig } from '../types'

const store = useAppStore()
const query = ref('')
const policy = ref<string | null>(null)
const editing = ref<RuleConfig | null>(null)
const pendingRule = ref<RuleConfig | null>(null)
const showEditor = ref(false)
const saving = ref(false)
const saveText = ref('')
let saveTimer: number | undefined

const rules = computed({
  get: () => store.config?.rules || [],
  set: (value) => {
    if (store.config) store.config.rules = value
  }
})

const filtered = computed(() => {
  const q = query.value.toLowerCase()
  return rules.value.filter((rule) => {
    const text = [rule.type, rule.value, rule.policy].join(' ').toLowerCase()
    return (!q || text.includes(q)) && (!policy.value || rule.policy === policy.value)
  })
})
const hasFilter = computed(() => Boolean(query.value.trim() || policy.value))

function addRule() {
  query.value = ''
  policy.value = null
  pendingRule.value = {
    id: `rule_${Date.now()}`,
    enabled: true,
    type: 'DOMAIN-SUFFIX',
    value: '',
    policy: 'DIRECT'
  }
  editing.value = cloneRule(pendingRule.value)
  showEditor.value = true
}

function editRule(rule: RuleConfig) {
  editing.value = cloneRule(rule)
  showEditor.value = true
}

async function saveRule(rule: RuleConfig) {
  if (!store.config) return
  const index = store.config.rules.findIndex((item) => item.id === rule.id)
  if (index >= 0) store.config.rules[index] = rule
  else store.config.rules.push(rule)
  pendingRule.value = null
  showEditor.value = false
  await persistRules()
}

async function deleteRule(rule: RuleConfig) {
  if (!store.config) return
  if (pendingRule.value?.id === rule.id) {
    pendingRule.value = null
    if (editing.value?.id === rule.id) {
      editing.value = null
      showEditor.value = false
    }
    return
  }
  store.config.rules = store.config.rules.filter((item) => item.id !== rule.id)
  await persistRules()
}

function setEditorVisible(value: boolean) {
  showEditor.value = value
  if (!value && pendingRule.value && editing.value?.id === pendingRule.value.id) {
    pendingRule.value = null
    editing.value = null
  }
}

function queuePersistRules() {
  if (saveTimer) window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => {
    persistRules()
  }, 250)
}

async function persistRules() {
  if (!store.config) return
  saving.value = true
  saveText.value = '正在保存...'
  const ok = await store.saveConfig()
  saving.value = false
  saveText.value = ok ? '已自动保存，重启 TUN 后生效' : '保存失败'
}

function cloneRule(rule: RuleConfig): RuleConfig {
  return JSON.parse(JSON.stringify(rule)) as RuleConfig
}
</script>

<template>
  <section class="page-stack">
    <div class="toolbar">
      <n-input v-model:value="query" placeholder="搜索规则">
        <template #prefix><Search :size="16" /></template>
      </n-input>
      <n-select v-model:value="policy" clearable placeholder="策略" :options="['DIRECT', 'VPN'].map((v) => ({ label: v, value: v }))" />
      <n-button type="primary" @click="addRule"><Plus :size="16" />新增</n-button>
    </div>
    <div class="rule-save-line">
      <span>{{ saveText || '规则修改会自动保存，重启 TUN 后生效' }}</span>
      <n-tag v-if="saving" size="small">保存中</n-tag>
      <n-tag v-else-if="store.lastError" type="error" size="small">失败</n-tag>
      <n-tag v-else-if="saveText" type="success" size="small">已保存</n-tag>
      <n-tag v-if="hasFilter" size="small">筛选时不可拖拽</n-tag>
    </div>

    <div class="rule-list">
      <template v-if="!hasFilter">
        <VueDraggable v-model="rules" handle=".drag-handle" @end="persistRules">
          <div v-for="rule in rules" :key="rule.id" class="rule-row">
            <span class="drag-handle">::</span>
            <n-switch v-model:value="rule.enabled" @update:value="queuePersistRules" />
            <n-tag size="small">{{ rule.type }}</n-tag>
            <button class="rule-main" @click="editRule(rule)">
              <strong>{{ rule.value || '未填写' }}</strong>
              <span>{{ rule.type }} · {{ rule.policy }}</span>
            </button>
            <n-tag :type="rule.policy === 'DIRECT' ? 'info' : 'default'">{{ rule.policy }}</n-tag>
            <n-button quaternary circle @click="deleteRule(rule)"><Trash2 :size="16" /></n-button>
          </div>
        </VueDraggable>
        <div v-if="pendingRule" class="rule-row draft">
          <span class="drag-handle disabled">::</span>
          <n-switch v-model:value="pendingRule.enabled" />
          <n-tag size="small">{{ pendingRule.type }}</n-tag>
          <button class="rule-main" @click="editRule(pendingRule)">
            <strong>{{ pendingRule.value || '未填写' }}</strong>
            <span>{{ pendingRule.type }} · {{ pendingRule.policy }}</span>
          </button>
          <n-tag type="warning">未保存</n-tag>
          <n-button quaternary circle @click="deleteRule(pendingRule)"><Trash2 :size="16" /></n-button>
        </div>
      </template>
      <div v-else class="rule-list">
        <div v-for="rule in filtered" :key="rule.id" class="rule-row">
          <span class="drag-handle disabled">::</span>
          <n-switch v-model:value="rule.enabled" @update:value="queuePersistRules" />
          <n-tag size="small">{{ rule.type }}</n-tag>
          <button class="rule-main" @click="editRule(rule)">
            <strong>{{ rule.value || '未填写' }}</strong>
            <span>{{ rule.type }} · {{ rule.policy }}</span>
          </button>
          <n-tag :type="rule.policy === 'DIRECT' ? 'info' : 'default'">{{ rule.policy }}</n-tag>
          <n-button quaternary circle @click="deleteRule(rule)"><Trash2 :size="16" /></n-button>
        </div>
      </div>
    </div>

    <RuleEditor :show="showEditor" :rule="editing" @update:show="setEditorVisible" @save="saveRule" />
  </section>
</template>
