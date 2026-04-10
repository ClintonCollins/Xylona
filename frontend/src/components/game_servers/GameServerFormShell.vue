<template>
  <div
    :class="{ 'server-form-shell--compact': compactHeader }"
    :data-testid="testId"
    class="server-form-shell full-width">
    <div ref="stickySentinel" class="sticky-sentinel"></div>

    <div :class="{ 'is-stuck': isStuck, 'is-compact': compactHeader }" class="server-form-header">
      <div class="server-form-header-left">
        <div v-if="!compactHeader && breadcrumbLabel" class="server-form-breadcrumbs">
          <router-link class="breadcrumb-link" to="/game-servers">Game Servers</router-link>
          <span class="breadcrumb-sep">/</span>
          <span class="breadcrumb-current">{{ breadcrumbLabel }}</span>
        </div>
        <div class="server-form-title font-display">{{ headerTitle }}</div>
        <div v-if="!compactHeader && subtitle" class="server-form-subtitle text-xy-secondary">
          {{ subtitle }}
        </div>
      </div>

      <div class="server-form-header-actions">
        <q-btn :disable="formSubmitting" flat label="Cancel" @click="$emit('cancel')" />
        <q-btn
          :class="{ 'server-form-save-btn--ready': saveReady }"
          :disable="saveDisabled"
          :label="saveLabel"
          :loading="formSubmitting"
          class="server-form-save-btn"
          color="primary"
          @click="$emit('save')" />
      </div>
    </div>

    <div v-if="loading" class="server-form-loading">
      <q-spinner-dots color="primary" size="40px" />
      <div class="text-xy-secondary q-mt-sm">{{ loadingText }}</div>
    </div>

    <div v-else class="server-form-body">
      <div v-if="guidance" class="server-form-guidance text-xy-muted">
        {{ guidance }}
      </div>
      <slot />
    </div>

    <q-inner-loading
      :label="submittingLabel"
      :showing="formSubmitting"
      label-class="text-primary" />
  </div>
</template>

<script lang="ts" setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'

withDefaults(
  defineProps<{
    breadcrumbLabel?: string
    compactHeader?: boolean
    formSubmitting: boolean
    guidance?: string
    headerTitle: string
    loading: boolean
    loadingText?: string
    saveDisabled?: boolean
    saveLabel?: string
    saveReady?: boolean
    submittingLabel?: string
    subtitle?: string
    testId?: string
  }>(),
  {
    breadcrumbLabel: '',
    compactHeader: false,
    guidance: '',
    loadingText: 'Loading server options...',
    saveDisabled: false,
    saveLabel: 'Save',
    saveReady: false,
    submittingLabel: 'Saving game server...',
    subtitle: '',
    testId: 'game-server-form-shell',
  },
)

defineEmits<{
  cancel: []
  save: []
}>()

const stickySentinel = ref<HTMLElement | null>(null)
const isStuck = ref(false)

let stickyObserver: IntersectionObserver | undefined

onMounted(() => {
  if (!stickySentinel.value) {
    return
  }

  stickyObserver = new IntersectionObserver(
    ([entry]) => {
      isStuck.value = !entry.isIntersecting
    },
    { threshold: 0 },
  )
  stickyObserver.observe(stickySentinel.value)
})

onBeforeUnmount(() => {
  stickyObserver?.disconnect()
})
</script>

<style>
.server-form-shell {
  width: 100%;
  --server-form-ease-smooth: cubic-bezier(0.25, 1, 0.5, 1);
}

.sticky-sentinel {
  height: 0;
  overflow: hidden;
}

.server-form-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-lg);
  padding: var(--xy-space-lg) var(--xy-space-lg) var(--xy-space-md);
  background:
    linear-gradient(180deg, var(--xy-accent-glow-soft), transparent 55%), var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-bottom: 1px solid var(--xy-border);
  border-radius: 10px 10px 0 0;
  position: sticky;
  top: 50px;
  z-index: 10;
  transition:
    border-color var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast),
    background var(--xy-transition-fast),
    transform 220ms var(--server-form-ease-smooth);
}

.server-form-header.is-stuck {
  border-bottom-color: var(--xy-accent-border-soft);
  box-shadow: var(--xy-shadow-sticky-lg);
  transform: translateY(-2px);
}

.server-form-header.is-compact {
  align-items: center;
  gap: var(--xy-space-md);
  padding-top: var(--xy-space-md);
  padding-bottom: var(--xy-space-sm);
}

.server-form-header.is-compact .server-form-header-left {
  gap: 2px;
}

.server-form-header.is-compact .server-form-title {
  font-size: clamp(1.08rem, 0.98rem + 0.46vw, 1.34rem);
  line-height: 1.1;
}

.server-form-header.is-compact .server-form-header-actions {
  padding-top: 0;
}

.server-form-header-left {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  min-width: 0;
}

.server-form-breadcrumbs {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.75rem;
}

.breadcrumb-link {
  color: var(--xy-text-muted);
  text-decoration: none;
}

.breadcrumb-sep {
  color: var(--xy-text-muted);
  opacity: 0.5;
}

.breadcrumb-current {
  color: var(--xy-text-secondary);
}

.server-form-title {
  font-size: clamp(1.28rem, 1.06rem + 0.8vw, 1.68rem);
  font-weight: 600;
  color: var(--xy-text-primary);
  letter-spacing: 0.015em;
  line-height: 1.15;
}

.server-form-subtitle {
  max-width: 64ch;
  font-size: 0.9rem;
  line-height: 1.6;
  color: var(--xy-text-secondary);
}

.server-form-header-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  flex-shrink: 0;
  padding-top: 2px;
}

.server-form-save-btn--ready {
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--xy-success) 22%, transparent);
}

.server-form-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 260px;
  padding: var(--xy-space-xl);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-top: none;
  border-radius: 0 0 10px 10px;
}

.server-form-body {
  padding: var(--xy-space-md) var(--xy-space-lg) var(--xy-space-lg);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-top: none;
  border-radius: 0 0 10px 10px;
}

.server-form-shell--compact .server-form-body {
  padding-top: clamp(1.5rem, 1.15rem + 0.9vw, 2rem);
}

.server-form-shell--compact .form-section:first-child {
  padding-top: var(--xy-space-md);
}

.server-form-guidance {
  margin-bottom: var(--xy-space-sm);
  font-size: 0.82rem;
  line-height: 1.45;
}

.server-form-layout {
  display: flex;
  flex-direction: column;
}

.form-section {
  padding: var(--xy-space-lg) 0;
  border-bottom: 1px solid var(--xy-border);
}

.form-section:first-child {
  padding-top: var(--xy-space-sm);
}

.form-section--last {
  border-bottom: none;
  padding-bottom: 0;
}

.form-section--summary {
  padding-top: clamp(1.5rem, 1.1rem + 1vw, 2rem);
  border-top: 1px solid var(--xy-border);
  border-bottom: none;
  margin-top: var(--xy-space-md);
  padding-bottom: 0;
}

.section-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-sm);
}

.section-title {
  font-size: 0.88rem;
  font-weight: 600;
  color: var(--xy-text-emphasis-soft);
  letter-spacing: 0.04em;
  text-transform: uppercase;
  white-space: nowrap;
  line-height: 1.1;
}

.section-line {
  flex: 1;
  height: 1px;
  background: var(--xy-border);
  margin-left: var(--xy-space-xs);
  opacity: 0.8;
}

.section-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 7px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
}

.section-icon--accent {
  color: var(--xy-accent);
}

.section-icon--primary {
  color: var(--xy-primary);
}

.section-icon--success {
  color: var(--xy-success);
}

.section-icon--warning {
  color: var(--xy-warning);
}

.section-icon--muted {
  color: var(--xy-text-muted);
}

.deployment-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px;
}

.deployment-review {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.deployment-review-heading {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-sm);
}

.deployment-review-copy {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.deployment-review-title {
  font-size: 0.88rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--xy-text-emphasis-strong);
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.deployment-review-subtitle {
  font-size: 0.82rem;
  line-height: 1.45;
}

.deployment-ready {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--xy-success-border-soft);
  border-radius: 10px;
  background:
    linear-gradient(180deg, var(--xy-success-bg-soft), transparent 65%),
    var(--xy-surface-raised-soft);
}

.deployment-ready-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  border: 1px solid var(--xy-success-border-softer);
  color: var(--xy-success);
  background: var(--xy-success-bg-soft);
  flex-shrink: 0;
}

.deployment-ready-content {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.deployment-ready-label {
  font-size: 0.76rem;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--xy-success-text-soft);
}

.deployment-ready-value {
  color: var(--xy-text-primary);
  font-size: 0.94rem;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.deployment-review-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  flex-shrink: 0;
  background: var(--xy-warning);
}

.deployment-summary-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  background: var(--xy-surface-raised-subtle);
  border: 1px solid var(--xy-border);
  border-radius: 9px;
}

.deployment-summary-item--warning {
  border-color: var(--xy-warning-border-soft);
}

.deployment-summary-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: var(--xy-surface-overlay-soft);
  border: 1px solid var(--xy-border);
  color: var(--xy-text-secondary);
  flex-shrink: 0;
}

.deployment-summary-icon.is-warning {
  color: var(--xy-warning);
  border-color: var(--xy-warning-border-soft);
}

.deployment-summary-content {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.deployment-summary-label {
  font-size: 0.76rem;
  line-height: 1.2;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--xy-text-emphasis-soft);
}

.deployment-summary-value {
  color: var(--xy-text-primary);
  font-size: 0.9rem;
  line-height: 1.4;
  overflow-wrap: anywhere;
}

.deployment-state-enter-active,
.deployment-state-leave-active {
  transition:
    opacity 180ms ease,
    transform 180ms ease;
}

.deployment-state-enter-from,
.deployment-state-leave-to {
  opacity: 0;
  transform: translateY(6px);
}
</style>
