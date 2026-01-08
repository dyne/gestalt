const injectedVersion =
  typeof __GESTALT_VERSION__ === 'undefined' ? 'dev' : __GESTALT_VERSION__

export const VERSION = injectedVersion
