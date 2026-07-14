<template>
  <main aria-labelledby="not-found-title" class="fullscreen flex flex-center error-page">
    <section class="error-content">
      <div aria-hidden="true" class="error-code">404</div>
      <h1 id="not-found-title" class="error-title">Page not found</h1>
      <p class="error-message">
        The address may be outdated, or you may not have access to this part of the panel.
      </p>
      <div class="error-actions">
        <q-btn
          color="primary"
          icon="dns"
          label="Game Servers"
          no-caps
          to="/game-servers"
          unelevated />
        <q-btn flat icon="arrow_back" label="Go back" no-caps @click="goBack" />
      </div>
    </section>
  </main>
</template>

<script lang="ts" setup>
import { useRouter } from 'vue-router'

const router = useRouter()

function goBack() {
  if (typeof window !== 'undefined' && window.history.length > 1) {
    router.back()
    return
  }

  void router.push('/game-servers')
}
</script>

<style scoped>
.error-page {
  background: var(--xy-base);
  text-align: center;
  padding: var(--xy-space-xl);
}

.error-content {
  width: min(100%, 36rem);
}

.error-code {
  font-family: var(--xy-font-brand);
  font-size: clamp(5rem, 20vw, 14rem);
  color: var(--xy-accent);
  line-height: 0.9;
  letter-spacing: 0.04em;
}

.error-title {
  margin: var(--xy-space-md) 0 0;
  font-family: var(--xy-font-display);
  font-size: clamp(1.4rem, 3vw, 2rem);
  color: var(--xy-text-primary);
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.error-message {
  margin: var(--xy-space-sm) auto 0;
  max-width: 32rem;
  color: var(--xy-text-secondary);
  font-size: 1rem;
  line-height: 1.6;
}

.error-actions {
  display: flex;
  justify-content: center;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
  margin-top: var(--xy-space-xl);
}
</style>
