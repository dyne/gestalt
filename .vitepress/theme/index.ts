import { h } from 'vue'
import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import DyneBrand from './components/DyneBrand.vue'
import DyneFooter from './components/DyneFooter.vue'
import InstallCommand from './components/InstallCommand.vue'
import './style.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('InstallCommand', InstallCommand)
  },
  Layout: () => {
    return h(DefaultTheme.Layout, null, {
      'nav-bar-title-before': () => h(DyneBrand, { variant: 'nav' }),
      'layout-bottom': () => h(DyneFooter)
    })
  }
} satisfies Theme
