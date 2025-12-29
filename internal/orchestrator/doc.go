// Package orchestrator coordinates inter-terminal communication.
//
// Intended responsibilities:
//   - Route messages between terminal sessions based on policy.
//   - Maintain shared state or metadata for multi-agent workflows.
//   - Provide hooks for auditing, throttling, and fan-out.
//   - Encapsulate orchestration logic so terminal and API packages stay focused.
package orchestrator
