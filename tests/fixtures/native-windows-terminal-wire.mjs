// Diagnostic-only strict decoder. Never resynchronizes corruption into a claim.
export function terminalVariants(source) {
  const eol = source.includes('\r\n') ? '\r\n' : '\n';
  const replace = (from, to) => {
    from = from.replaceAll('\n', eol); to = to.replaceAll('\n', eol);
    if (source.split(from).length !== 2) throw Error(`Exactly one diagnostic injection required: ${from}`);
    return source.replace(from, to);
  };
  return [
    { name: 'primary', source, role: 'unchanged production candidate' },
    { name: 'zero-startup-fields', role: 'test-only STARTUPINFO standard fields', source: replace(
      'startup.StartupInfo.wShowWindow = SW_HIDE; startup.StartupInfo.hStdInput = input_read;\n  startup.StartupInfo.hStdOutput = output_write; startup.StartupInfo.hStdError = error_write; startup.lpAttributeList = attributes;',
      'startup.StartupInfo.wShowWindow = SW_HIDE;\n  if (!terminal_columns) { startup.StartupInfo.hStdInput = input_read; startup.StartupInfo.hStdOutput = output_write; startup.StartupInfo.hStdError = error_write; }\n  startup.lpAttributeList = attributes;') },
    { name: 'empty-parent-stdio', role: 'test-only parent standard-handle table; saved control handles unchanged', source: replace(
      'BOOL created = line && CreateProcessW',
      'if (terminal_columns) { SetStdHandle(STD_INPUT_HANDLE, NULL); SetStdHandle(STD_OUTPUT_HANDLE, NULL); SetStdHandle(STD_ERROR_HANDLE, NULL); }\n  BOOL created = line && CreateProcessW') },
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
