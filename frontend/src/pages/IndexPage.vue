<template>
  <q-page class="index-page flex flex-center">
    <div class="index-content">
      <div class="brand-title">Xylona</div>
      <div class="brand-subtitle">Game Server Control Panel</div>
      <div class="nav-grid">
        <router-link v-for="item in navItems" :key="item.to" :to="item.to" class="nav-card">
          <q-icon :name="item.icon" size="1.5rem" class="nav-card-icon" />
          <span class="nav-card-label">{{ item.label }}</span>
          <q-icon name="chevron_right" size="xs" class="nav-card-arrow" />
        </router-link>
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
const navItems = [
  { to: '/game-servers', icon: 'dns', label: 'Game Servers' },
  { to: '/games', icon: 'sports_esports', label: 'Games' },
  { to: '/nodes', icon: 'device_hub', label: 'Nodes' },
  { to: '/dashboard', icon: 'dashboard', label: 'Dashboard' },
]
</script>

<style scoped>
.index-content {
  text-align: center;
}

.brand-title {
  font-family: var(--xy-font-brand);
  font-size: clamp(3rem, 8vw, 5rem);
  color: var(--xy-accent);
  letter-spacing: 0.06em;
  line-height: 1;
  margin-bottom: var(--xy-space-sm);
}

.brand-subtitle {
  font-family: var(--xy-font-display);
  font-size: 0.85rem;
  color: var(--xy-text-muted);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.nav-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--xy-space-sm);
  margin-top: var(--xy-space-2xl);
  max-width: 400px;
  margin-left: auto;
  margin-right: auto;
}

.nav-card {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background-color: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  color: var(--xy-text-primary);
  text-decoration: none;
  font-family: var(--xy-font-display);
  font-size: 0.9rem;
  font-weight: 500;
  transition:
    border-color 0.2s ease,
    background-color 0.2s ease,
    box-shadow 0.2s ease;
}

.nav-card:hover,
.nav-card:focus-visible {
  border-color: var(--xy-primary);
  background-color: var(--xy-surface-2);
  box-shadow: var(--xy-shadow-md);
  outline: none;
}

.nav-card-icon {
  color: var(--xy-accent);
  flex-shrink: 0;
}

.nav-card-label {
  flex: 1;
  text-align: left;
}

.nav-card-arrow {
  color: var(--xy-text-muted);
  opacity: 0;
  transform: translateX(-4px);
  transition:
    opacity 0.2s ease,
    transform 0.2s ease;
}

.nav-card:hover .nav-card-arrow,
.nav-card:focus-visible .nav-card-arrow {
  opacity: 1;
  transform: translateX(0);
}

@media (max-width: 480px) {
  .nav-grid {
    grid-template-columns: 1fr;
    max-width: 280px;
  }
}
</style>
