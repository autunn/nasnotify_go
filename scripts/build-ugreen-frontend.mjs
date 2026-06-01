import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..");
const sourceDir = path.join(repoRoot, "frontend", "ugreen-app");
const distDir = path.join(sourceDir, "dist");
const publicDir = path.join(sourceDir, "public");
const targetDir = path.join(repoRoot, "packaging", "ugreen-native-app", "rootfs_common", "www");
const desktopVersionPath = "version.json";
const packageJsonPath = path.join(sourceDir, "package.json");
const officialCompatibleDependencies = {
  "ugos-core": "1.6.19",
  "ugos-pro-builder": "1.2.70"
};

function ensureDir(dir) {
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }
}

function cleanDir(dir) {
  ensureDir(dir);
  for (const entry of readdirSync(dir)) {
    rmSync(path.join(dir, entry), { recursive: true, force: true });
  }
}

cleanDir(targetDir);
if (!existsSync(distDir)) {
  throw new Error(`frontend dist not found: ${distDir}`);
}

if (!existsSync(packageJsonPath)) {
  throw new Error(`frontend package.json not found: ${packageJsonPath}`);
}

const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, "utf8"));

const desktopMetadataFiles = ["version.json"];
for (const fileName of desktopMetadataFiles) {
  const sourcePath = path.join(publicDir, fileName);
  const distPath = path.join(distDir, fileName);
  if (existsSync(sourcePath)) {
    cpSync(sourcePath, distPath, { force: true });
  }
}

cpSync(distDir, targetDir, { recursive: true });

for (const dir of [publicDir, distDir, targetDir]) {
  normalizeVersionFile(dir, packageJson);
}

for (const dir of [distDir, targetDir]) {
  const placeholder = path.join(dir, ".gitkeep");
  if (existsSync(placeholder)) {
    rmSync(placeholder, { force: true });
  }
}

console.log(`UGREEN frontend dist synced to ${targetDir}`);

function normalizeVersionFile(dir, packageJson) {
  const versionPath = path.join(dir, desktopVersionPath);
  if (!existsSync(versionPath)) {
    return;
  }

  const current = JSON.parse(fs.readFileSync(versionPath, "utf8"));
  const versionInfo = normalizeReleaseVersion(packageJson.version, current.version, current.fullVersion);
  const supportLanguages = Array.isArray(current.supportLanguages) && current.supportLanguages.length > 0
    ? current.supportLanguages
    : ["zh-CN", "en-US"];

  const normalized = {
    ...current,
    id: packageJson.appId || packageJson.name,
    name: packageJson.name,
    author: current.author || packageJson.author || "Autunn",
    version: versionInfo.version,
    fullVersion: versionInfo.fullVersion,
    description: current.description || packageJson.description || "",
    supportLanguages,
    dependenciesInfo: buildDependenciesInfo(current.dependenciesInfo, packageJson),
    childApp: current.childApp || {},
    widgets: current.widgets || [],
    trays: current.trays || [],
    contextMenu: current.contextMenu || []
  };

  fs.writeFileSync(versionPath, `${JSON.stringify(normalized, null, 4)}\n`);
}

function buildDependenciesInfo(existing, packageJson) {
  return officialCompatibleDependencies;
}

function normalizeReleaseVersion(packageVersion, currentVersion, currentFullVersion) {
  if (Number.isFinite(currentVersion) && currentVersion > 0 && typeof currentFullVersion === "string" && currentFullVersion.length > 0) {
    return {
      version: currentVersion,
      fullVersion: currentFullVersion
    };
  }

  const [major, minor, patch] = String(packageVersion || "1.0.0")
    .split(".")
    .map((value) => Number.parseInt(value, 10) || 0);

  return {
    version: (major * 100000000) + (minor * 1000000) + (patch * 10000) + 1,
    fullVersion: `v${major}.${minor}.${patch}.0001`
  };
}
