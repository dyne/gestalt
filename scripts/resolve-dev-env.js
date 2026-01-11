#!/usr/bin/env node
"use strict";

const net = require("net");

function hasScheme(value) {
  return /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(value);
}

function parsePortFromUrl(rawUrl) {
  if (!rawUrl) {
    return "";
  }
  let scheme = "";
  let port = "";
  if (hasScheme(rawUrl)) {
    try {
      const parsed = new URL(rawUrl);
      scheme = parsed.protocol.replace(":", "");
      port = parsed.port || "";
    } catch (err) {
      scheme = "";
      port = "";
    }
  }
  if (port) {
    return port;
  }
  return scheme === "https" ? "443" : "80";
}

function shellEscape(value) {
  return "'" + value.replace(/'/g, "'\"'\"'") + "'";
}

function output(port, url) {
  process.stdout.write(
    `BACKEND_PORT=${shellEscape(port)}\nBACKEND_URL=${shellEscape(url)}\n`
  );
}

function getFreePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.on("error", reject);
    server.listen(0, () => {
      const address = server.address();
      const port = typeof address === "string" ? "" : String(address.port);
      server.close((err) => {
        if (err) {
          reject(err);
          return;
        }
        resolve(port);
      });
    });
  });
}

async function main() {
  let backendPort = process.env.GESTALT_PORT || "";
  let backendURL = process.env.GESTALT_BACKEND_URL || "";

  if (!backendPort && backendURL) {
    backendPort = parsePortFromUrl(backendURL);
  }
  if (!backendPort) {
    backendPort = await getFreePort();
  }
  if (!backendURL) {
    backendURL = `http://localhost:${backendPort}`;
  }

  if (!backendPort) {
    throw new Error("failed to resolve backend port");
  }

  output(backendPort, backendURL);
}

main().catch((err) => {
  console.error(err && err.message ? err.message : String(err));
  process.exit(1);
});
