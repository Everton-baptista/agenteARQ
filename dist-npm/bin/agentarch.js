#!/usr/bin/env node
// Thin launcher. The platform binary arrives as an optionalDependency, the pattern esbuild and
// swc use: npm installs only the one that matches, and there is no postinstall script
// downloading anything.
//
// That matters more than convenience. A postinstall that fetches a binary is a supply-chain
// step that runs with the developer's credentials and is invisible in a lockfile — which would
// be a poor thing for a tool whose whole argument is that a pack must be inert data.

const { spawnSync } = require("node:child_process");
const path = require("node:path");

const PLATFORMS = {
  "darwin arm64": "@agentarch/cli-darwin-arm64",
  "darwin x64": "@agentarch/cli-darwin-x64",
  "linux arm64": "@agentarch/cli-linux-arm64",
  "linux x64": "@agentarch/cli-linux-x64",
  "win32 x64": "@agentarch/cli-win32-x64",
};

const key = `${process.platform} ${process.arch}`;
const pkg = PLATFORMS[key];

if (!pkg) {
  console.error(
    `agentarch has no prebuilt binary for ${key}.\n` +
      `Build from source instead:\n` +
      `  go install github.com/Everton-baptista/agenteARQ/cmd/agentarch@latest`
  );
  process.exit(1);
}

let binary;
try {
  const exe = process.platform === "win32" ? "agentarch.exe" : "agentarch";
  binary = path.join(path.dirname(require.resolve(`${pkg}/package.json`)), "bin", exe);
} catch {
  console.error(
    `agentarch could not find ${pkg}.\n\n` +
      `This usually means the install ran with --no-optional or --omit=optional,\n` +
      `which skips the platform binary. Reinstall without it, or:\n` +
      `  go install github.com/Everton-baptista/agenteARQ/cmd/agentarch@latest`
  );
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`agentarch failed to start: ${result.error.message}`);
  process.exit(1);
}
// Exit codes are part of the contract — see spec/normative/06-exit-codes.md. Passing them
// through unchanged is the whole job of this launcher.
process.exit(result.status ?? 1);
