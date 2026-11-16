// Test-only UI quit: pinned0.164 Ctrl-C quits at a settled turn. Wait for the
// submitted blocking turn to settle, then send exactly one control. This case
// proves quit/control forwarding, never active work cancellation.
export function terminalControls(control) {
  if (!['eof', 'interrupt'].includes(control)) throw Error('Unknown terminal control fixture');
  let sent = false;
  const events = [];
  return {
    events,
    observe(output, authorityReady) {
      if (sent || !authorityReady || !output.includes('Native core slice no-model barrier')) return null;
      const statuses = [...output.matchAll(/\x1b\]7501;state=([a-z]+)/g)];
      const latest = statuses.at(-1), working = statuses.findLast(status => status[1] === 'working');
      const settled = [...output.matchAll(/\x1b\]7501;state=(idle|done)/g)];
      // The startup idle marker cannot establish completion of this turn.
      // Rendering may place Done before the HookStopped/barrier text; require
      // both plus actual authority, with the settled status after Working.
      if (control === 'interrupt' && (!latest || !working ||
          !['idle', 'done'].includes(latest[1]) || latest.index <= working.index)) {
        return null;
      }
      sent = true;
      events.push({ action: control === 'eof' ? 'eof' : 'ctrl-c-ui-quit', outputOffset: output.length,
        settledCount: settled.length, stateBeforeControl: latest?.[1] ?? null, awaitingSettledControl: false,
        intent: control === 'eof' ? 'eof' : 'settled-ui-quit', activeCancellationClaimed: false,
        ...(control === 'interrupt' ? { workingStatusOffset: working.index, settledStatusOffset: latest.index } : {}),
        observedAt: new Date().toISOString() });
      return control === 'eof' ? '\x04' : '\x03';
    },
  };
}
