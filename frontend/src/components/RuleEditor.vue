<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { RuleConfig } from '../types'

const props = defineProps<{ show: boolean; rule: RuleConfig | null }>()
const emit = defineEmits<{ 'update:show': [value: boolean]; save: [rule: RuleConfig] }>()

const draft = ref<RuleConfig | null>(null)
const errorText = ref('')
const visible = computed({
  get: () => props.show,
  set: (value) => emit('update:show', value)
})

watch(
  () => props.rule,
  (rule) => {
    draft.value = rule ? cloneRule(rule) : null
    errorText.value = ''
  },
  { immediate: true }
)

function save() {
  if (!draft.value) return
  const error = validateRule(draft.value)
  if (error) {
    errorText.value = error
    return
  }
  errorText.value = ''
  emit('save', draft.value)
}

function validateRule(rule: RuleConfig): string {
  if (!rule.value.trim()) {
    return '请填写匹配值'
  }
  return ''
}

function cloneRule(rule: RuleConfig): RuleConfig {
  return JSON.parse(JSON.stringify(rule)) as RuleConfig
}
</script>

<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="规则编辑"
    class="rule-editor-modal"
    :bordered="false"
    :mask-closable="false"
  >
    <div class="rule-editor-body">
      <n-alert v-if="errorText" type="error" :bordered="false" class="editor-error">{{ errorText }}</n-alert>
      <n-form v-if="draft" label-placement="top" class="rule-editor-form">
        <n-form-item label="启用"><n-switch v-model:value="draft.enabled" /></n-form-item>
        <n-form-item label="类型">
          <n-select v-model:value="draft.type" :options="['DOMAIN', 'DOMAIN-SUFFIX', 'DOMAIN-KEYWORD', 'IP-CIDR', 'GEOIP'].map((v) => ({ label: v, value: v }))" />
        </n-form-item>
        <n-form-item label="匹配值"><n-input v-model:value="draft.value" /></n-form-item>
        <n-form-item label="策略">
          <n-select v-model:value="draft.policy" :options="['DIRECT', 'VPN'].map((v) => ({ label: v, value: v }))" />
        </n-form-item>

      </n-form>
      <n-alert v-else type="error" :bordered="false">规则数据为空，请关闭后重新打开。</n-alert>
    </div>
    <template #footer>
      <div class="modal-actions">
        <n-button @click="visible = false">取消</n-button>
        <n-button type="primary" @click="save">保存</n-button>
      </div>
    </template>
  </n-modal>
</template>
