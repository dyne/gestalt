# UI Freeze Triage

Use this checklist any time tabs/buttons stop responding. Capture everything listed here for each occurrence.

## Repro checklist

- Build mode:
  - Vite dev server (npm run dev) or embedded/prod (backend serving frontend/dist)
  - HMR enabled or disabled
- Browser + version, OS
- Timestamp (local time) and timezone
- Active tab/view when the freeze starts
- Last action before the freeze (click, keyboard input, API action)
- Console:
  - First thrown error with stack (copy full stack)
  - Whether new logs appear after the freeze
- Network:
  - Last failing request (status + payload shape)
  - Pay attention to /api/skills, /api/agents, /api/plans, /api/logs, /api/metrics/summary
- CPU/Memory:
  - Is CPU pegged or idle after the freeze?
  - Any long tasks visible in Performance tab

## Triage decision tree

1. If there is a thrown error around the time of the freeze:
   - Treat as crash-mode (record error + stack trace + sourcemap output).
2. If there is no error and CPU is idle but clicks do not land:
   - Treat as overlay/pointer-events issue (look for modals, invisible overlays).
3. If CPU is pegged and UI janks:
   - Treat as perf/event-storm issue (logs, websocket bursts, render loops).

## Capture format (paste into report)

- Timestamp:
- Build mode:
- Browser/OS:
- Active tab/view:
- Last action:
- Console error:
- Network failure:
- CPU state:
- Notes:
