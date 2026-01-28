<script>
  import EventActivityAssigner from '../../src/components/EventActivityAssigner.svelte'

  export let trigger = null
  export let activityDefs = []
  export let bindings = []

  let lastEvent = ''
  let lastDetail = null

  const record = (eventName, detail) => {
    lastEvent = eventName
    lastDetail = detail
  }

  const handleAssign = (event) => record('assign_activity', event.detail)
  const handleUnassign = (event) => record('unassign_activity', event.detail)
  const handleUpdate = (event) => record('update_activity_config', event.detail)
</script>

<EventActivityAssigner
  {trigger}
  {activityDefs}
  {bindings}
  on:assign_activity={handleAssign}
  on:unassign_activity={handleUnassign}
  on:update_activity_config={handleUpdate}
/>

<div data-testid="last-event">{lastEvent}</div>
<div data-testid="last-detail">{lastDetail ? JSON.stringify(lastDetail) : ''}</div>
