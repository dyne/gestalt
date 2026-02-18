#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const filename = process.argv[2];
if (!filename) {
  console.error('Usage: parse-org.js <filename>');
  process.exit(1);
}

const KEYWORDS = new Set(['TODO', 'WIP', 'DONE']);

const parseFallback = (content) => {
  const properties = {};
  const rawLines = content.split('\n');
  const lineInfo = [];
  let offset = 0;

  for (let i = 0; i < rawLines.length; i += 1) {
    const raw = rawLines[i];
    const hasNext = i < rawLines.length - 1;
    const text = raw.endsWith('\r') ? raw.slice(0, -1) : raw;
    const start = offset;
    const end = start + text.length;
    lineInfo.push({ text, start, end });
    offset += raw.length + (hasNext ? 1 : 0);
  }

  const roots = [];
  const stack = [];

  for (const line of lineInfo) {
    const meta = line.text.match(/^#\+([A-Za-z0-9_]+):\s*(.*)$/);
    if (meta) {
      properties[meta[1].toLowerCase()] = meta[2].trim();
      continue;
    }

    const headingMatch = line.text.match(/^(\*+)\s+(.*)$/);
    if (!headingMatch) {
      continue;
    }

    const level = headingMatch[1].length;
    let rest = headingMatch[2].trim();
    let keyword = '';
    let priority = '';
    const tags = [];

    const keywordMatch = rest.match(/^([A-Za-z][A-Za-z0-9_-]*)\s*(.*)$/);
    if (keywordMatch) {
      const candidate = keywordMatch[1].toUpperCase();
      if (KEYWORDS.has(candidate)) {
        keyword = candidate;
        rest = keywordMatch[2].trim();
      }
    }

    const priorityMatch = rest.match(/^\[#([A-Za-z])\]\s*(.*)$/);
    if (priorityMatch) {
      priority = priorityMatch[1].toUpperCase();
      rest = priorityMatch[2].trim();
    }

    const tagsMatch = rest.match(/^(.*?)\s+(:[^:\s]+(?::[^:\s]+)*:)\s*$/);
    if (tagsMatch) {
      rest = tagsMatch[1].trim();
      tags.push(...tagsMatch[2].split(':').filter(Boolean));
    }

    const headlineChildren = [];
    if (keyword) {
      headlineChildren.push({ type: 'todo', keyword });
    }
    if (priority) {
      headlineChildren.push({ type: 'priority', value: `[#${priority}]` });
    }
    if (rest) {
      headlineChildren.push({ type: 'text', value: rest });
    }

    const section = {
      type: 'section',
      level,
      children: [
        {
          type: 'headline',
          level,
          keyword,
          priority,
          tags,
          actionable: keyword !== '' && keyword !== 'DONE',
          children: headlineChildren,
          position: {
            start: { offset: line.start },
            end: { offset: line.end },
          },
        },
      ],
    };

    while (stack.length > 0 && stack[stack.length - 1].level >= level) {
      stack.pop();
    }

    if (stack.length === 0) {
      roots.push(section);
    } else {
      stack[stack.length - 1].children.push(section);
    }
    stack.push(section);
  }

  return {
    type: 'document',
    properties,
    children: roots,
  };
};

const isMissingOrga = (error) => {
  const message = error && error.message ? error.message : String(error);
  return message.includes("Cannot find package 'orga'") || (error && error.code === 'ERR_MODULE_NOT_FOUND' && message.includes('orga'));
};

const loadParse = async () => {
  try {
    const module = await import('orga');
    if (!module || typeof module.parse !== 'function') {
      throw new Error('orga parse function not available');
    }
    return module.parse;
  } catch (error) {
    if (isMissingOrga(error)) {
      return parseFallback;
    }
    throw error;
  }
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
