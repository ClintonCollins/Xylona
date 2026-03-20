<template>
  <div class="advisory-list">
    <div class="advisory-header">
      <div class="advisory-tabs">
        <button class="advisory-tab" :class="{ active: filter === 'all' }" @click="filter = 'all'">
          All
          <span v-if="totalCount > 0" class="advisory-tab-count">{{ totalCount }}</span>
        </button>
        <button
          class="advisory-tab"
          :class="{ active: filter === 'unread' }"
          @click="filter = 'unread'">
          Unread
          <span v-if="unreadCount > 0" class="advisory-tab-count advisory-tab-count--unread">{{
            unreadCount
          }}</span>
        </button>
      </div>
      <q-btn
        v-if="unreadCount > 0"
        flat
        dense
        color="primary"
        label="Mark all read"
        icon="done_all"
        @click="markAllRead" />
    </div>

    <div v-if="loading" class="advisory-loading">
      <q-spinner color="primary" size="2rem" />
    </div>

    <div v-else-if="filteredAdvisories.length === 0" class="advisory-empty">
      <q-icon name="notifications_none" size="3rem" class="advisory-empty-icon" />
      <div class="advisory-empty-title">No advisories</div>
      <div class="advisory-empty-subtitle">
        {{ filter === 'unread' ? 'All caught up.' : 'Federation activity will appear here.' }}
      </div>
    </div>

    <div v-else class="advisory-items">
      <div
        v-for="advisory in filteredAdvisories"
        :key="advisory.id"
        class="advisory-item"
        :class="{ 'advisory-item--read': advisory.read }"
        :style="{ borderLeftColor: advisoryBorderColor(advisory.type) }">
        <div class="advisory-item-header">
          <span class="advisory-item-title">{{ advisory.title }}</span>
          <span class="advisory-item-time">{{ relativeTime(advisory.createdAt) }}</span>
        </div>
        <div class="advisory-item-message">{{ advisory.message }}</div>
        <div class="advisory-item-meta">
          <span v-if="advisory.subjectNodeName" class="advisory-meta-tag">
            {{ advisory.subjectNodeName }}
          </span>
          <span v-if="advisory.subjectNodeBaseUrl" class="advisory-meta-url">
            {{ advisory.subjectNodeBaseUrl }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { Notify } from 'quasar'
import { computed, onMounted, ref } from 'vue'
import {
  FederationAdvisory,
  ListFederationAdvisoriesRequestSchema,
  MarkAdvisoriesReadRequestSchema,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { Timestamp } from '@bufbuild/protobuf/wkt'

const advisories = ref<FederationAdvisory[]>([])
const totalCount = ref(0)
const loading = ref(false)
const filter = ref<'all' | 'unread'>('all')

const unreadCount = computed(() => advisories.value.filter((a) => !a.read).length)

const filteredAdvisories = computed(() => {
  if (filter.value === 'unread') {
    return advisories.value.filter((a) => !a.read)
  }
  return advisories.value
})

function advisoryBorderColor(type: string): string {
  switch (type) {
    case 'NODE_AUTO_PAIRED':
      return 'var(--xy-primary)'
    case 'NODE_REVOKED':
      return 'var(--xy-danger)'
    case 'NODE_DEPARTED':
    case 'AUTO_PAIR_FAILED':
      return 'var(--xy-warning)'
    default:
      return 'var(--xy-border)'
  }
}

function relativeTime(ts: Timestamp | undefined): string {
  if (!ts?.seconds) return ''
  const now = Date.now()
  const then = Number(ts.seconds) * 1000
  const diffMs = now - then
  const diffSec = Math.floor(diffMs / 1000)
  if (diffSec < 60) return 'just now'
  const diffMin = Math.floor(diffSec / 60)
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  const diffDay = Math.floor(diffHr / 24)
  if (diffDay < 30) return `${diffDay}d ago`
  const date = new Date(then)
  return date.toLocaleDateString()
}

async function fetchAdvisories() {
  loading.value = true
  try {
    const resp = await GetXylonaClient().listFederationAdvisories(
      create(ListFederationAdvisoriesRequestSchema, {
        unreadOnly: false,
        limit: 100,
        offset: 0,
      }),
    )
    advisories.value = resp.advisories
    totalCount.value = resp.totalCount
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
    console.error(err.message)
  } finally {
    loading.value = false
  }
}

async function markAllRead() {
  const unreadIds = advisories.value.filter((a) => !a.read).map((a) => a.id)
  if (unreadIds.length === 0) return
  try {
    await GetXylonaClient().markAdvisoriesRead(
      create(MarkAdvisoriesReadRequestSchema, { advisoryIds: unreadIds }),
    )
    for (const a of advisories.value) {
      a.read = true
    }
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
    console.error(err.message)
  }
}

onMounted(() => {
  void fetchAdvisories()
})
</script>

<style scoped>
.advisory-list {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.advisory-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.advisory-tabs {
  display: flex;
  gap: var(--xy-space-xs);
}

.advisory-tab {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs) var(--xy-space-sm);
  border: 1px solid var(--xy-border);
  border-radius: 6px;
  background: transparent;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-body);
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.advisory-tab:hover {
  background: var(--xy-surface-1);
  color: var(--xy-text-primary);
}

.advisory-tab.active {
  background: var(--xy-surface-2);
  color: var(--xy-text-primary);
  border-color: var(--xy-primary);
}

.advisory-tab-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--xy-surface-2);
  color: var(--xy-text-secondary);
  font-size: 0.7rem;
  font-weight: 600;
}

.advisory-tab-count--unread {
  background: var(--xy-primary);
  color: #fff;
}

.advisory-loading {
  display: flex;
  justify-content: center;
  padding: var(--xy-space-lg);
}

.advisory-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--xy-space-lg);
  color: var(--xy-text-secondary);
}

.advisory-empty-icon {
  color: var(--xy-text-muted);
  margin-bottom: var(--xy-space-sm);
}

.advisory-empty-title {
  font-family: var(--xy-font-display);
  font-size: 1rem;
}

.advisory-empty-subtitle {
  font-size: 0.85rem;
  color: var(--xy-text-muted);
  margin-top: var(--xy-space-xs);
}

.advisory-items {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
}

.advisory-item {
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-1);
  border-left: 3px solid var(--xy-border);
  border-radius: 4px;
  transition: opacity 0.15s ease;
}

.advisory-item--read {
  opacity: 0.55;
}

.advisory-item:hover {
  opacity: 1;
}

.advisory-item-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-xs);
}

.advisory-item-title {
  font-family: var(--xy-font-display);
  font-size: 0.9rem;
  color: var(--xy-text-primary);
}

.advisory-item-time {
  font-size: 0.75rem;
  color: var(--xy-text-muted);
  white-space: nowrap;
}

.advisory-item-message {
  font-size: 0.85rem;
  color: var(--xy-text-secondary);
  line-height: 1.4;
  margin-bottom: var(--xy-space-xs);
}

.advisory-item-meta {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  font-size: 0.75rem;
}

.advisory-meta-tag {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
  padding: 1px 6px;
  border-radius: 3px;
}

.advisory-meta-url {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono, monospace);
  font-size: 0.7rem;
}
</style>
