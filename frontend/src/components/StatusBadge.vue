<template>
 <q-chip class="server-status-badge" :class="color" :label="label"/>
</template>

<script setup lang="ts">
import {Status} from "src/proto/shared_pb";
import {computed, PropType, ref} from "vue";

const props = defineProps({
  status: {
    type: Object as PropType<Status>,
    required: true,
    default: Status.UNKNOWN
  },
})

const color = ref("grey")
const label = computed(() => {
  switch (props.status) {
    case Status.ONLINE:
      color.value = "bg-success"
      return "Online"
    case Status.OFFLINE:
      color.value = "bg-error"
      return "Offline"
    case Status.UPDATING:
      color.value = "bg-alert"
      return "Updating"
    case Status.INSTALLING:
      color.value = "bg-alert"
      return "Installing"
    default:
      color.value = "bg-neutral"
      return "Unknown"
  }
})


</script>

<style scoped>
.server-status-badge {
  font-family: 'Goldman',serif;
}
</style>
