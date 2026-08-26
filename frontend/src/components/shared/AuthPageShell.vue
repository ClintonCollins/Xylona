<template>
  <q-layout view="lHh Lpr fff">
    <q-page-container>
      <q-page class="auth-page">
        <div class="auth-layout">
          <div class="auth-brand">
            <div class="auth-brand-name">Xylona</div>
            <div class="auth-brand-tagline">
              Game Server<br />
              Control Panel
            </div>
          </div>
          <main class="auth-content">
            <div class="auth-panel">
              <slot />
            </div>
          </main>
        </div>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<style scoped>
.auth-page,
.auth-layout {
  background: var(--xy-base);
}

.auth-layout {
  display: flex;
  min-height: 100vh;
  min-height: 100dvh;
  align-items: center;
}

.auth-brand {
  flex: 0 0 44%;
  align-self: stretch;
  padding: var(--xy-space-3xl);
  display: flex;
  flex-direction: column;
  justify-content: center;
  background: var(--xy-surface-1);
  border-right: 1px solid var(--xy-border);
}

.auth-brand-name {
  font-family: var(--xy-font-brand);
  font-size: var(--xy-font-size-2xl);
  color: var(--xy-accent);
  letter-spacing: 0.04em;
  line-height: 1.15;
}

.auth-brand-tagline {
  margin-top: var(--xy-space-md);
  color: var(--xy-text-muted);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-base);
  letter-spacing: 0.08em;
  line-height: 1.5;
  text-transform: uppercase;
}

.auth-content {
  flex: 1;
  min-width: 0;
  padding: var(--xy-space-3xl);
  display: flex;
  align-items: center;
  justify-content: center;
}

.auth-panel {
  box-sizing: border-box;
  width: 100%;
  max-width: 420px;
  padding: var(--xy-space-2xl);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  box-shadow: var(--xy-shadow-lg);
}

.auth-panel :deep(.auth-header) {
  margin-bottom: var(--xy-space-xl);
}

.auth-panel :deep(.auth-title) {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-2xl);
  line-height: 1.2;
}

.auth-panel :deep(.auth-help) {
  margin: var(--xy-space-sm) 0 0;
  color: var(--xy-text-secondary);
  line-height: 1.5;
}

.auth-panel :deep(.auth-error) {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
}

.auth-panel :deep(.auth-button) {
  font-family: var(--xy-font-control);
  font-weight: 600;
  letter-spacing: 0.04em;
}

@media (max-width: 599px) {
  .auth-layout {
    flex-direction: column;
    align-items: stretch;
    justify-content: flex-start;
    gap: var(--xy-space-lg);
    padding: var(--xy-space-md);
  }

  .auth-brand {
    flex: none;
    align-self: auto;
    padding: var(--xy-space-lg) 0 0;
    text-align: center;
    background: transparent;
    border-right: 0;
  }

  .auth-content {
    flex: none;
    width: 100%;
    padding: 0;
  }

  .auth-panel {
    padding: var(--xy-space-lg);
  }
}
</style>
