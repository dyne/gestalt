import notificationStore from './notificationStore.js'

export const getErrorMessage = (error, fallback = 'Request failed.') => {
  if (error && typeof error.message === 'string' && error.message.trim()) {
    return error.message
  }
  if (typeof fallback === 'string' && fallback.trim()) {
    return fallback
  }
  return 'Request failed.'
}

export const notifyError = (error, fallback, level = 'error') => {
  const message = getErrorMessage(error, fallback)
  notificationStore.addNotification(level, message)
  return message
}
