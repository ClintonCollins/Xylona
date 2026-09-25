<template>
  <section class="form-section" data-testid="readonly-provisioning">
    <div class="section-header">
      <span class="section-icon">
        <q-icon name="hub" size="14px" />
      </span>
      <h3 class="section-title">Provisioning</h3>
      <span class="section-line"></span>
    </div>

    <div class="readonly-provisioning-grid">
      <article class="readonly-provisioning-item" data-testid="readonly-game">
        <span class="readonly-provisioning-label">Game</span>
        <span class="readonly-provisioning-value">{{ game }}</span>
      </article>
      <article class="readonly-provisioning-item" data-testid="readonly-owner">
        <span class="readonly-provisioning-label">Owner</span>
        <span class="readonly-provisioning-value">{{ owner }}</span>
      </article>
      <article class="readonly-provisioning-item" data-testid="readonly-node">
        <span class="readonly-provisioning-label">Node</span>
        <span class="readonly-provisioning-value">{{ node }}</span>
      </article>
      <article class="readonly-provisioning-item" data-testid="readonly-connection">
        <span class="readonly-provisioning-label">Connection</span>
        <span class="readonly-provisioning-value font-mono">{{ connection }}</span>
      </article>
      <article class="readonly-provisioning-item" data-testid="readonly-capacity">
        <span class="readonly-provisioning-label">Capacity</span>
        <span class="readonly-provisioning-value xy-num">{{ capacity }}</span>
      </article>
      <article class="readonly-provisioning-item" data-testid="readonly-server-executable">
        <span class="readonly-provisioning-label">Executable</span>
        <span class="readonly-provisioning-value font-mono">{{ executable }}</span>
      </article>
      <article
        v-if="showMemory"
        class="readonly-provisioning-item"
        data-testid="readonly-max-memory">
        <span class="readonly-provisioning-label">Java heap limit</span>
        <span class="readonly-provisioning-value xy-num">{{ memory }}</span>
      </article>
    </div>
  </section>
</template>

<script lang="ts" setup>
defineProps<{
  capacity: string
  connection: string
  executable: string
  game: string
  memory: string
  node: string
  owner: string
  showMemory: boolean
}>()
</script>

<style scoped>
.readonly-provisioning-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.readonly-provisioning-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  border-radius: var(--xy-radius-lg);
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-raised-subtle);
}

.readonly-provisioning-label {
  font-size: var(--xy-font-size-xs);
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--xy-text-emphasis-soft);
}

.readonly-provisioning-value {
  color: var(--xy-text-primary);
  line-height: 1.45;
  overflow-wrap: anywhere;
}
</style>
