<template>
  <div class="fullscreen flex flex-center error-page">
    <div>
      <div class="error-code">404</div>
      <div class="error-message">Nothing here</div>
      <q-btn class="q-mt-xl" color="primary" unelevated to="/" label="Go Home" no-caps />
    </div>
  </div>
</template>

<script setup lang="ts"></script>

<style scoped>
.error-page {
  background: var(--xy-base);
  text-align: center;
}

.error-code {
  font-family: var(--xy-font-brand);
  font-size: clamp(5rem, 20vw, 14rem);
  color: var(--xy-accent);
  line-height: 0.9;
  letter-spacing: 0.04em;
}

.error-message {
  font-family: var(--xy-font-display);
  font-size: clamp(1rem, 3vw, 1.5rem);
  color: var(--xy-text-muted);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  margin-top: var(--xy-space-sm);
}
</style>
