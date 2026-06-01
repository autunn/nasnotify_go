import childProcess from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "..");
const packageRoot = path.join(repoRoot, "packaging", "ugreen-native-app");
const defaultUgcliPath = path.join(repoRoot, "tools", "ugcli", "ugcli-v1.1.0.12-windows-amd64.exe");
const forcedRoute = "/nasnotify";
const forcedCategory = "category.media.manage";
const forcedPublisherType = "ugos";
const desktopSupports = ["pc"];
const servicePort = 21010;

const args = parseArgs(process.argv.slice(2));
const arch = args.arch || "all";
const build = args.build || "1";
const ugcliPath = args.ugcli ? path.resolve(args.ugcli) : defaultUgcliPath;
const packArchList = arch === "all" ? ["amd64", "arm64"] : [arch];

if (!fs.existsSync(ugcliPath)) {
  throw new Error(`ugcli not found: ${ugcliPath}`);
}

fs.rmSync(path.join(packageRoot, "build_dir"), { recursive: true, force: true });
run(ugcliPath, ["check"], packageRoot, { stdio: "inherit" });
for (const packArch of packArchList) {
  await runPackWithDesktopRoutePatch(ugcliPath, ["pack", "--arch", packArch, "--build", String(build)], packageRoot);
}

const builtUpks = findBuiltUpks(arch);
if (builtUpks.length === 0) {
  throw new Error("ugcli did not produce any UPK files");
}

const generatedConfig = readGeneratedConfig();
printAccessSummary(generatedConfig);

console.log("Signed UPK files:");
for (const file of builtUpks) {
  console.log(`- ${file}`);
}

function findBuiltUpks(requestedArch) {
  const upkDir = path.join(packageRoot, "build_dir", "pkgs", "upk");
  if (!fs.existsSync(upkDir)) {
    return [];
  }

  const files = fs.readdirSync(upkDir)
    .filter((name) => name.endsWith(".upk"))
    .map((name) => path.join(upkDir, name));

  if (requestedArch === "all") {
    return files.sort();
  }
  return files.filter((file) => path.basename(file).startsWith(`${requestedArch}_`)).sort();
}

function run(command, commandArgs, cwd, options = {}) {
  childProcess.execFileSync(command, commandArgs, {
    cwd,
    stdio: options.stdio || "pipe"
  });
}

async function runPackWithDesktopRoutePatch(command, commandArgs, cwd) {
  const tarballPath = path.join(packageRoot, "build_dir", "tar", "com.autunn.nasnotifyfresh.tar.gz");
  const ugbPath = path.join(packageRoot, "build_dir", "com.autunn.nasnotifyfresh.ugb");

  fs.rmSync(tarballPath, { force: true });
  fs.rmSync(ugbPath, { force: true });
  patchDesktopRouteMode();

  await new Promise((resolve, reject) => {
    const child = childProcess.spawn(command, commandArgs, {
      cwd,
      stdio: "inherit"
    });
    let settled = false;

    const timer = setInterval(() => {
      if (fs.existsSync(tarballPath)) {
        clearInterval(timer);
        return;
      }
      try {
        patchDesktopRouteMode();
      } catch (error) {
        clearInterval(timer);
        if (!settled) {
          settled = true;
          reject(error);
        }
        child.kill();
      }
    }, 25);

    child.once("error", (error) => {
      clearInterval(timer);
      if (!settled) {
        settled = true;
        reject(error);
      }
    });

    child.once("exit", (code) => {
      clearInterval(timer);
      try {
        patchDesktopRouteMode();
      } catch (error) {
        if (!settled) {
          settled = true;
          reject(error);
        }
        return;
      }
      if (!settled) {
        settled = true;
        if (code === 0) {
          resolve();
        } else {
          reject(new Error(`ugcli pack failed with exit code ${code ?? "unknown"}`));
        }
      }
    });
  });
}

function readGeneratedConfig() {
  const configPath = path.join(packageRoot, "build_dir", "rootfs", "config.json");
  if (!fs.existsSync(configPath)) {
    throw new Error(`generated config.json not found: ${configPath}`);
  }
  return JSON.parse(fs.readFileSync(configPath, "utf8"));
}

function printAccessSummary(config) {
  const route = config?.route || "(missing)";
  const accessMode = config?.baseAccessInfo?.accessMode;
  const openType = config?.baseAccessInfo?.openType || "(missing)";
  const specVersion = config?.specVersion || "(none)";
  const checkParameter = config?.checkParameter
    ? `present readyFlagType=${config.checkParameter.readyFlagType ?? "(missing)"} port=${config.checkParameter.portChecker?.portFlag ?? "(missing)"}`
    : "(none)";
  const runtimeEnvironment = config?.runtimeEnvironment ? "present" : "(none)";
  const supports = Array.isArray(config?.accessCtrl?.supports)
    ? config.accessCtrl.supports.join(",")
    : "(none)";
  const baseSupports = Array.isArray(config?.baseAccessInfo?.supports)
    ? config.baseAccessInfo.supports.join(",")
    : "(none)";
  const proxies = Array.isArray(config?.runtimeEnvironment?.proxy)
    ? config.runtimeEnvironment.proxy.map((item) => `${item.location}:${item.type}->${item.target}`).join(", ")
    : "(none)";
  const modeLabel = accessMode === 0 ? "route" : accessMode === 1 ? "port+proxy_path" : `unknown(${String(accessMode)})`;

  console.log("Generated package access profile:");
  console.log(`- route: ${route}`);
  console.log(`- accessMode: ${modeLabel}`);
  console.log(`- openType: ${openType}`);
  console.log(`- specVersion: ${specVersion}`);
  console.log(`- runtimeEnvironment: ${runtimeEnvironment}`);
  console.log(`- checkParameter: ${checkParameter}`);
  console.log(`- accessCtrl.supports: ${supports}`);
  console.log(`- baseAccessInfo.supports: ${baseSupports}`);
  console.log(`- proxy: ${proxies}`);
}

function patchDesktopRouteMode() {
  const rootfsDir = path.join(packageRoot, "build_dir", "rootfs");
  const configPath = path.join(rootfsDir, "config.json");
  if (!fs.existsSync(configPath)) {
    return;
  }

  const original = JSON.parse(fs.readFileSync(configPath, "utf8"));
  const patched = buildDesktopRouteConfig(original);
  const currentText = JSON.stringify(original);
  const patchedText = JSON.stringify(patched);

  if (currentText !== patchedText) {
    fs.writeFileSync(configPath, `${JSON.stringify(patched)}\n`);
  }

  const checkAppPath = path.join(rootfsDir, ".check-app");
  if (fs.existsSync(checkAppPath)) {
    syncCheckApp(checkAppPath, rootfsDir);
  }
}

function buildDesktopRouteConfig(config) {
  const patched = structuredClone(config);

  patched.route = forcedRoute;
  patched.appType = 1;
  patched.category = forcedCategory;
  patched.tagTypes = ["media"];
  patched.publisherType = forcedPublisherType;
  patched.baseAccessInfo = {
    accessMode: 0,
    openType: "inner",
    supports: [...desktopSupports],
    innerInfo: {},
    portInfo: {
      httpsEnable: false,
      port: "",
      httpsPort: "",
      urlPath: ""
    },
    portalInfo: {
      urlPath: ""
    }
  };

  delete patched.specVersion;

  patched.runtimeEnvironment = {
    user: {
      createMethod: "internal"
    },
    service: {
      execStart: `bin/nasnotify --port=${servicePort}`
    },
    permissions: ["NETWORK.ACCESS_INTERNET"]
  };

  patched.checkParameter = {
    readyFlagType: 2,
    portChecker: {
      portFlag: servicePort
    }
  };

  if (patched.accessCtrl) {
    if ("jumpInfo" in patched.accessCtrl) {
      patched.accessCtrl.jumpInfo = null;
    }
    patched.accessCtrl.supports = [...desktopSupports];
  } else {
    patched.accessCtrl = {
      accessType: 0,
      isAuthRequired: false,
      urlAppAccesses: null,
      gateWayAppAccesses: null,
      supports: [...desktopSupports],
      jumpInfo: null
    };
  }

  patched.searchServiceAddress = patched.searchServiceAddress || "";
  patched.subApps = null;
  patched.parentAppID = "";
  patched.parentAppName = "";

  return patched;
}

function syncCheckApp(checkAppPath, rootfsDir) {
  const manifest = JSON.parse(fs.readFileSync(checkAppPath, "utf8"));
  const normalizedManifest = {};

  for (const [relativePath, currentDigest] of Object.entries(manifest)) {
    const normalizedKey = relativePath.replace(/\\/g, "/");
    const absolutePath = path.join(rootfsDir, relativePath.replace(/[\\/]/g, path.sep));
    if (!fs.existsSync(absolutePath) || fs.statSync(absolutePath).isDirectory()) {
      normalizedManifest[normalizedKey] = currentDigest;
      continue;
    }
    const digest = crypto.createHash("md5").update(fs.readFileSync(absolutePath)).digest("hex");
    normalizedManifest[normalizedKey] = digest;
  }

  fs.writeFileSync(checkAppPath, JSON.stringify(normalizedManifest));
}

function parseArgs(items) {
  const parsed = {};
  for (let i = 0; i < items.length; i += 1) {
    const item = items[i];
    if (item === "--arch" || item === "-a") {
      parsed.arch = items[++i];
    } else if (item === "--build" || item === "-b") {
      parsed.build = items[++i];
    } else if (item === "--ugcli") {
      parsed.ugcli = items[++i];
    }
  }
  return parsed;
}
