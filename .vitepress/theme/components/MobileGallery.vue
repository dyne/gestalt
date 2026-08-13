<script setup lang="ts">
import { withBase } from 'vitepress'

type GalleryItem = {
  file: string
  title: string
  caption: string
  alt: string
  width: number
  height: number
  stage: string
}

const items: GalleryItem[] = [
  {
    file: '01-first-device-enrollment.png',
    title: 'Authorize the first device',
    caption: 'Claim an empty relay locally with a named passkey, or continue setup from the QR code.',
    alt: 'First-device enrollment screen with device nickname, authorization button, QR code, and setup link.',
    width: 375,
    height: 1077,
    stage: 'Onboarding'
  },
  {
    file: '02-passkey-login.png',
    title: 'Unlock the relay',
    caption: 'Return to a protected relay with a discoverable passkey.',
    alt: 'Passkey login screen for unlocking Gestalt Mobile.',
    width: 375,
    height: 812,
    stage: 'Onboarding'
  },
  {
    file: '03-workspace-selection.png',
    title: 'Choose a workspace',
    caption: 'Navigate the bounded filesystem tree and select the repository Codex should use.',
    alt: 'Sessions tab with an expanded workspace tree and the gestalt-mobile Git repository selected.',
    width: 390,
    height: 844,
    stage: 'Sessions'
  },
  {
    file: '04-session-setup.png',
    title: 'Configure a session',
    caption: 'Choose the skill profile, sandbox, and approval policy before starting.',
    alt: 'Sessions tab showing new-session controls for skill profile, sandbox, and approval policy.',
    width: 390,
    height: 844,
    stage: 'Sessions'
  },
  {
    file: '05-skill-profiles.png',
    title: 'Manage skill profiles',
    caption: 'Build a reusable selection of agent capabilities for focused sessions.',
    alt: 'Skill profile editor with Playwright and verification skills enabled.',
    width: 390,
    height: 844,
    stage: 'Sessions'
  },
  {
    file: '06-configuration.png',
    title: 'Open configuration',
    caption: 'Switch appearance, open the scratchpad, manage devices, or lock the relay.',
    alt: 'Gestalt Mobile configuration popover open over the Sessions tab.',
    width: 390,
    height: 844,
    stage: 'Sessions'
  },
  {
    file: '07-chat.png',
    title: 'Work in Chat',
    caption: 'Follow durable prompts, commentary, final answers, timing, and relay status.',
    alt: 'Chat tab with a user request and a completed Gestalt Mobile response.',
    width: 390,
    height: 844,
    stage: 'Chat'
  },
  {
    file: '08-command-approval.png',
    title: 'Approve a command',
    caption: 'Keep execution decisions in the timeline while Codex is working.',
    alt: 'Chat tab showing a pending command approval with Approve and Deny controls.',
    width: 390,
    height: 844,
    stage: 'Chat'
  },
  {
    file: '09-file-approval.png',
    title: 'Review file changes',
    caption: 'See every requested file target before accepting or denying a write.',
    alt: 'Chat tab showing a file-change approval and three requested paths.',
    width: 390,
    height: 880,
    stage: 'Chat'
  },
  {
    file: '10-user-input.png',
    title: 'Answer a bounded question',
    caption: 'Respond to agent quizzes without leaving the active session.',
    alt: 'Chat tab with a selected supervised multi-agent answer and Send answers button.',
    width: 390,
    height: 844,
    stage: 'Chat'
  },
  {
    file: '11-plans-list.png',
    title: 'Browse plans',
    caption: 'The Plan tab remains useful even before the selected session has an active plan.',
    alt: 'Plan tab showing the plans list with no active plan selected.',
    width: 390,
    height: 844,
    stage: 'Plans'
  },
  {
    file: '12-plan-progress.png',
    title: 'Track supervised progress',
    caption: 'See the current milestone, nested steps, assigned skills, and evidence status.',
    alt: 'Plan tab showing a supervised plan with one of three steps complete and a nested current step.',
    width: 390,
    height: 844,
    stage: 'Plans'
  },
  {
    file: '13-plan-review.png',
    title: 'Await review',
    caption: 'Completed implementation remains visibly unreviewed until its evidence is accepted.',
    alt: 'Plan tab showing completed steps awaiting review.',
    width: 390,
    height: 844,
    stage: 'Plans'
  },
  {
    file: '14-plan-complete.png',
    title: 'Complete the plan',
    caption: 'A reviewed plan closes the loop with every milestone and child step done.',
    alt: 'Plan tab showing a fully completed and reviewed supervised plan.',
    width: 390,
    height: 844,
    stage: 'Plans'
  },
  {
    file: '15-git.png',
    title: 'Inspect Git',
    caption: 'Review branch divergence, working-tree counts, recent commits, and safe actions.',
    alt: 'Git tab showing branch, upstream, ahead count, dirty counts, Push and Pull controls, and recent commits.',
    width: 390,
    height: 844,
    stage: 'Workspace'
  },
  {
    file: '16-scratchpad.png',
    title: 'Keep a scratchpad',
    caption: 'Carry local notes across tabs without adding them to the Codex conversation.',
    alt: 'Gestalt Mobile scratchpad with a note editor and save controls.',
    width: 375,
    height: 964,
    stage: 'Workspace'
  },
  {
    file: '17-authorized-devices.png',
    title: 'Manage authorized devices',
    caption: 'Rename or revoke passkeys and see which device is currently in use.',
    alt: 'Authorized devices screen with the current phone and a laptop, each with rename and revoke controls.',
    width: 375,
    height: 1076,
    stage: 'Security'
  },
  {
    file: '18-add-device.png',
    title: 'Authorize another device',
    caption: 'Create a short-lived enrollment link, share it as a QR code, or create the passkey here.',
    alt: 'Authorized devices screen with an active enrollment link, QR code, and passkey controls.',
    width: 375,
    height: 1585,
    stage: 'Security'
  }
]
</script>

<template>
<div class="mobile-gallery">
  <article v-for="(item, index) in items" :key="item.file" class="mobile-gallery__card">
    <a
      class="mobile-gallery__image-link"
      :href="withBase(`/images/gestalt-mobile/${item.file}`)"
      :aria-label="`Open full-size screenshot: ${item.title}`"
    >
      <img
        :src="withBase(`/images/gestalt-mobile/${item.file}`)"
        :alt="item.alt"
        :width="item.width"
        :height="item.height"
        :loading="index === 0 ? 'eager' : 'lazy'"
        :fetchpriority="index === 0 ? 'high' : undefined"
      />
    </a>
    <div class="mobile-gallery__copy">
      <p class="mobile-gallery__stage">{{ item.stage }}</p>
      <h2>{{ item.title }}</h2>
      <p>{{ item.caption }}</p>
    </div>
  </article>
</div>
</template>
