<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const logView = ref<HTMLElement | null>(null)
let timer: number | undefined

async function refreshLog() {
  await store.readLog(300)
  await nextTick()
  if (logView.value) {
    logView.value.scrollTop = logView.value.scrollHeight
  }
}

onMounted(async () => {
  await refreshLog()
  timer = window.setInterval(refreshLog, 1000)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <section class="page-stack">
    <div class="toolbar">
      <h2>日志</h2>
      <n-button @click="refreshLog">刷新</n-button>
    </div>
    <pre ref="logView" class="log-view">{{ store.logChunk?.text || '暂无日志' }}</pre>
  </section>
</template>

<style scoped>
.page-stack {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.log-view {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}
</style>
