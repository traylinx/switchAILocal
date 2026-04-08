#!/usr/bin/env node

// switchAILocal npx launcher
// Downloads the pre-built binary for the current platform and runs it.
// Binary source: GitHub Releases (https://github.com/traylinx/switchAILocal/releases)

import { execFileSync, spawnSync } from "node:child_process";
import { createWriteStream, existsSync, mkdirSync, chmodSync, unlinkSync, writeFileSync } from "node:fs";
import { get as httpsGet } from "node:https";
import { homedir, platform, arch } from "node:os";
import { join } from "node:path";

const REPO = "traylinx/switchAILocal";
const BINARY_NAME = "switchAILocal";
const DEFAULT_PORT = 18080;

// ── Platform Detection ──────────────────────────────────────────

function getPlatform() {
  const p = platform();
  const a = arch();

  const osMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { arm64: "arm64", x64: "amd64", ia32: "386" };

  const os = osMap[p];
  const cpu = archMap[a];

  if (!os || !cpu) {
    console.error(`Unsupported platform: ${p}/${a}`);
    process.exit(1);
  }

  const ext = os === "windows" ? ".zip" : ".tar.gz";
  const bin = os === "windows" ? `${BINARY_NAME}.exe` : BINARY_NAME;

  return { os, arch: cpu, ext, bin };
}

// ── Version Resolution ──────────────────────────────────────────

function parseVersion(args) {
  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--version" && args[i + 1]) {
      args.splice(i, 2);
      return args[i + 1];
    }
    if (args[i].startsWith("--version=")) {
      const ver = args[i].split("=")[1];
      args.splice(i, 1);
      return ver;
    }
  }
  return "latest";
}

async function resolveLatestVersion() {
  return new Promise((resolve, reject) => {
    const options = {
      hostname: "api.github.com",
      path: `/repos/${REPO}/releases/latest`,
      headers: { "User-Agent": "switchailocal-npx" },
    };
    httpsGet(options, (res) => {
      if (res.statusCode === 302 || res.statusCode === 301) {
        // Follow redirect
        httpsGet(res.headers.location, { headers: options.headers }, handleResponse);
        return;
      }
      handleResponse(res);
    }).on("error", reject);

    function handleResponse(res) {
      let data = "";
      res.on("data", (chunk) => (data += chunk));
      res.on("end", () => {
        try {
          const release = JSON.parse(data);
          if (release.tag_name) {
            resolve(release.tag_name);
          } else {
            reject(new Error("No releases found"));
          }
        } catch (e) {
          reject(e);
        }
      });
      res.on("error", reject);
    }
  });
}

// ── Cache Directory ─────────────────────────────────────────────

function getCacheDir(version) {
  const home = homedir();
  let base;
  switch (platform()) {
    case "darwin":
      base = join(home, "Library", "Caches", "switchailocal");
      break;
    case "win32":
      base = join(process.env.LOCALAPPDATA || join(home, "AppData", "Local"), "switchailocal");
      break;
    default:
      base = join(process.env.XDG_CACHE_HOME || join(home, ".cache"), "switchailocal");
  }
  return join(base, version, "bin");
}

// ── Download ────────────────────────────────────────────────────

function followRedirects(url) {
  return new Promise((resolve, reject) => {
    httpsGet(url, { headers: { "User-Agent": "switchailocal-npx" } }, (res) => {
      if (res.statusCode === 302 || res.statusCode === 301) {
        followRedirects(res.headers.location).then(resolve, reject);
        return;
      }
      if (res.statusCode !== 200) {
        reject(new Error(`HTTP ${res.statusCode} fetching ${url}`));
        return;
      }
      resolve(res);
    }).on("error", reject);
  });
}

async function downloadAndExtract(version, plat, destDir) {
  // GoReleaser strips the 'v' prefix from the version in archive names
  // but the GitHub release tag keeps it. e.g. tag=v1.0.0, archive=switchAILocal_1.0.0_...
  const archiveVersion = version.startsWith("v") ? version.slice(1) : version;
  const archiveName = `${BINARY_NAME}_${archiveVersion}_${plat.os}_${plat.arch}${plat.ext}`;
  const url = `https://github.com/${REPO}/releases/download/${version}/${archiveName}`;

  console.log(`Downloading switchAILocal ${version} for ${plat.os}/${plat.arch}...`);

  mkdirSync(destDir, { recursive: true });
  const tmpFile = join(destDir, `download-${process.pid}.tmp`);

  try {
    const res = await followRedirects(url);
    const total = parseInt(res.headers["content-length"], 10) || 0;
    let downloaded = 0;

    await new Promise((resolve, reject) => {
      const ws = createWriteStream(tmpFile);
      res.on("data", (chunk) => {
        downloaded += chunk.length;
        if (total > 0) {
          const pct = ((downloaded / total) * 100).toFixed(0);
          const mb = (downloaded / 1048576).toFixed(1);
          process.stdout.write(`\r  ${pct}% (${mb} MB)`);
        }
      });
      res.pipe(ws);
      ws.on("finish", () => { console.log(""); resolve(); });
      ws.on("error", reject);
      res.on("error", reject);
    });

    // Extract — only the binary from the archive
    if (plat.ext === ".tar.gz") {
      execFileSync("tar", ["xzf", tmpFile, "-C", destDir, "--include", plat.bin], { stdio: "pipe" });
    } else {
      // Windows: use PowerShell to extract
      execFileSync("powershell", [
        "-Command",
        `Expand-Archive -Path '${tmpFile}' -DestinationPath '${destDir}' -Force`,
      ], { stdio: "pipe" });
    }

    chmodSync(join(destDir, plat.bin), 0o755);
  } finally {
    try { unlinkSync(tmpFile); } catch {}
  }
}

// ── Config Setup ────────────────────────────────────────────────

function ensureConfig() {
  const home = homedir();
  const configDir = join(home, ".switchailocal");
  const configFile = join(configDir, "config.yaml");

  if (!existsSync(configFile)) {
    mkdirSync(configDir, { recursive: true });
    const defaultConfig = `# switchAILocal configuration
# See: https://github.com/traylinx/switchAILocal#configuration

port: ${DEFAULT_PORT}

# Uncomment and add your API keys:
# gemini-api-key:
#   - api-key: "your-gemini-key"
# claude-api-key:
#   - api-key: "your-claude-key"
# openai-api-key:
#   - api-key: "your-openai-key"

# Auto-discover local models:
ollama:
  enabled: true
  auto-discover: true
`;
    writeFileSync(configFile, defaultConfig, { mode: 0o600 });
    console.log(`Created default config at ${configFile}`);
    console.log(`Edit it to add your API keys, then restart.\n`);
  }

  return configFile;
}

// ── Main ────────────────────────────────────────────────────────

async function main() {
  const args = process.argv.slice(2);
  let version = parseVersion(args);

  const plat = getPlatform();

  // Resolve latest version
  if (version === "latest") {
    try {
      version = await resolveLatestVersion();
      console.log(`Latest version: ${version}`);
    } catch (e) {
      console.error(`Could not resolve latest version: ${e.message}`);
      console.error("Tip: specify a version with --version=v1.0.0");
      process.exit(1);
    }
  }

  // Check cache
  const cacheDir = getCacheDir(version);
  const binaryPath = join(cacheDir, plat.bin);

  if (!existsSync(binaryPath)) {
    try {
      await downloadAndExtract(version, plat, cacheDir);
    } catch (e) {
      console.error(`\nFailed to download: ${e.message}`);
      console.error(`\nTry manually: https://github.com/${REPO}/releases`);
      process.exit(1);
    }
  }

  // Verify binary works
  try {
    execFileSync(binaryPath, ["--version"], { stdio: "pipe", timeout: 5000 });
  } catch {
    // Binary might not support --version flag, that's ok
  }

  // Run
  const result = spawnSync(binaryPath, args, {
    stdio: "inherit",
    env: { ...process.env },
  });

  process.exit(result.status ?? 1);
}

main().catch((e) => {
  console.error(e.message);
  process.exit(1);
});
