<template>
  <router-view/>
</template>

<script setup lang="ts">
import {onMounted, ref} from "vue";

const backgroundImages = ['background1.webp', 'background2.webp', 'background3.webp', 'background4.webp', 'background5.webp']
const selectedBackgroundIndex = ref(2)
const backgroundURL = ref(setBackgroundImageURL(backgroundImages[selectedBackgroundIndex.value]))
const backgroundOpacity = ref(1)
const backgroundFadeDuration = ref('opacity 2s')
const secondsBetweenBackgroundChanges = 300

onMounted(async () => {
  console.log("Mounted")
  setInterval(changeBackgroundImage, secondsBetweenBackgroundChanges * 1000)
})

function setBackgroundImageURL(image: string): string {
  return `url('/src/assets/backgrounds/${image}')`
}

function changeBackgroundImage() {
  selectedBackgroundIndex.value++
  if (selectedBackgroundIndex.value >= backgroundImages.length) {
    selectedBackgroundIndex.value = 0
  }
  backgroundOpacity.value = 0
  setTimeout(() => {
    backgroundURL.value = setBackgroundImageURL(backgroundImages[selectedBackgroundIndex.value])
    setTimeout(() => {
      backgroundOpacity.value = 1
    }, 50)
  }, 2000)
}
</script>

<style>
.q-layout:before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  right: 0;
  background-image: v-bind(backgroundURL);
  background-size: cover;
  background-repeat: no-repeat;
  opacity: v-bind(backgroundOpacity);
  transition: v-bind(backgroundFadeDuration);
}
</style>
