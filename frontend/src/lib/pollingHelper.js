export const createPollingHelper = ({ intervalMs, onPoll }) => {
  let timer = null

  const start = () => {
    if (timer) return
    timer = setInterval(() => {
      onPoll()
    }, intervalMs)
  }

  const stop = () => {
    if (!timer) return
    clearInterval(timer)
    timer = null
  }

  const isActive = () => Boolean(timer)

  return {
    start,
    stop,
    isActive,
  }
}
