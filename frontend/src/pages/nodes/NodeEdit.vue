<template>
  <q-page class="xy-page-content">
    <node-form ref="formRef" :existing-node-id="nodeID"></node-form>
  </q-page>
</template>

<script lang="ts" setup>
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import NodeForm from '@/components/nodes/NodeForm.vue'
import { useUnsavedChangesGuard } from '@/utils/unsaved-changes-guard'

const route = useRoute()
const nodeID = ref(route.params.id)
const formRef = ref<InstanceType<typeof NodeForm> | null>(null)

useUnsavedChangesGuard(() => formRef.value?.isDirty ?? false)
</script>
