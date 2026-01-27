import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { installGlobalCrashHandlers } from './lib/appHealthStore.js'

installGlobalCrashHandlers()

const app = mount(App, {
  target: document.getElementById('app'),
})

export default app
