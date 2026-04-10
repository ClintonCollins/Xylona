<template>
  <div class="downstream-impact">
    <div v-if="showHeader" class="downstream-impact__header">
      <div class="font-display downstream-impact__title">Downstream impact</div>
      <div class="text-xy-secondary downstream-impact__copy">
        {{ servers.length }} servers currently use this template.
      </div>
    </div>

    <div v-if="servers.length === 0" class="downstream-impact__empty text-xy-muted">
      No game servers are currently using this game definition.
    </div>

    <div v-else class="downstream-impact__shell">
      <div class="downstream-impact__summary-row">
        <div class="downstream-impact__summary">
          <span
            class="downstream-impact__summary-pill downstream-impact__summary-pill--customized text-xy-secondary">
            {{ customizedServerCount }} customized
          </span>
          <span
            class="downstream-impact__summary-pill downstream-impact__summary-pill--default text-xy-secondary">
            {{ defaultServerCount }} all defaults
          </span>
        </div>

        <button class="downstream-impact__toggle" type="button" @click="expanded = !expanded">
          {{ expanded ? 'Hide server list' : `Review ${servers.length} servers` }}
        </button>
      </div>

      <div
        v-if="expanded"
        aria-label="Affected servers"
        class="downstream-impact__list"
        role="list">
        <article v-for="server in visibleServers" :key="server.name" class="downstream-impact__row">
          <div :title="server.name" class="downstream-impact__name">{{ server.name }}</div>
          <div
            :class="
              server.patchCount === 0
                ? 'downstream-impact__detail--default'
                : 'downstream-impact__detail--customized'
            "
            class="downstream-impact__detail text-xy-secondary">
            {{ server.patchCount === 0 ? 'all defaults' : `${server.patchCount} customized` }}
          </div>
        </article>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue'

type DownstreamServer = {
  name: string
  patchCount: number
}

const props = withDefaults(
  defineProps<{
    servers: DownstreamServer[]
    showHeader?: boolean
  }>(),
  {
    showHeader: true,
  },
)

const expanded = ref(false)
const visibleServers = computed(() => props.servers)
const customizedServerCount = computed(
  () => props.servers.filter((server) => server.patchCount > 0).length,
)
const defaultServerCount = computed(() => props.servers.length - customizedServerCount.value)
</script>

<style scoped>
.downstream-impact {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.downstream-impact__shell {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
}

.downstream-impact__summary-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.6rem;
  flex-wrap: wrap;
}

.downstream-impact__header {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.downstream-impact__title {
  font-size: 0.95rem;
  color: var(--xy-text-primary);
}

.downstream-impact__copy {
  font-size: 0.82rem;
}

.downstream-impact__empty {
  padding: 0.9rem;
  border-radius: 12px;
  border: 1px dashed var(--xy-border);
  background: color-mix(in srgb, var(--xy-surface-0) 84%, transparent);
}

.downstream-impact__summary {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
}

.downstream-impact__summary-pill,
.downstream-impact__toggle {
  display: inline-flex;
  align-items: center;
  min-height: 1.7rem;
  padding: 0.12rem 0.55rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 78%, transparent);
  background: color-mix(in srgb, var(--xy-surface-1) 72%, transparent);
  font-size: 0.72rem;
  white-space: nowrap;
}

.downstream-impact__summary-pill--customized {
  border-color: color-mix(in srgb, var(--xy-accent) 28%, var(--xy-border) 72%);
  background: color-mix(in srgb, var(--xy-accent) 8%, var(--xy-surface-1) 92%);
  color: color-mix(in srgb, var(--xy-accent) 30%, var(--xy-text-primary) 70%);
}

.downstream-impact__summary-pill--default {
  border-color: color-mix(in srgb, var(--xy-success) 18%, var(--xy-border) 82%);
  background: color-mix(in srgb, var(--xy-success) 7%, var(--xy-surface-1) 93%);
  color: color-mix(in srgb, var(--xy-success) 26%, var(--xy-text-primary) 74%);
}

.downstream-impact__list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.downstream-impact__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
  min-height: 34px;
  padding: 0.38rem 0.6rem;
  border-radius: 9px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 78%, transparent);
  background: color-mix(in srgb, var(--xy-surface-0) 62%, transparent);
}

.downstream-impact__name {
  color: var(--xy-text-primary);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.8rem;
}

.downstream-impact__detail {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  min-height: 1.4rem;
  padding: 0.06rem 0.42rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 78%, transparent);
  background: color-mix(in srgb, var(--xy-surface-1) 72%, transparent);
  font-size: 0.68rem;
  white-space: nowrap;
}

.downstream-impact__detail--customized {
  border-color: color-mix(in srgb, var(--xy-accent) 24%, var(--xy-border) 76%);
  background: color-mix(in srgb, var(--xy-accent) 8%, var(--xy-surface-1) 92%);
  color: color-mix(in srgb, var(--xy-accent) 28%, var(--xy-text-primary) 72%);
}

.downstream-impact__detail--default {
  border-color: color-mix(in srgb, var(--xy-success) 16%, var(--xy-border) 84%);
  background: color-mix(in srgb, var(--xy-success) 7%, var(--xy-surface-1) 93%);
  color: color-mix(in srgb, var(--xy-success) 24%, var(--xy-text-primary) 76%);
}

.downstream-impact__toggle {
  appearance: none;
  justify-content: center;
  border-color: color-mix(in srgb, var(--xy-accent) 26%, var(--xy-border) 74%);
  background: color-mix(in srgb, var(--xy-accent) 7%, var(--xy-surface-1) 93%);
  color: color-mix(in srgb, var(--xy-accent) 24%, var(--xy-text-primary) 76%);
  cursor: pointer;
  transition:
    border-color var(--xy-transition-fast),
    background var(--xy-transition-fast),
    color var(--xy-transition-fast);
}

.downstream-impact__toggle:hover {
  border-color: color-mix(in srgb, var(--xy-accent) 42%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-accent) 10%, var(--xy-surface-0) 90%);
}

@media (max-width: 720px) {
  .downstream-impact__summary {
    gap: 0.35rem;
  }

  .downstream-impact__summary-row {
    align-items: flex-start;
  }

  .downstream-impact__row {
    gap: 0.45rem;
    padding: 0.42rem 0.55rem;
  }

  .downstream-impact__detail {
    font-size: 0.68rem;
  }
}
</style>
