<template>
  <div @click="copyValue(props.clipBoardValue)" class="copy-clipboard">{{ props.displayText}}
    <q-tooltip id="clipBoardCopyTooltip" @before-show="resetClipboardCopy" :anchor="props.tooltipAnchor" :self="props.tooltipSelf" :offset="[10, 10]" v-html="clipboardInnerHTML"></q-tooltip>
  </div>
</template>

<script setup lang="ts">
import {copyToClipboard, QTooltip} from "quasar";
import {ref} from "vue";

const props = defineProps({
  clipBoardValue: {
    type: String,
    required: true,
    default: ""
  },
  displayText: {
    type: String,
    default: ""
  },
  tooltipAnchor: {
    type: String,
    default: "center right"
  },
  tooltipSelf: {
    type: String,
    default: "center left"
  },
  clipBoardInnerHTML: {
    type: String,
    default: "Copy to clipboard"
  },
  clipBoardSuccessInnerHTML: {
    type: String,
    default: "Copied!"
  }
})

const clipboardInnerHTML = ref(props.clipBoardInnerHTML)

async function resetClipboardCopy() {
  clipboardInnerHTML.value = props.clipBoardInnerHTML
}

async function copyValue(value: string) {
  copyToClipboard(value).then(() => {
    clipboardInnerHTML.value = props.clipBoardSuccessInnerHTML
  }).catch((e) => {
    clipboardInnerHTML.value = "<span class='text-red-1'>Error trying to copy</span>"
    console.error(e)
  })
}

</script>

<style>
#clipBoardCopyTooltip {
  font-family: 'Zen Dots', sans-serif;
  font-weight: 400;
  font-size: 0.65rem;
}
</style>
