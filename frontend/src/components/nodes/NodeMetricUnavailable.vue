<template>
  <span class="text-xy-muted"
    >Unavailable<span class="xy-visually-hidden">: {{ reason }}</span>
    <q-tooltip>{{ reason }}</q-tooltip>
  </span>
</template>

<script lang="ts" setup>
// Muted and uncoloured: a reading the node couldn't take is neither healthy nor critical.
import { computed } from 'vue'
import { nodeResourceUnavailableReason, type NodeResource } from './node-display'

const props = defineProps<{ resource: NodeResource }>()
const reason = computed(() => nodeResourceUnavailableReason(props.resource))
</script>
