<script setup lang="ts">
import { ref, watch } from 'vue'
import { api } from '../api/client'
import type { VPNNode } from '../types'

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'selected', node: VPNNode): void
}>()

const nodes = ref<VPNNode[]>([])
const nodesError = ref('')
const nodesBusy = ref(false)
const pinging = ref(false)

watch(() => props.modelValue, (visible) => {
  if (visible) loadNodes()
})

async function loadNodes() {
  nodesBusy.value = true
  nodesError.value = ''
  try {
    const res = await api.listVPNNodes()
    if (!res.ok) { nodesError.value = res.error || '获取节点失败'; return }
    nodes.value = (res.nodes || []).map(n => ({ ...n, latency_ms: -1 }))
    pingAllStreaming()
  } catch (e) {
    nodesError.value = String(e)
  } finally {
    nodesBusy.value = false
  }
}

function pingAllStreaming() {
  if (!nodes.value.length) return
  pinging.value = true
  let done = 0
  nodes.value.forEach((node, i) => {
    api.pingSingleNode(node.id).then(pr => {
      if (pr.ok && pr.nodes && pr.nodes[0]) {
        nodes.value[i] = { ...nodes.value[i], latency_ms: pr.nodes[0].latency_ms }
      }
    }).catch(() => {}).finally(() => {
      done++
      if (done === nodes.value.length) pinging.value = false
    })
  })
}

async function pingNodes() {
  pinging.value = true
  nodes.value = nodes.value.map(n => ({ ...n, latency_ms: -1 }))
  let done = 0
  nodes.value.forEach((node, i) => {
    api.pingSingleNode(node.id).then(pr => {
      if (pr.ok && pr.nodes && pr.nodes[0]) {
        nodes.value[i] = { ...nodes.value[i], latency_ms: pr.nodes[0].latency_ms }
      }
    }).catch(() => {}).finally(() => {
      done++
      if (done === nodes.value.length) pinging.value = false
    })
  })
}

function selectNode(node: VPNNode) {
  emit('selected', node)
  emit('update:modelValue', false)
}

function latencyLabel(ms: number): string {
  if (ms < 0) return '测速中...'
  if (ms === 0) return '—'
  return `${ms} ms`
}

function latencyClass(ms: number): string {
  if (ms < 0) return 'lat-pinging'
  if (ms === 0) return 'lat-unknown'
  if (ms < 100) return 'lat-good'
  if (ms < 300) return 'lat-ok'
  return 'lat-bad'
}
</script>

<template>
  <n-modal
    :show="props.modelValue"
    preset="card"
    title="选择节点"
    style="width: 440px; max-height: 540px"
    @update:show="(v: boolean) => emit('update:modelValue', v)"
  >
    <template #header-extra>
      <n-button size="small" :loading="pinging" @click="pingNodes">测速</n-button>
    </template>

    <n-spin :show="nodesBusy">
      <n-alert v-if="nodesError" type="error" :show-icon="false" class="mb-8">
        {{ nodesError }}
      </n-alert>

      <div v-if="!nodes.length && !nodesBusy" class="empty-hint">
        暂无节点，请检查账号或网络
      </div>

      <n-list hoverable clickable class="nodes-list">
        <n-list-item
          v-for="node in nodes"
          :key="node.id"
          class="node-item"
        >
          <div class="node-info">
            <strong>{{ node.name }}</strong>
            <span :class="['node-latency', latencyClass(node.latency_ms)]">
              {{ latencyLabel(node.latency_ms) }}
            </span>
          </div>
          <template #suffix>
            <n-button
              type="primary"
              size="small"
              @click="selectNode(node)"
            >
              选择
            </n-button>
          </template>
        </n-list-item>
      </n-list>
    </n-spin>
  </n-modal>
</template>

<style scoped>
.nodes-list {
  max-height: 360px;
  overflow-y: auto;
}

.node-item {
  padding: 8px 0;
}

.node-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.node-info strong {
  font-size: 14px;
}

.node-latency {
  font-size: 12px;
  font-weight: 500;
}

.lat-good { color: #18a058; }
.lat-ok   { color: #e6a23c; }
.lat-bad  { color: #d03050; }
.lat-unknown { color: #aaa; }
.lat-pinging { color: #aaa; font-style: italic; }

.empty-hint {
  text-align: center;
  color: #aaa;
  padding: 32px 0;
}

.mb-8 {
  margin-bottom: 8px;
}
</style>
