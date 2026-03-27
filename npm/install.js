#!/usr/bin/env node

/**
 * Postinstall script for the cr npm package.
 * Downloads the correct Go binary for the user's platform.
 *
 * Follows the esbuild/turbo pattern:
 * - Detects OS + arch
 * - Downloads prebuilt binary from GitHub releases
 * - Places it in bin/
 */

const https = require("https");
const http = require("http");
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");
const os = require("os");

const VERSION = "1.0.0";
const BINARY_NAME = process.platform === "win32" ? "cr.exe" : "cr";
const BIN_DIR = path.join(__dirname, "bin");
const BINARY_PATH = path.join(BIN_DIR, BINARY_NAME);

// GitHub release URL pattern
// Update this to your actual GitHub releases URL
const REPO = "qzhello/code-review";
const BASE_URL = `https://github.com/${REPO}/releases/download/v${VERSION}`;

function getPlatformKey() {
  const platform = process.platform;
  const arch = process.arch;

  const platformMap = {
    darwin: "darwin",
    linux: "linux",
    win32: "windows",
  };

  const archMap = {
    x64: "amd64",
    arm64: "arm64",
  };

  const os = platformMap[platform];
  const cpu = archMap[arch];

  if (!os || !cpu) {
    throw new Error(
      `Unsupported platform: ${platform}-${arch}. ` +
        `Supported: darwin-x64, darwin-arm64, linux-x64, linux-arm64, win32-x64`
    );
  }

  return `${os}-${cpu}`;
}

function getDownloadUrl() {
  const key = getPlatformKey();
  const ext = process.platform === "win32" ? ".exe" : "";
  return `${BASE_URL}/cr-${key}${ext}`;
}

function download(url) {
  return new Promise((resolve, reject) => {
    const get = url.startsWith("https") ? https.get : http.get;
    get(url, (res) => {
      // Follow redirects
      if (res.statusCode === 301 || res.statusCode === 302) {
        return download(res.headers.location).then(resolve).catch(reject);
      }

      if (res.statusCode !== 200) {
        reject(new Error(`Download failed: HTTP ${res.statusCode} from ${url}`));
        return;
      }

      const chunks = [];
      res.on("data", (chunk) => chunks.push(chunk));
      res.on("end", () => resolve(Buffer.concat(chunks)));
      res.on("error", reject);
    }).on("error", reject);
  });
}

async function main() {
  // Skip if binary already exists (e.g., from local build)
  if (fs.existsSync(BINARY_PATH)) {
    try {
      execSync(`"${BINARY_PATH}" version`, { stdio: "pipe" });
      console.log(`cr binary already exists at ${BINARY_PATH}`);
      return;
    } catch {
      // Binary exists but doesn't work, re-download
    }
  }

  // Ensure bin directory exists
  fs.mkdirSync(BIN_DIR, { recursive: true });

  const url = getDownloadUrl();
  const platformKey = getPlatformKey();

  console.log(`Downloading cr ${VERSION} for ${platformKey}...`);
  console.log(`  URL: ${url}`);

  try {
    const data = await download(url);
    fs.writeFileSync(BINARY_PATH, data);
    fs.chmodSync(BINARY_PATH, 0o755);
    console.log(`  Installed cr to ${BINARY_PATH}`);

    // Verify
    try {
      const version = execSync(`"${BINARY_PATH}" version`, {
        encoding: "utf-8",
      }).trim();
      console.log(`  ${version}`);
    } catch {
      console.warn("  Warning: binary installed but version check failed");
    }
  } catch (err) {
    console.error(`\nFailed to download cr binary: ${err.message}`);
    console.error(`\nYou can build from source instead:`);
    console.error(`  git clone https://github.com/${REPO}.git`);
    console.error(`  cd code-review && go build -o cr .`);
    process.exit(1);
  }
}

main();
