// Pinned 0.164 disables double-press quit: active Ctrl-C interrupts; an idle
// Ctrl-C exits. Never batch them or infer idle from a deadline/parent liveness.
export function terminalControls(control) {
  if (!['eof', 'interrupt'].includes(control)) throw Error('Unknown terminal control fixture');
  let phase = 'barrier', offset = 0, settledCount = 0;
  const events = [];
  return {
    events,
    observe(output, authorityReady) {
      const settled = [...output.matchAll(/\x1b\]7501;state=(idle|done)/g)];
      const stateBeforeControl = [...output.matchAll(/\x1b\]7501;state=([a-z]+)/g)].at(-1)?.[1] ?? null;
      if (phase === 'barrier' && authorityReady && output.includes('Native core slice no-model barrier')) {
        // With double-press quit disabled, one Ctrl-C after an already settled
        // turn begins shutdown. Its later idle/clear output is not another
        // invitation for control input while terminal modes are restoring.
        offset = output.length; settledCount = settled.length;
        phase = control === 'eof' || ['idle', 'done'].includes(stateBeforeControl) ? 'done' : 'interrupted';
        events.push({ action: control === 'eof' ? 'eof' : 'ctrl-c-after-barrier', outputOffset: offset, settledCount,
          stateBeforeControl, awaitingSettledControl: phase === 'interrupted', observedAt: new Date().toISOString() });
        return control === 'eof' ? '\x04' : '\x03';
      }
      if (phase === 'interrupted' && settled.length > settledCount) {
        phase = 'done'; events.push({ action: 'ctrl-c-after-settled', outputOffset: output.length, settledCount: settled.length,
          settledState: settled.at(-1)[1], conversationInterruptedObserved: output.slice(offset).includes('Conversation interrupted'), observedAt: new Date().toISOString() });
        return '\x03';
      }
      return null;
    },
  };
}
