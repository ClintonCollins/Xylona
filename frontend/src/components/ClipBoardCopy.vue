<template>
  <span class="copy-clipboard-wrap">
    <!-- The name keeps the visible value, so screen readers hear what gets copied. -->
    <button
      :aria-label="`Copy ${props.displayText}`"
      class="copy-clipboard"
      type="button"
      @click="copyValue(props.clipBoardValue)">
      {{ props.displayText }}
      <!-- eslint-disable vue/no-v-text-v-html-on-component, vue/no-v-html -- application-controlled tooltip HTML -->
      <q-tooltip
        :anchor="props.tooltipAnchor"
        :offset="[10, 10]"
        :self="props.tooltipSelf"
        class="clipboard-tooltip"
        @before-show="resetClipboardCopy"
        v-html="clipboardInnerHTML"></q-tooltip>
      <!-- eslint-enable vue/no-v-text-v-html-on-component, vue/no-v-html -->
    </button>
    <!-- Outside the button: a button's children are presentational, so a region inside it may go unannounced. -->
    <span class="xy-visually-hidden" role="status">{{ announcement }}</span>
  </span>
</template>

<script lang="ts" setup>
import { copyToClipboard, QTooltip } from 'quasar'
import { nextTick, ref } from 'vue'

const props = defineProps({
  clipBoardValue: {
    type: String,
    default: '',
  },
  displayText: {
    type: String,
    default: '',
  },
  tooltipAnchor: {
    type: String,
    default: 'center right',
  },
  tooltipSelf: {
    type: String,
    default: 'center left',
  },
  clipBoardInnerHTML: {
    type: String,
    default: 'Copy to clipboard',
  },
  clipBoardSuccessInnerHTML: {
    type: String,
    default: 'Copied!',
  },
})

const clipboardInnerHTML = ref(props.clipBoardInnerHTML)
const announcement = ref('')

async function resetClipboardCopy() {
  clipboardInnerHTML.value = props.clipBoardInnerHTML
}

async function copyValue(value: string) {
  // Clear and flush first: a second copy of the same value must change the region to be announced.
  announcement.value = ''
  await nextTick()
  copyToClipboard(value)
    .then(() => {
      clipboardInnerHTML.value = props.clipBoardSuccessInnerHTML
      announcement.value = `Copied ${props.displayText}`
    })
    .catch((e) => {
      clipboardInnerHTML.value =
        "<span style='color: var(--xy-danger-text)'>Error trying to copy</span>"
      announcement.value = 'Copy failed'
      console.error(e)
    })
}
</script>

<style>
.clipboard-tooltip {
  font-family: var(--xy-font-body);
  font-weight: 400;
  font-size: var(--xy-font-size-2xs);
}
</style>
