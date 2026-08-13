<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { withBase } from 'vitepress'

type Slide = {
  file: string
  title: string
  caption: string
  alt: string
  width: number
  height: number
}

const autoplayDelay = 5000
const swipeThreshold = 48
const slides: Slide[] = [
  {
    file: '03-workspace-selection.png',
    title: 'Choose a workspace',
    caption: 'Select the repository and session boundary from the mobile-first Sessions tab.',
    alt: 'Sessions tab with the gestalt-mobile repository selected in the workspace tree.',
    width: 390,
    height: 844
  },
  {
    file: '07-chat.png',
    title: 'Keep the conversation moving',
    caption: 'Follow durable prompts, commentary, final answers, timing, and relay status.',
    alt: 'Chat tab with a user request and completed Gestalt Mobile response.',
    width: 390,
    height: 844
  },
  {
    file: '09-file-approval.png',
    title: 'Review changes in context',
    caption: 'See every requested file before approving or denying a write.',
    alt: 'Chat tab showing a file-change approval and three requested paths.',
    width: 390,
    height: 880
  },
  {
    file: '12-plan-progress.png',
    title: 'Track supervised progress',
    caption: 'Keep milestones, nested steps, skills, evidence, and review status visible.',
    alt: 'Plan tab showing a supervised plan with one of three steps complete.',
    width: 390,
    height: 844
  },
  {
    file: '15-git.png',
    title: 'Inspect Git safely',
    caption: 'Review branch divergence, working-tree counts, commits, and bounded actions.',
    alt: 'Git tab showing branch divergence, dirty counts, recent commits, Push, and Pull.',
    width: 390,
    height: 844
  },
  {
    file: '18-add-device.png',
    title: 'Take Gestalt to another device',
    caption: 'Create a short-lived enrollment link or QR code from an authorized device.',
    alt: 'Authorized Devices screen with an active enrollment link and QR code.',
    width: 375,
    height: 1585
  }
]

const renderedSlides = [slides.at(-1)!, ...slides, slides[0]!]
const position = ref(1)
const transitionEnabled = ref(false)
const animating = ref(false)
const manualPaused = ref(false)
const temporarilyPaused = ref(false)
const announcement = ref('')
const dragOffset = ref(0)
let dragStart: number | null = null
let dragged = false
let timer: ReturnType<typeof setTimeout> | undefined
let reducedMotion: MediaQueryList | undefined

const activeIndex = computed(() => (position.value - 1 + slides.length) % slides.length)
const activeSlide = computed(() => slides[activeIndex.value]!)
const playing = computed(() => !manualPaused.value && !temporarilyPaused.value)
const trackStyle = computed(() => ({
  transform: `translate3d(calc(${-position.value * 100}% + ${dragOffset.value}px), 0, 0)`,
  transition: transitionEnabled.value && dragStart === null ? undefined : 'none'
}))

function clearTimer(): void {
  if (timer !== undefined) window.clearTimeout(timer)
  timer = undefined
}

function schedule(): void {
  clearTimer()
  if (!playing.value || document.hidden) return
  timer = window.setTimeout(() => {
    move(1, false)
    schedule()
  }, autoplayDelay)
}

function move(direction: -1 | 1, announce = true): void {
  if (animating.value) return
  transitionEnabled.value = true
  animating.value = true
  position.value += direction
  if (announce) {
    announcement.value = `${slides[(position.value - 1 + slides.length) % slides.length]!.title}, slide ${activeIndex.value + 1} of ${slides.length}`
  }
  schedule()
}

function goTo(index: number): void {
  if (animating.value || index === activeIndex.value) return
  transitionEnabled.value = true
  animating.value = true
  position.value = index + 1
  announcement.value = `${slides[index]!.title}, slide ${index + 1} of ${slides.length}`
  schedule()
}

function finishTransition(): void {
  animating.value = false
  if (position.value !== 0 && position.value !== slides.length + 1) return
  transitionEnabled.value = false
  position.value = position.value === 0 ? slides.length : 1
  void nextTick(() => requestAnimationFrame(() => (transitionEnabled.value = true)))
}

function toggleAutoplay(): void {
  manualPaused.value = !manualPaused.value
  announcement.value = manualPaused.value ? 'Automatic slideshow paused.' : 'Automatic slideshow started.'
}

function onPointerDown(event: PointerEvent): void {
  if (event.button !== 0) return
  dragStart = event.clientX
  dragged = false
  dragOffset.value = 0
  temporarilyPaused.value = true
  event.currentTarget instanceof HTMLElement && event.currentTarget.setPointerCapture(event.pointerId)
}

function onPointerMove(event: PointerEvent): void {
  if (dragStart === null) return
  dragOffset.value = event.clientX - dragStart
  if (Math.abs(dragOffset.value) > 8) dragged = true
}

function finishPointer(event: PointerEvent): void {
  if (dragStart === null) return
  const offset = event.clientX - dragStart
  dragStart = null
  dragOffset.value = 0
  temporarilyPaused.value = false
  if (Math.abs(offset) >= swipeThreshold) move(offset < 0 ? 1 : -1)
}

function preventDraggedClick(event: MouseEvent): void {
  if (!dragged) return
  event.preventDefault()
  event.stopPropagation()
  dragged = false
}

function onVisibilityChange(): void {
  schedule()
}

function onMotionPreference(event: MediaQueryListEvent): void {
  if (event.matches) manualPaused.value = true
}

watch(playing, schedule)

onMounted(() => {
  transitionEnabled.value = true
  reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)')
  manualPaused.value = reducedMotion.matches
  reducedMotion.addEventListener('change', onMotionPreference)
  document.addEventListener('visibilitychange', onVisibilityChange)
  schedule()
})

onBeforeUnmount(() => {
  clearTimer()
  reducedMotion?.removeEventListener('change', onMotionPreference)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <section
    class="home-carousel"
    aria-roledescription="carousel"
    aria-label="Gestalt Mobile journey"
    @pointerenter="temporarilyPaused = true"
    @pointerleave="temporarilyPaused = false"
    @focusin="temporarilyPaused = true"
    @focusout="temporarilyPaused = false"
  >
    <div
      class="home-carousel__viewport"
      @click.capture="preventDraggedClick"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="finishPointer"
      @pointercancel="finishPointer"
    >
      <ul
        class="home-carousel__track"
        :class="{ 'home-carousel__track--animated': transitionEnabled }"
        :style="trackStyle"
        @transitionend="finishTransition"
      >
        <li
          v-for="(slide, renderedIndex) in renderedSlides"
          :key="`${slide.file}-${renderedIndex}`"
          class="home-carousel__slide"
          :aria-hidden="renderedIndex !== activeIndex + 1"
        >
          <a
            class="home-carousel__image-link"
            :href="withBase(`/images/gestalt-mobile/${slide.file}`)"
            :tabindex="renderedIndex === activeIndex + 1 ? 0 : -1"
            :aria-label="`Open full-size screenshot: ${slide.title}`"
          >
            <img
              :src="withBase(`/images/gestalt-mobile/${slide.file}`)"
              :alt="renderedIndex === activeIndex + 1 ? slide.alt : ''"
              :width="slide.width"
              :height="slide.height"
              :loading="renderedIndex === 1 ? 'eager' : 'lazy'"
              :fetchpriority="renderedIndex === 1 ? 'high' : 'low'"
              draggable="false"
            />
          </a>
          <div class="home-carousel__copy">
            <p class="home-carousel__eyebrow">Gestalt Mobile</p>
            <h3>{{ slide.title }}</h3>
            <p>{{ slide.caption }}</p>
          </div>
        </li>
      </ul>
    </div>

    <div class="home-carousel__controls">
      <button type="button" aria-label="Previous screenshot" @click="move(-1)">←</button>
      <div class="home-carousel__dots" aria-label="Choose screenshot">
        <button
          v-for="(slide, index) in slides"
          :key="slide.file"
          type="button"
          :class="{ 'is-active': index === activeIndex }"
          :aria-label="`Show slide ${index + 1}: ${slide.title}`"
          :aria-current="index === activeIndex ? 'true' : undefined"
          @click="goTo(index)"
        ><span aria-hidden="true"></span></button>
      </div>
      <button type="button" aria-label="Next screenshot" @click="move(1)">→</button>
      <button
        class="home-carousel__autoplay"
        type="button"
        :aria-pressed="manualPaused"
        :aria-label="manualPaused ? 'Start automatic slideshow' : 'Pause automatic slideshow'"
        @click="toggleAutoplay"
      >{{ manualPaused ? 'Play' : 'Pause' }}</button>
    </div>

    <p class="visually-hidden" :aria-live="playing ? 'off' : 'polite'" aria-atomic="true">
      {{ announcement }}
    </p>
    <p class="home-carousel__position">{{ activeIndex + 1 }} / {{ slides.length }} · {{ activeSlide.title }}</p>
  </section>
</template>
