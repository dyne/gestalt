// Diagnostic-only strict decoder. Never resynchronizes corruption into a claim.
export function terminalVariants(source) {
  const eol = source.includes('\r\n') ? '\r\n' : '\n';
  const replace = (from, to) => {
    from = from.replaceAll('\n', eol); to = to.replaceAll('\n', eol);
    if (source.split(from).length !== 2) throw Error(`Exactly one diagnostic injection required: ${from}`);
    return source.replace(from, to);
  };
  const isolation = [
    '  /* ConPTY must install its own console handles in the child. Otherwise the',
    "   * helper's standard-handle table can carry private protocol pipes into it.",
    '   * The captured control_input/control_output values remain open and pinned. */',
    '  if (terminal_columns &&',
    '      (!SetStdHandle(STD_INPUT_HANDLE, NULL) || !SetStdHandle(STD_OUTPUT_HANDLE, NULL) || !SetStdHandle(STD_ERROR_HANDLE, NULL))) {',
    '    message(\'X\', "Owned ConPTY standard-handle isolation failed before payload launch"); CloseHandle(job); free(line); return 2;',
    '  }',
  ].join('\n');
  return [
    { name: 'primary', source, role: 'unchanged production candidate' },
    { name: 'legacy-parent-stdio', role: 'test-only reproduction with standard-handle isolation removed', source: replace(isolation, '  /* test-only legacy standard-handle table */') },
    { name: 'setstd-failure', role: 'test-only failed SetStdHandle before payload', source: replace('!SetStdHandle(STD_INPUT_HANDLE, NULL)', '!(SetLastError(ERROR_INVALID_HANDLE), FALSE)') },
  ];
}
export function encodeFrame(kind, payload = Buffer.alloc(0)) {
  const bytes = Buffer.from(payload), header = Buffer.alloc(5);
  header.writeUInt32LE(bytes.length + 1); header[4] = kind.charCodeAt(0);
  return Buffer.concat([header, bytes]);
}
export function decodeFrames(raw) {
  const frames = []; let offset = 0;
  while (raw.length - offset >= 4) {
    const size = raw.readUInt32LE(offset);
    if (!size || size > 1024 * 1024) return { frames, error: { offset, size, hex: raw.subarray(offset, offset + 64).toString('hex') } };
    if (raw.length - offset < size + 4) break;
    const kind = String.fromCharCode(raw[offset + 4]);
    if (!'ROBAFDXUZ'.includes(kind)) return { frames, error: { offset, kind, hex: raw.subarray(offset, offset + 64).toString('hex') } };
    frames.push({ kind, bytes: raw.subarray(offset + 5, offset + 4 + size) }); offset += size + 4;
  }
  return { frames, pendingBytes: raw.length - offset };
}
