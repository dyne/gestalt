## Shutdown behavior

Gestalt uses a graceful shutdown sequence on SIGINT/SIGTERM. The first Ctrl+C
starts the shutdown. Additional Ctrl+C presses are logged and ignored so the
graceful sequence can finish.

Shutdown has an overall 5s deadline with a 1s budget per phase. Phases are
ordered to keep dependencies alive while dependents stop (for example, the OTel
SDK stops before the collector, and Temporal workers stop before the dev server).

## Acceptance checklist

- Run 10 start/stop cycles; after each stop there are no defunct (zombie)
  processes left behind by the backend.
- A normal Ctrl+C shutdown logs info-level shutdown entries and avoids
  error-level logs when possible. Any remaining errors should indicate
  unavoidable failures rather than ordering issues.
