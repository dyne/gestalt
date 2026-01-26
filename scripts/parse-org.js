#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const filename = process.argv[2];
if (!filename) {
  console.error('Usage: parse-org.js <filename>');
  process.exit(1);
}

const loadParse = async () => {
  const module = await import('orga');
  if (!module || typeof module.parse !== 'function') {
    throw new Error('orga parse function not available');
  }
  return module.parse;
};

(async () => {
  try {
    const parse = await loadParse();
    const resolved = path.resolve(filename);
    const content = fs.readFileSync(resolved, 'utf8');
    const ast = parse(content);
    process.stdout.write(JSON.stringify(ast, null, 2));
  } catch (error) {
    const message = error && error.message ? error.message : String(error);
    console.error(`Parse error: ${message}`);
    process.exit(1);
  }
})();
