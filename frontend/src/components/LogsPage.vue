<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const logView = ref<HTMLElement | null>(null)
const followTail = ref(true)
let timer: number | undefined

function isNearBottom(el: HTMLElement) {
  return el.scrollHeight - el.scrollTop - el.clientHeight < 24
}

async function refreshLog(forceTail = false) {
  const shouldFollow = forceTail || (logView.value ? isNearBottom(logView.value) : followTail.value)
  await store.readLog(300)
  await nextTick()
  if (logView.value && shouldFollow) {
    logView.value.scrollTop = logView.value.scrollHeight
  }
}

function onLogScroll() {
  if (!logView.value) return
  followTail.value = isNearBottom(logView.value)
}

onMounted(async () => {
  await refreshLog(true)
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
      <n-button @click="refreshLog(true)">刷新</n-button>
    </div>
    <pre ref="logView" class="log-view" @scroll="onLogScroll">{{ store.logChunk?.text || '暂无日志' }}</pre>
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
