#!/usr/bin/env node

// switchAILocal npx launcher
// Downloads the pre-built binary for the current platform and runs it.
// Binary source: GitHub Releases (https://github.com/traylinx/switchAILocal/releases)

import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  createWriteStream, existsSync, mkdirSync, mkdtempSync, chmodSync, readdirSync,
  readFileSync, renameSync, rmSync, statSync, unlinkSync, writeFileSync,
} from "node:fs";
import { get as httpsGet } from "node:https";
import { homedir, platform, arch } from "node:os";
import { join } from "node:path";

const REPO = "traylinx/switchAILocal";
const BINARY_NAME = "switchAILocal";
const DEFAULT_PORT = 18080;
// Platforms the release workflow publishes (.github/workflows/release.yml).
const PUBLISHED = new Set(["darwin/arm64", "linux/amd64", "linux/arm64"]);

// ── Platform Detection ──────────────────────────────────────────

function getPlatform() {
  const p = platform();
  const a = arch();

  const osMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { arm64: "arm64", x64: "amd64", ia32: "386" };

  const os = osMap[p];
  let cpu = archMap[a];

  // An x64 Node under Rosetta on Apple Silicon: the arm64 binary runs natively.
  if (os === "darwin" && cpu === "amd64" && runningUnderRosetta()) {
    cpu = "arm64";
  }

  if (!os || !cpu) {
    console.error(`Unsupported platform: ${p}/${a}`);
    process.exit(1);
  }

  if (!PUBLISHED.has(`${os}/${cpu}`)) {
    console.error(`No pre-built switchAILocal binary is published for ${os}/${cpu}.`);
    console.error(`Published: ${[...PUBLISHED].join(", ")}.`);
    console.error(`Use Docker or build from source: https://github.com/${REPO}#installation`);
    process.exit(1);
  }

  const ext = os === "windows" ? ".zip" : ".tar.gz";
  const bin = os === "windows" ? `${BINARY_NAME}.exe` : BINARY_NAME;

  return { os, arch: cpu, ext, bin };
}

function runningUnderRosetta() {
  try {
    return execFileSync("sysctl", ["-in", "sysctl.proc_translated"], { stdio: ["ignore", "pipe", "ignore"] })
      .toString().trim() === "1";
  } catch {
    return false;
  }
}

// ── Version Resolution ──────────────────────────────────────────

function parseVersion(args) {
  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--version" && args[i + 1]) {
      const ver = args[i + 1];
      args.splice(i, 2);
      return ver;
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

function fetchText(url) {
  return followRedirects(url).then((res) => new Promise((resolve, reject) => {
    let data = "";
    res.setEncoding("utf8");
    res.on("data", (c) => { data += c; if (data.length > 1 << 20) reject(new Error("response too large")); });
    res.on("end", () => resolve(data));
    res.on("error", reject);
  }));
}

// Returns the path of the first regular file called `name` below `dir`.
function findFile(dir, name) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isFile() && entry.name === name) return path;
    if (entry.isDirectory()) {
      const found = findFile(path, name);
      if (found) return found;
    }
  }
  return null;
}

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

    // Verify the archive against the release's checksums.txt.
    const sums = await fetchText(`https://github.com/${REPO}/releases/download/${version}/checksums.txt`);
    const line = sums.split("\n").find((l) => l.trim().endsWith(`  ${archiveName}`) || l.trim().endsWith(` ${archiveName}`));
    const expected = line && line.trim().split(/\s+/)[0];
    const actual = createHash("sha256").update(readFileSync(tmpFile)).digest("hex");
    if (!expected || expected.toLowerCase() !== actual) {
      throw new Error(`checksum mismatch for ${archiveName}`);
    }

    // Extract into a scratch directory and take the binary from wherever the
    // archive puts it (releases since 0.6 nest it in a versioned folder).
    const scratch = mkdtempSync(join(destDir, "extract-"));
    try {
      if (plat.ext === ".tar.gz") {
        execFileSync("tar", ["xzf", tmpFile, "-C", scratch], { stdio: "pipe" });
      } else {
        execFileSync("powershell", [
          "-NoProfile", "-Command",
          `Expand-Archive -LiteralPath '${tmpFile}' -DestinationPath '${scratch}' -Force`,
        ], { stdio: "pipe" });
      }
      const found = findFile(scratch, plat.bin);
      if (!found || !statSync(found).isFile()) {
        throw new Error(`${plat.bin} not found in ${archiveName}`);
      }
      renameSync(found, join(destDir, plat.bin));
    } finally {
      rmSync(scratch, { recursive: true, force: true });
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

# Where OAuth tokens and provider credentials are stored.
auth-dir: "~/.switchailocal"

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

  // Use ~/.switchailocal/config.yaml (created on first run) unless the caller
  // passes -config or the current directory has its own config.yaml.
  const hasConfigFlag = args.some((a) => /^--?config(=|$)/.test(a));
  if (!hasConfigFlag && !existsSync(join(process.cwd(), "config.yaml"))) {
    args.unshift("-config", ensureConfig());
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
