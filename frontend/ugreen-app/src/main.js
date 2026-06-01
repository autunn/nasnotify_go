import cloudWindow from "@ugreen-nas/core/cloudWindow";

const APP_PROXY_PATH = "nasnotify";
const LEGACY_APP_ID = "com.autunn.nasnotifyfresh";
const APP_ROUTE_SEGMENTS = [APP_PROXY_PATH, LEGACY_APP_ID];
const DEFAULT_UGOS_PREFIX = "/ugreen/v1";
const HOST_API_BASE = "/api";
const DESIGN_VIEWPORT_WIDTH = 1200;
const DESIGN_VIEWPORT_HEIGHT = 800;
const CANONICAL_API_BASE_CANDIDATES = [
  HOST_API_BASE,
  `${DEFAULT_UGOS_PREFIX}/${APP_PROXY_PATH}/api`,
  `/${APP_PROXY_PATH}/api`
];
const LEGACY_API_BASE_CANDIDATES = [
  `${DEFAULT_UGOS_PREFIX}/${LEGACY_APP_ID}/api`,
  `/${LEGACY_APP_ID}/api`
];
let ugToken = null;
let API_BASE = null;
let cloudWindowReadyTriggered = false;
let cloudWindowReadyPromise = null;
const PREVIEW_MODE = new URLSearchParams(window.location.search).get("preview");
const INITIAL_DASHBOARD_TAB = new URLSearchParams(window.location.search).get("tab");
const WINDOW_ACTION_OFFSET_Y = 10;
const WINDOW_ACTION_OFFSET_X = 0;

const state = {
  bootstrap: null,
  flash: "",
  gatewayStatus: null,
  dashboardTab: initialDashboardTab()
};

function initialDashboardTab() {
  const tab = String(INITIAL_DASHBOARD_TAB || "").trim().toLowerCase();
  return ["overview", "settings", "enterprise", "gateway", "commands"].includes(tab) ? tab : "settings";
}

function currentPreviewMode() {
  const mode = String(PREVIEW_MODE || "").trim().toLowerCase();
  return ["setup", "login", "dashboard"].includes(mode) ? mode : "";
}

function updateAppScale() {
  if (typeof window === "undefined" || typeof document === "undefined") {
    return;
  }
  const widthScale = window.innerWidth / DESIGN_VIEWPORT_WIDTH;
  const heightScale = window.innerHeight / DESIGN_VIEWPORT_HEIGHT;
  const scale = Math.max(0.78, Math.min(1.08, Math.min(widthScale, heightScale)));
  document.documentElement.style.setProperty("--app-scale", scale.toFixed(4));
}

function isPreviewMode() {
  return Boolean(currentPreviewMode());
}

function previewConfig() {
  return {
    interval_minutes: 5,
    system_status_interval_minutes: 60,
    local_nas_name: "书房绿联 NAS",
    local_nas_host: "192.168.1.9",
    local_nas_port: 9999,
    local_nas_username: "admin",
    local_nas_password: "",
    corpid: "ww-preview-corp",
    agentid: "1000002",
    corpsecret: "",
    token: "",
    encoding_aes_key: "",
    nas_url: "https://nas.example.com",
    photo_url: "",
    proxy_url: "",
    wechat_gateway_url: "http://127.0.0.1:5091",
    wechat_gateway_secret: "preview-gateway-key"
  };
}

function buildPreviewBootstrap(mode) {
  return {
    initialized: mode !== "setup",
    authenticated: mode === "dashboard",
    version: "1.0.0.0001",
    setup_token: "PREVIEW-SETUP-TOKEN",
    config: previewConfig()
  };
}

function buildPreviewGatewayStatus() {
  return {
    configured: true,
    open_api_ready: true,
    entry_bound: true,
    bound: true,
    activated: true,
    need_verify_code: false,
    binding_code: "Q7KD2M",
    bind_time: "2026-06-01 20:15:06",
    entry_bind_time: "2026-06-01 20:11:27",
    qrcode: {
      qrcode: "预览模式二维码"
    },
    tips: [
      "这是本地预览模式，用于检查 PC 端排版，不会发起真实 API 请求。",
      "正式环境下会自动改为 NAS 后端返回的绑定状态与二维码。"
    ],
    last_error: ""
  };
}

function bootstrapSetupToken() {
  return state.bootstrap?.setup_token || "";
}

function normalizePathValue(value) {
  const raw = String(value || "").trim();
  if (!raw) {
    return "";
  }
  const withLeadingSlash = raw.startsWith("/") ? raw : `/${raw}`;
  const normalized = withLeadingSlash.replace(/\/{2,}/g, "/").replace(/\/$/, "");
  return normalized || "/";
}

function normalizeApiBase(value) {
  const normalized = normalizePathValue(value);
  if (!normalized || normalized === "/") {
    return "";
  }
  return normalized.endsWith("/api") ? normalized : `${normalized}/api`;
}

function addApiCandidate(values, seen, value) {
  const normalized = normalizeApiBase(value);
  if (!normalized || seen.has(normalized)) {
    return;
  }
  seen.add(normalized);
  values.push(normalized);
}

function relativePathFromBase(baseURL, suffix) {
  try {
    const url = new URL(suffix, baseURL);
    return `${url.pathname}${url.search || ""}`.replace(/\/$/, "");
  } catch (_) {
    return "";
  }
}

function appRoutePrefixesFromPath(pathname) {
  const currentPath = normalizePathValue(String(pathname || "/").split(/[?#]/)[0]);
  const segments = currentPath.split("/").filter(Boolean);
  const prefixes = [];

  for (let index = 0; index < segments.length; index += 1) {
    if (APP_ROUTE_SEGMENTS.includes(segments[index])) {
      prefixes.push(`/${segments.slice(0, index + 1).join("/")}`);
      break;
    }
  }

  const ugosVersionMatch = currentPath.match(/^(\/ugreen\/v\d+)(?:\/|$)/);
  if (ugosVersionMatch) {
    prefixes.push(`${ugosVersionMatch[1]}/${APP_PROXY_PATH}`);
    if (currentPath.includes(`/${LEGACY_APP_ID}`)) {
      prefixes.push(`${ugosVersionMatch[1]}/${LEGACY_APP_ID}`);
    }
  }

  return prefixes;
}

function resolveApiBase() {
  return API_BASE || apiBaseCandidates()[0] || HOST_API_BASE;
}

function wechatQRCodeImageSrc() {
  return `${resolveApiBase()}/wechat/qrcode?t=${Date.now()}`;
}

function appRoot() {
  return document.getElementById("app");
}

function escapeHtml(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

async function ensureCloudWindowReady() {
  if (cloudWindowReadyTriggered) {
    return;
  }
  if (cloudWindowReadyPromise) {
    await cloudWindowReadyPromise;
    return;
  }

  cloudWindowReadyPromise = (async () => {
    try {
      const shouldCallReady =
        typeof window !== "undefined" &&
        typeof cloudWindow?.ready === "function" &&
        (window.isClient === true || typeof window.UGOSLauncher !== "undefined");
      if (shouldCallReady) {
        await cloudWindow.ready();
      }
    } catch (_) {
    } finally {
      cloudWindowReadyTriggered = true;
      cloudWindowReadyPromise = null;
    }
  })();

  await cloudWindowReadyPromise;
}

function getCloudWindowCandidates() {
  const candidates = [];
  if (typeof cloudWindow?.useCapacity === "function") {
    candidates.push(cloudWindow);
  }
  if (typeof window === "undefined") {
    return candidates;
  }

  for (const target of [window, window.parent, window.top]) {
    if (!target) {
      continue;
    }
    try {
      if (typeof target.cloudWindow?.useCapacity === "function") {
        candidates.push(target.cloudWindow);
      }
    } catch (_) {
      // Ignore cross-origin frame access errors.
    }
  }

  return candidates.filter(Boolean);
}

function getLegacySdkCandidates() {
  const candidates = [];
  if (typeof window === "undefined") {
    return candidates;
  }

  if (window.ugSdk) {
    candidates.push(window.ugSdk);
  }

  for (const target of [window.parent, window.top]) {
    if (!target || target === window) {
      continue;
    }
    try {
      if (target.ugSdk) {
        candidates.push(target.ugSdk);
      }
    } catch (_) {
      // Ignore cross-origin frame access errors.
    }
  }

  return candidates.filter(Boolean);
}

async function resolveThirdTokenFromCapacity() {
  await ensureCloudWindowReady();
  for (const candidate of getCloudWindowCandidates()) {
    try {
      const info = await candidate.useCapacity("getThirdToken", undefined, 1000);
      if (info?.third_token) {
        return String(info.third_token);
      }
    } catch (_) {
    }
  }
  return null;
}

async function resolveThirdTokenFromLegacySDK() {
  const sdk = getLegacySdkCandidates().find((candidate) => typeof candidate?.getUgInfo === "function");
  if (!sdk) {
    return null;
  }
  try {
    return await new Promise((resolve) => {
      let settled = false;

      const finish = (value) => {
        if (!settled) {
          settled = true;
          resolve(value || null);
        }
      };

      sdk.getUgInfo((error, info) => {
        if (error) {
          finish(null);
          return;
        }
        finish(info && info.third_token ? String(info.third_token) : null);
      });

      window.setTimeout(() => finish(null), 1200);
    });
  } catch (_) {
    return null;
  }
}

async function resolveUgToken() {
  if (ugToken) {
    return ugToken;
  }

  const token = (await resolveThirdTokenFromCapacity()) || (await resolveThirdTokenFromLegacySDK());
  if (token) {
    ugToken = token;
  }
  return ugToken;
}

function apiBaseCandidates() {
  const values = [];
  const seen = new Set();

  addApiCandidate(values, seen, API_BASE);
  addApiCandidate(values, seen, relativePathFromBase(document.baseURI, "api"));
  addApiCandidate(values, seen, relativePathFromBase(window.location.href, "./api"));
  for (const prefix of appRoutePrefixesFromPath(window.location.pathname || "/")) {
    addApiCandidate(values, seen, prefix);
  }
  for (const base of CANONICAL_API_BASE_CANDIDATES) {
    addApiCandidate(values, seen, base);
  }
  for (const base of LEGACY_API_BASE_CANDIDATES) {
    addApiCandidate(values, seen, base);
  }
  addApiCandidate(values, seen, HOST_API_BASE);

  return values;
}

async function fetchWithTimeout(url, options = {}, timeoutMs = 12000) {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);

  try {
    return await fetch(url, {
      ...options,
      signal: controller.signal
    });
  } catch (error) {
    if (error?.name === "AbortError") {
      throw new Error(`请求超时（${timeoutMs}ms）`);
    }
    throw error;
  } finally {
    window.clearTimeout(timer);
  }
}

function looksLikeHtmlResponse(contentType, text) {
  return /html/i.test(contentType) || /^\s*<(?:!doctype|html|head|body)\b/i.test(text || "");
}

function retryableApiError(message, response, url) {
  const error = new Error(message);
  error.status = response?.status || 0;
  error.url = url;
  error.retryable = true;
  return error;
}

function isBootstrapResponse(data) {
  return Boolean(
    data &&
      typeof data === "object" &&
      ("initialized" in data || "authenticated" in data || "version" in data || "setup_token" in data)
  );
}

async function api(path, options = {}) {
  let lastError = null;
  const candidates = apiBaseCandidates();

  for (const base of candidates) {
    const headers = {
      "Content-Type": "application/json",
      ...(options.headers || {})
    };

    if (ugToken) {
      headers["Ugreen-Ttk"] = ugToken;
    }

    try {
      const url = `${base}${path}`;
      const response = await fetchWithTimeout(url, {
        credentials: "same-origin",
        headers,
        ...options
      });

      let data = null;
      const contentType = response.headers.get("content-type") || "";
      const text = await response.text();
      if (looksLikeHtmlResponse(contentType, text)) {
        throw retryableApiError(`API 地址 ${url} 返回了 HTML 页面，不是 JSON`, response, url);
      }

      if (text.trim()) {
        try {
          data = JSON.parse(text);
        } catch (error) {
          const parseError = retryableApiError(`响应 JSON 解析失败：${error.message}（${url}）`, response, url);
          parseError.retryable = !contentType.includes("application/json") || response.status === 404;
          throw parseError;
        }
      }

      if (path === "/bootstrap" && response.ok && !isBootstrapResponse(data)) {
        throw retryableApiError(`API 地址 ${url} 返回的不是 NasNotify 初始化数据`, response, url);
      }

      if (!response.ok) {
        const error = new Error((data && (data.error || data.message)) || "请求失败");
        error.status = response.status;
        if (response.status === 404 || (path === "/bootstrap" && (response.status === 401 || response.status === 403))) {
          error.retryable = true;
        }
        throw error;
      }

      API_BASE = base;
      return data;
    } catch (error) {
      lastError = error;
      if (!error.retryable && error.status && error.status !== 404) {
        throw error;
      }
    }
  }

  const attempted = candidates.map((base) => `${base}${path}`).join(", ");
  if (lastError) {
    throw new Error(`${lastError.message}。已尝试：${attempted}`);
  }
  throw new Error(`无法连接 NasNotify 服务。已尝试：${attempted}`);
}

function currentView() {
  if (!state.bootstrap) {
    return "loading";
  }
  if (!state.bootstrap.initialized) {
    return "setup";
  }
  if (!state.bootstrap.authenticated) {
    return "login";
  }
  return "dashboard";
}

async function loadBootstrap() {
  await resolveUgToken();
  state.bootstrap = await api("/bootstrap", { method: "GET" });
}

async function loadGatewayStatus() {
  if (!state.bootstrap?.authenticated) {
    state.gatewayStatus = null;
    return;
  }

  try {
    state.gatewayStatus = await api("/wechat/status", { method: "GET" });
  } catch (error) {
    state.gatewayStatus = {
      configured: false,
      open_api_ready: false,
      entry_bound: false,
      bound: false,
      activated: false,
      binding_code: "",
      tips: [],
      last_error: error.message
    };
  }
}

function statusLabel(active, onText, offText) {
  return active ? onText : offText;
}

function sidePillMarkup(item) {
  const label = typeof item === "string" ? item : item?.label;
  const tone = typeof item === "string" ? "" : item?.tone || "";
  return `<span class="${tone ? `tone-${escapeHtml(tone)}` : ""}">${escapeHtml(label || "")}</span>`;
}

function shellTemplate({
  title,
  subtitle,
  sideTitle,
  sideText,
  body,
  actionBar = "",
  sidePills = [],
  sideList = [],
  viewClass = ""
}) {
  const version = state.bootstrap?.version || "未初始化";
  const heroPills = sidePills.length
    ? `<div class="hero-quick">${sidePills.map((item) => sidePillMarkup(item)).join("")}</div>`
    : "";
  const heroList = sideList.length
    ? `
      <div class="hero-section">
        <div class="hero-section-title">当前能力</div>
        <ul class="hero-list">
          ${sideList.map((item) => `<li>${escapeHtml(item)}</li>`).join("")}
        </ul>
      </div>
    `
    : "";
  return `
    <div class="window-shell ${escapeHtml(viewClass)}">
      <div class="window-toolbar">
        <div class="window-toolbar-drag">
          <div class="window-toolbar-copy">
            <span class="window-app-name">NasNotify</span>
            <span class="window-app-meta">${isPreviewMode() ? "布局预览" : "UGREEN NAS 内嵌应用"}</span>
          </div>
          <div class="window-toolbar-note">本地通知控制台</div>
        </div>
        <div class="window-toolbar-actions" aria-hidden="true"></div>
      </div>
      <div class="shell ${escapeHtml(viewClass)}">
        <aside class="hero">
          <div class="hero-topline">
            <div class="hero-badge">UGREEN NAS</div>
            <div class="hero-state">${isPreviewMode() ? "预览模式" : "桌面控制台"}</div>
          </div>
          <h1>${escapeHtml(sideTitle)}</h1>
          <p>${escapeHtml(sideText)}</p>
          ${heroPills}
          ${heroList}
          <div class="hero-footer">
            <div class="hero-version-label">当前版本</div>
            <div class="hero-version">${escapeHtml(version)}</div>
          </div>
        </aside>
        <main class="panel">
          <header class="panel-head">
            <div>
              <div class="eyebrow">NasNotify</div>
              <h2>${escapeHtml(title)}</h2>
              <p>${escapeHtml(subtitle)}</p>
            </div>
            ${actionBar}
          </header>
          <section class="panel-body">${body}</section>
        </main>
      </div>
    </div>
  `;
}

async function bindWindowChrome() {
  const toolbar = document.querySelector(".window-toolbar");
  const dragRegion = document.querySelector(".window-toolbar-drag");
  if (!toolbar) {
    return;
  }

  try {
    await ensureCloudWindowReady();
    if (typeof cloudWindow?.setMovable === "function") {
      await cloudWindow.setMovable(true);
    }
    if (typeof cloudWindow?.registerHeader === "function") {
      // Re-rendering replaces the toolbar DOM node, so the host drag region must be rebound.
      await cloudWindow.registerHeader(dragRegion || toolbar, true);
    }
    if (typeof cloudWindow?.setActionPosition === "function") {
      // SDK signature is `(y, x)`, and the desktop host keeps the controls right-aligned.
      await cloudWindow.setActionPosition(WINDOW_ACTION_OFFSET_Y, WINDOW_ACTION_OFFSET_X);
    }
  } catch (_) {
  }
}

function renderLoading() {
  appRoot().innerHTML = `
    <div class="loading-screen">
      <div class="loading-card">
        <div class="spinner"></div>
        <h1>NasNotify</h1>
        <p>正在连接 NAS 应用服务，请稍候。</p>
      </div>
    </div>
  `;
}

function renderError(message) {
  appRoot().innerHTML = `
    <div class="loading-screen">
      <div class="loading-card error-card">
        <h1>应用加载失败</h1>
        <p>${escapeHtml(message)}</p>
        <button class="primary-btn" id="retryBtn">重新加载</button>
      </div>
    </div>
  `;
  document.getElementById("retryBtn").addEventListener("click", bootstrapApp);
}

function renderNotice(id) {
  return `<div id="${id}" class="notice error hidden"></div>`;
}

function dashboardOverviewMarkup(config) {
  const status = state.gatewayStatus || {};
  const cards = [
    {
      label: "通知轮询",
      value: `${config.interval_minutes || 5} 分钟`,
      meta: "系统通知抓取节奏"
    },
    {
      label: "状态推送",
      value: `${config.system_status_interval_minutes || 60} 分钟`,
      meta: "主动推送系统概览"
    },
    {
      label: "微信网关",
      value: statusLabel(status.open_api_ready, "在线", "离线"),
      meta: statusLabel(status.bound, "绑定已完成", "等待绑定"),
      tone: status.open_api_ready ? "ok" : "warn"
    },
    {
      label: "本机 NAS",
      value: config.local_nas_name || "本机绿联 NAS",
      meta: `${config.local_nas_username ? "账号已配置" : "待配置账号"} · ${config.local_nas_host || "127.0.0.1"}:${config.local_nas_port || 9999}`
    }
  ];

  return `
    <section class="overview-grid">
      ${cards
        .map(
          (card) => `
            <div class="stat-card ${card.tone ? `tone-${card.tone}` : ""}">
              <div class="stat-label">${escapeHtml(card.label)}</div>
              <div class="stat-value">${escapeHtml(card.value)}</div>
              <div class="stat-meta">${escapeHtml(card.meta)}</div>
            </div>
          `
        )
        .join("")}
    </section>
  `;
}

function dashboardTabButton(id, label, meta, active) {
  return `
    <button type="button" class="dashboard-tab ${active ? "active" : ""}" data-dashboard-tab="${escapeHtml(id)}" aria-selected="${active ? "true" : "false"}">
      <span>${escapeHtml(label)}</span>
      <small>${escapeHtml(meta)}</small>
    </button>
  `;
}

function dashboardTabPage(id, content, active, extraClass = "") {
  return `
    <section class="dashboard-page ${extraClass} ${active ? "active" : ""}" data-tab-page="${escapeHtml(id)}">
      ${content}
    </section>
  `;
}

function commandReferenceMarkup() {
  return `
    <section class="section-card reference-card command-reference-wide">
      <div class="section-head">
        <h3>常用指令</h3>
        <span>微信入口</span>
      </div>
      <div class="command-board">
        <div class="command-group">
          <div class="command-group-title">查询类</div>
          <div class="command-strip">
            <span>菜单</span><span>状态</span><span>通知</span><span>存储</span><span>Docker</span><span>进程</span>
          </div>
        </div>
        <div class="command-group">
          <div class="command-group-title">运维类</div>
          <div class="command-strip">
            <span>备份</span><span>电源</span><span>UPS</span><span>测试</span>
          </div>
        </div>
        <div class="command-group">
          <div class="command-group-title">控制类</div>
          <div class="command-strip">
            <span>风扇1</span><span>风扇2</span><span>风扇3</span><span>CPU0</span><span>CPU1</span><span>CPU2</span>
          </div>
        </div>
      </div>
    </section>
  `;
}

function setupGuideMarkup() {
  return `
    <section class="section-card helper-card">
      <div class="section-head">
        <h3>部署顺序</h3>
        <span>一次完成</span>
      </div>
      <div class="helper-list">
        <div class="helper-item"><strong>1.</strong><span>设置管理员密码，完成控制台初始化。</span></div>
        <div class="helper-item"><strong>2.</strong><span>填入本机 NAS 管理账号，用于读取状态与执行固定控制命令。</span></div>
        <div class="helper-item"><strong>3.</strong><span>企业微信和 ClawBot 可同时保留，图片卡片会优先发送到可用通道。</span></div>
      </div>
    </section>
  `;
}

function baseConfigForm(config, includeAdminPassword, layoutClass = "single-desktop") {
  return `
    <section class="section-card gateway-card">
      <div class="section-head">
        <h3>基础设置</h3>
        <span>NAS / 轮询</span>
      </div>
      <div class="grid two ${escapeHtml(layoutClass)}">
        ${includeAdminPassword ? `
          <label class="field field-wide">
            <span>新管理员密码</span>
            <input type="password" id="new_admin_password" placeholder="留空表示不修改">
          </label>
        ` : ""}
        <label class="field">
          <span>通知轮询间隔（分钟）</span>
          <input type="number" id="interval_minutes" min="0.1" step="0.1" value="${escapeHtml(config.interval_minutes || 5)}">
        </label>
        <label class="field">
          <span>系统状态推送间隔（分钟）</span>
          <input type="number" id="system_status_interval_minutes" min="1" step="1" value="${escapeHtml(config.system_status_interval_minutes || 60)}">
        </label>
        <label class="field">
          <span>本机 NAS 显示名称</span>
          <input type="text" id="local_nas_name" value="${escapeHtml(config.local_nas_name || "本机绿联 NAS")}" placeholder="例如：客厅 NAS">
        </label>
        <label class="field">
          <span>NAS 地址 / IP</span>
          <input type="text" id="local_nas_host" value="${escapeHtml(config.local_nas_host || "127.0.0.1")}" placeholder="例如：192.168.1.9 或 nas.local">
        </label>
        <label class="field">
          <span>本机 NAS 端口</span>
          <input type="number" id="local_nas_port" min="1" step="1" value="${escapeHtml(config.local_nas_port || 9999)}" placeholder="默认 9999">
        </label>
        <label class="field">
          <span>本机 NAS 管理账号</span>
          <input type="text" id="local_nas_username" value="${escapeHtml(config.local_nas_username || "")}" placeholder="用于读取系统状态">
        </label>
        <label class="field">
          <span>本机 NAS 管理密码</span>
          <input type="password" id="local_nas_password" value="${escapeHtml(config.local_nas_password || "")}" placeholder="留空表示保持现有值">
        </label>
      </div>
    </section>
  `;
}

function enterpriseWechatForm(config, layoutClass = "two-desktop") {
  return `
    <section class="section-card gateway-card enterprise-card">
      <div class="section-head">
        <h3>企业微信配置</h3>
        <span>保留原通道</span>
      </div>
      <div class="grid two ${escapeHtml(layoutClass)}">
        <label class="field">
          <span>CorpID</span>
          <input type="text" id="corpid" value="${escapeHtml(config.corpid || "")}" placeholder="企业 ID">
        </label>
        <label class="field">
          <span>AgentID</span>
          <input type="text" id="agentid" value="${escapeHtml(config.agentid || "")}" placeholder="应用 AgentID">
        </label>
        <label class="field field-wide">
          <span>CorpSecret</span>
          <input type="password" id="corpsecret" value="${escapeHtml(config.corpsecret || "")}" placeholder="留空表示保持现有值">
        </label>
        <label class="field">
          <span>回调 Token</span>
          <input type="password" id="token" value="${escapeHtml(config.token || "")}" placeholder="留空表示保持现有值">
        </label>
        <label class="field">
          <span>EncodingAESKey</span>
          <input type="password" id="encoding_aes_key" value="${escapeHtml(config.encoding_aes_key || "")}" placeholder="留空表示保持现有值">
        </label>
        <label class="field field-wide">
          <span>NAS 跳转地址</span>
          <input type="text" id="nas_url" value="${escapeHtml(config.nas_url || "")}" placeholder="例如：https://nas.example.com">
        </label>
        <label class="field">
          <span>图文封面 API</span>
          <input type="text" id="photo_url" value="${escapeHtml(config.photo_url || "")}" placeholder="可选，文本回退时使用">
        </label>
        <label class="field">
          <span>企业微信代理地址</span>
          <input type="text" id="proxy_url" value="${escapeHtml(config.proxy_url || "")}" placeholder="可选，默认官方 API">
        </label>
      </div>
    </section>
  `;
}

function gatewayStatusMarkup() {
  const status = state.gatewayStatus || {};
  const qr = status.qrcode || null;
  const needVerifyCode = Boolean(status.need_verify_code);
  const hasQRCode = Boolean(qr?.url || qr?.qrcode);
  const loginButtonText = hasQRCode ? "重新生成二维码" : "生成二维码";
  const tips = Array.isArray(status.tips) ? status.tips : [];
  const visibleTips = tips.slice(0, 1);
  const qrMarkup = qr?.url
    ? `<img class="qr-image" src="${escapeHtml(wechatQRCodeImageSrc())}" alt="QR code" referrerpolicy="no-referrer">`
    : qr?.qrcode
      ? `<div class="qr-fallback">${escapeHtml(qr.qrcode)}</div>`
      : `<div class="qr-placeholder">保存配置后生成微信登录二维码。</div>`;

  const bindState = status.bound ? "已完成绑定" : "等待发送绑定码";

  return `
    <section class="section-card gateway-binding-card">
      <div class="section-head">
        <h3>微信网关绑定</h3>
        <span>${escapeHtml(bindState)}</span>
      </div>
      <div class="grid two gateway-config-grid">
        <label class="field">
          <span>本地微信网关地址</span>
          <input type="text" id="wechat_gateway_url" value="${escapeHtml(state.bootstrap?.config?.wechat_gateway_url || "http://127.0.0.1:5091")}" placeholder="例如：http://127.0.0.1:5091">
        </label>
        <label class="field">
          <span>网关共享密钥</span>
          <input type="password" id="wechat_gateway_secret" value="${escapeHtml(state.bootstrap?.config?.wechat_gateway_secret || "")}" placeholder="可选，用于保护本地网关接口">
        </label>
      </div>
      <div class="binding-layout">
        <div class="qr-stack">
          <div class="qr-box">${qrMarkup}</div>
          <button type="button" class="primary-btn gateway-login-btn" id="startGatewayLoginBtn">${loginButtonText}</button>
          <button type="button" class="ghost-btn test-push-btn">发送测试通知</button>
        </div>
        <div class="binding-panel">
          <div class="status-pills">
            <span class="pill ${status.configured ? "ok" : ""}">配置${status.configured ? "已完成" : "未完成"}</span>
            <span class="pill ${status.open_api_ready ? "ok" : ""}">网关${status.open_api_ready ? "在线" : "离线"}</span>
            <span class="pill ${status.entry_bound ? "ok" : ""}">登录${status.entry_bound ? "已进入" : "未进入"}</span>
            <span class="pill ${status.bound ? "ok" : ""}">绑定${status.bound ? "已完成" : "待匹配"}</span>
          </div>
          ${status.last_error ? `<div class="notice warm">${escapeHtml(status.last_error)}</div>` : ""}
          <div class="binding-code-card">
            <div class="binding-code-label">当前绑定码</div>
            <div class="binding-code-value" id="bindingCodeValue">${escapeHtml(status.binding_code || "------")}</div>
            <button type="button" class="ghost-btn" id="copyBindingCodeBtn">复制绑定码</button>
          </div>
          <div class="steps-card gateway-command-card">
            <div class="step-grid">
              <div class="step-item"><strong>扫码登录：</strong>生成二维码，用微信扫描并完成登录。</div>
              <div class="step-item"><strong>绑定入口：</strong>先发任意消息激活会话，再发送当前绑定码。</div>
            </div>
            <div class="command-strip">
              <span>菜单</span><span>状态</span><span>通知</span><span>存储</span><span>Docker</span><span>进程</span><span>备份</span><span>电源</span><span>UPS</span><span>风扇2</span><span>CPU1</span>
            </div>
          </div>
          ${needVerifyCode ? `
            <div class="verify-card">
              <label class="field">
                <span>手机微信数字验证码</span>
                <input type="text" id="wechat_verify_code" placeholder="输入扫码后微信里显示的数字">
              </label>
              <button type="button" class="ghost-btn" id="submitVerifyCodeBtn">提交验证码</button>
            </div>
          ` : ""}
          ${status.entry_bind_time ? `<div class="meta-line">微信登录时间：${escapeHtml(status.entry_bind_time)}</div>` : ""}
          ${status.bind_time ? `<div class="meta-line">NasNotify 绑定时间：${escapeHtml(status.bind_time)}</div>` : ""}
          ${visibleTips.length ? `
            <div class="tips-list">
              ${visibleTips.map((tip) => `<div class="tip-item">${escapeHtml(tip)}</div>`).join("")}
            </div>
          ` : ""}
        </div>
      </div>
      <div class="inline-actions gateway-secondary-actions">
        <button type="button" class="ghost-btn" id="refreshGatewayBtn">刷新绑定状态</button>
        <button type="button" class="ghost-btn danger-btn" id="unbindGatewayBtn">解绑并重置绑定码</button>
      </div>
    </section>
  `;
}

function setupBody(config) {
  return `
    <form id="setupForm" class="form-stack">
      ${state.flash ? `<div class="notice success">${escapeHtml(state.flash)}</div>` : ""}
      ${renderNotice("setupError")}
      <div class="setup-layout">
        <div class="setup-main">
          <section class="section-card">
            <div class="section-head">
              <h3>管理员初始化</h3>
              <span>必填</span>
            </div>
            <div class="grid two">
              <label class="field field-wide">
                <span>初始化密钥</span>
                <input type="text" id="init_token" value="${escapeHtml(bootstrapSetupToken())}" placeholder="后端生成的初始化密钥" readonly>
              </label>
              <label class="field">
                <span>管理员密码</span>
                <input type="password" id="admin_password" placeholder="至少 8 位">
              </label>
              <label class="field">
                <span>确认管理员密码</span>
                <input type="password" id="admin_password_confirm" placeholder="再次输入密码">
              </label>
            </div>
          </section>
          ${baseConfigForm(config, false, "two-desktop")}
        </div>
        <aside class="setup-side">
          ${enterpriseWechatForm(config, "single-desktop")}
          <section class="section-card">
            <div class="section-head">
              <h3>微信网关配置</h3>
              <span>默认本机</span>
            </div>
            <div class="grid two single-desktop">
              <label class="field">
                <span>本地微信网关地址</span>
                <input type="text" id="wechat_gateway_url" value="${escapeHtml(config.wechat_gateway_url || "http://127.0.0.1:5091")}" placeholder="例如：http://127.0.0.1:5091">
              </label>
              <label class="field">
                <span>网关共享密钥</span>
                <input type="password" id="wechat_gateway_secret" value="${escapeHtml(config.wechat_gateway_secret || "")}" placeholder="可选，用于保护本地网关接口">
              </label>
            </div>
          </section>
          ${setupGuideMarkup()}
        </aside>
      </div>
      <div class="setup-submit-bar">
        <div class="setup-submit-copy">
          <span>准备完成</span>
          <strong>保存配置并进入控制台登录</strong>
        </div>
        <button type="submit" class="primary-btn init-submit-btn">
          <span>完成初始化</span>
          <i aria-hidden="true"></i>
        </button>
      </div>
    </form>
  `;
}

function loginBody() {
  return `
    <form id="loginForm" class="form-stack compact">
      ${state.flash ? `<div class="notice success">${escapeHtml(state.flash)}</div>` : ""}
      ${renderNotice("loginError")}
      <section class="section-card">
        <div class="section-head">
          <h3>进入控制台</h3>
          <span>认证</span>
        </div>
        <label class="field">
          <span>管理员密码</span>
          <input type="password" id="login_password" placeholder="请输入管理员密码">
        </label>
      </section>
      <div class="footer-actions">
        <button type="submit" class="primary-btn">登录</button>
      </div>
    </form>
  `;
}

function dashboardBody(config) {
  const tabs = [
    { id: "overview", label: "运行总览", meta: "状态速览" },
    { id: "settings", label: "基础设置", meta: "NAS / 轮询" },
    { id: "enterprise", label: "企业微信", meta: "回调 / 菜单" },
    { id: "gateway", label: "微信绑定", meta: "扫码 / 绑定码" },
    { id: "commands", label: "指令操作", meta: "测试 / 参考" }
  ];
  const tabIds = new Set(tabs.map((tab) => tab.id));
  const activeTab = tabIds.has(state.dashboardTab) ? state.dashboardTab : "settings";
  state.dashboardTab = activeTab;
  const settingsPage = `
    <div class="settings-page-grid">
      <div class="settings-primary">
        ${baseConfigForm(config, true, "two-desktop")}
      </div>
      <aside class="settings-summary section-card">
        <div class="section-head">
          <h3>保存策略</h3>
          <span>即时生效</span>
        </div>
        <div class="summary-stack">
          <div class="summary-line">
            <span>轮询节奏</span>
            <strong>${escapeHtml(config.interval_minutes || 5)} 分钟</strong>
          </div>
          <div class="summary-line">
            <span>状态推送</span>
            <strong>${escapeHtml(config.system_status_interval_minutes || 60)} 分钟</strong>
          </div>
          <div class="summary-line">
            <span>NAS 地址</span>
            <strong>${escapeHtml(config.local_nas_host || "127.0.0.1")}</strong>
          </div>
          <div class="summary-line">
            <span>NAS 端口</span>
            <strong>${escapeHtml(config.local_nas_port || 9999)}</strong>
          </div>
        </div>
        <div class="settings-action-copy">修改后点击保存，后端会立即使用新的轮询、账号和网关参数。</div>
        <button type="submit" class="primary-btn save-config-btn">保存并应用</button>
      </aside>
    </div>
  `;
  const enterprisePage = `
    <div class="settings-page-grid">
      <div class="settings-primary">
        ${enterpriseWechatForm(config, "two-desktop")}
      </div>
      <aside class="settings-summary section-card">
        <div class="section-head">
          <h3>企业微信状态</h3>
          <span>兼容保留</span>
        </div>
        <div class="summary-stack">
          <div class="summary-line">
            <span>CorpID</span>
            <strong>${escapeHtml(config.corpid ? "已填写" : "未填写")}</strong>
          </div>
          <div class="summary-line">
            <span>AgentID</span>
            <strong>${escapeHtml(config.agentid || "未填写")}</strong>
          </div>
          <div class="summary-line">
            <span>回调入口</span>
            <strong>/wx-receive</strong>
          </div>
        </div>
        <div class="settings-action-copy">保存后会自动同步企业微信菜单。图片卡片优先走图片消息，失败时回退到图文文本。</div>
        <button type="submit" class="primary-btn save-config-btn">保存并应用</button>
      </aside>
    </div>
  `;
  const overviewPage = `
    ${dashboardOverviewMarkup(config)}
    <div class="overview-stage">
      <section class="section-card overview-panel">
        <div class="section-head">
          <h3>当前运行状态</h3>
          <span>桌面控制台</span>
        </div>
        <div class="overview-rhythm">
          <div>
            <span>通知轮询</span>
            <strong>${escapeHtml(config.interval_minutes || 5)} 分钟一次</strong>
          </div>
          <div>
            <span>系统概览</span>
            <strong>${escapeHtml(config.system_status_interval_minutes || 60)} 分钟一次</strong>
          </div>
          <div>
            <span>微信入口</span>
            <strong>${statusLabel(state.gatewayStatus?.bound, "已绑定", "待绑定")}</strong>
          </div>
        </div>
      </section>
      <section class="section-card action-card">
        <div class="section-head">
          <h3>快速操作</h3>
          <span>验证链路</span>
        </div>
        <div class="action-stack">
          <button type="button" class="ghost-btn test-push-btn">发送测试通知</button>
          <button type="submit" class="primary-btn">保存当前配置</button>
        </div>
      </section>
    </div>
  `;
  const commandsPage = `
    <div class="commands-page-grid">
      ${commandReferenceMarkup()}
      <section class="section-card action-card">
        <div class="section-head">
          <h3>链路测试</h3>
          <span>快速执行</span>
        </div>
        <div class="action-stack">
          <button type="button" class="ghost-btn test-push-btn">发送测试通知</button>
          <div class="action-hint">配置保存已集中在基础设置页，这里只保留指令参考和通知测试。</div>
        </div>
      </section>
    </div>
  `;

  return `
    <form id="dashboardForm" class="form-stack dashboard-console" data-active-tab="${escapeHtml(activeTab)}">
      ${state.flash ? `<div class="notice success">${escapeHtml(state.flash)}</div>` : ""}
      ${renderNotice("dashboardError")}
      <nav class="dashboard-tabs" aria-label="控制台页面">
        ${tabs.map((tab) => dashboardTabButton(tab.id, tab.label, tab.meta, tab.id === activeTab)).join("")}
      </nav>
      <div class="dashboard-pages">
        ${dashboardTabPage("overview", overviewPage, activeTab === "overview", "dashboard-page-overview")}
        ${dashboardTabPage("settings", settingsPage, activeTab === "settings", "dashboard-page-settings")}
        ${dashboardTabPage("enterprise", enterprisePage, activeTab === "enterprise", "dashboard-page-settings")}
        ${dashboardTabPage("gateway", gatewayStatusMarkup(), activeTab === "gateway", "dashboard-page-gateway")}
        ${dashboardTabPage("commands", commandsPage, activeTab === "commands", "dashboard-page-commands")}
      </div>
    </form>
  `;
}

function renderApp() {
  const view = currentView();
  const config = state.bootstrap?.config || {};

  if (view === "setup") {
    appRoot().innerHTML = shellTemplate({
      title: "初始化 NasNotify",
      subtitle: "完成初始化后即可直接在 NAS 页面内管理通知与微信绑定。",
      sideTitle: "单机通知应用",
      sideText: "面向当前这台绿联 NAS，使用本地微信网关收发通知，并通过 NAS 内嵌路由访问。",
      sidePills: ["本机 NAS", "微信网关", "首次部署"],
      sideList: ["初始化完成后即可登录控制台。", "微信入口绑定完成后可直接发送固定指令。", "所有请求都走 NAS 路由代理。"],
      body: setupBody(config),
      viewClass: "view-setup"
    });
    bindSetup();
    bindWindowChrome();
    return;
  }

  if (view === "login") {
    appRoot().innerHTML = shellTemplate({
      title: "登录控制台",
      subtitle: "输入管理员密码后继续管理通知、绑定和查询指令。",
      sideTitle: "微信网关版",
      sideText: "登录后即可查看 NAS 状态、微信绑定情况和当前轮询配置。",
      sidePills: ["控制台登录", "配置管理"],
      sideList: ["进入后可调整轮询周期、NAS 管理账号和微信网关参数。", "常用命令与绑定状态都会集中展示。"],
      body: loginBody(),
      viewClass: "view-login"
    });
    bindLogin();
    bindWindowChrome();
    return;
  }

  appRoot().innerHTML = shellTemplate({
    title: "本机绿联 NAS 管理",
    subtitle: "在一个窗口内完成轮询、账号、微信绑定和常用指令管理。",
    sideTitle: "固定指令机器人",
    sideText: "绑定后接收系统通知，并响应状态、存储、风扇和 CPU 等固定指令。",
    sidePills: [
      {
        label: statusLabel(state.gatewayStatus?.open_api_ready, "网关在线", "网关离线"),
        tone: state.gatewayStatus?.open_api_ready ? "ok" : "danger"
      },
      {
        label: statusLabel(state.gatewayStatus?.bound, "绑定完成", "等待绑定"),
        tone: state.gatewayStatus?.bound ? "ok" : "warn"
      },
      { label: "桌面控制台", tone: "info" }
    ],
    sideList: [
      "左侧概览聚合轮询节奏、网关状态和本机 NAS 配置。",
      "右侧聚焦微信绑定、二维码和绑定码。",
      "底部保留完整命令参考与快捷操作。"
    ],
    actionBar: `<button type="button" class="ghost-btn" id="logoutBtn">退出登录</button>`,
    body: dashboardBody(config),
    viewClass: "view-dashboard"
  });
  bindDashboard();
  bindDashboardTabs();
  bindGatewayActions("dashboardError");
  document.getElementById("logoutBtn").addEventListener("click", handleLogout);
  bindWindowChrome();
}

function showFormError(id, message) {
  const box = document.getElementById(id);
  if (!box) {
    return;
  }
  box.textContent = message;
  box.classList.remove("hidden");
  box.scrollIntoView({ behavior: "smooth", block: "center" });
}

function clearFormError(id) {
  const box = document.getElementById(id);
  if (!box) {
    return;
  }
  box.textContent = "";
  box.classList.add("hidden");
}

function inputValue(id) {
  return document.getElementById(id)?.value || "";
}

function numberValue(id, fallback) {
  return Number(inputValue(id)) || fallback;
}

function collectConfig() {
  return {
    interval_minutes: numberValue("interval_minutes", 5),
    system_status_interval_minutes: numberValue("system_status_interval_minutes", 60),
    local_nas_name: inputValue("local_nas_name").trim(),
    local_nas_host: inputValue("local_nas_host").trim(),
    local_nas_port: numberValue("local_nas_port", 9999),
    local_nas_username: inputValue("local_nas_username").trim(),
    local_nas_password: inputValue("local_nas_password"),
    corpid: inputValue("corpid").trim(),
    agentid: inputValue("agentid").trim(),
    corpsecret: inputValue("corpsecret"),
    token: inputValue("token"),
    encoding_aes_key: inputValue("encoding_aes_key"),
    nas_url: inputValue("nas_url").trim(),
    photo_url: inputValue("photo_url").trim(),
    proxy_url: inputValue("proxy_url").trim(),
    wechat_gateway_url: inputValue("wechat_gateway_url").trim(),
    wechat_gateway_secret: inputValue("wechat_gateway_secret").trim()
  };
}

async function copyText(text) {
  if (!text) {
    return;
  }
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const input = document.createElement("textarea");
  input.value = text;
  document.body.appendChild(input);
  input.select();
  document.execCommand("copy");
  document.body.removeChild(input);
}

function bindGatewayActions(errorId) {
  const startBtn = document.getElementById("startGatewayLoginBtn");
  const refreshBtn = document.getElementById("refreshGatewayBtn");
  const unbindBtn = document.getElementById("unbindGatewayBtn");
  const copyBtn = document.getElementById("copyBindingCodeBtn");
  const verifyBtn = document.getElementById("submitVerifyCodeBtn");

  if (copyBtn) {
    copyBtn.addEventListener("click", async () => {
      try {
        await copyText(state.gatewayStatus?.binding_code || "");
        copyBtn.textContent = "已复制";
        copyBtn.classList.add("copied");
        window.setTimeout(() => {
          copyBtn.textContent = "复制绑定码";
          copyBtn.classList.remove("copied");
        }, 1600);
      } catch (error) {
        showFormError(errorId, error.message || "复制失败");
      }
    });
  }

  if (startBtn) {
    startBtn.addEventListener("click", async () => {
      clearFormError(errorId);
      try {
        await api("/wechat/login/start", { method: "POST", body: "{}" });
        state.flash = "新的二维码已生成，请使用微信扫码。";
        await loadGatewayStatus();
        renderApp();
      } catch (error) {
        showFormError(errorId, error.message);
      }
    });
  }

  if (refreshBtn) {
    refreshBtn.addEventListener("click", async () => {
      clearFormError(errorId);
      try {
        await loadGatewayStatus();
        state.flash = "绑定状态已刷新。";
        renderApp();
      } catch (error) {
        showFormError(errorId, error.message);
      }
    });
  }

  if (unbindBtn) {
    unbindBtn.addEventListener("click", async () => {
      clearFormError(errorId);
      try {
        await api("/wechat/unbind", { method: "POST", body: "{}" });
        state.flash = "微信入口已解绑，并已生成新的绑定码。";
        await loadGatewayStatus();
        renderApp();
      } catch (error) {
        showFormError(errorId, error.message);
      }
    });
  }

  if (verifyBtn) {
    verifyBtn.addEventListener("click", async () => {
      clearFormError(errorId);
      try {
        const code = document.getElementById("wechat_verify_code")?.value?.trim() || "";
        await api("/wechat/login/verify-code", {
          method: "POST",
          body: JSON.stringify({ verify_code: code })
        });
        state.flash = "验证码已提交，请稍候刷新状态。";
        await loadGatewayStatus();
        renderApp();
      } catch (error) {
        showFormError(errorId, error.message);
      }
    });
  }
}

function bindSetup() {
  document.getElementById("setupForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    clearFormError("setupError");

    const initToken = document.getElementById("init_token").value.trim();
    const password = document.getElementById("admin_password").value;
    const confirmPassword = document.getElementById("admin_password_confirm").value;
    const config = collectConfig();

    if (!initToken) {
      showFormError("setupError", "请填写初始化密钥。");
      return;
    }
    if (password.length < 8) {
      showFormError("setupError", "管理员密码至少 8 位。");
      return;
    }
    if (password !== confirmPassword) {
      showFormError("setupError", "两次输入的管理员密码不一致。");
      return;
    }
    if (!config.local_nas_username) {
      showFormError("setupError", "请填写本机 NAS 管理账号。");
      return;
    }
    if (!config.local_nas_password) {
      showFormError("setupError", "首次初始化请填写本机 NAS 管理密码。");
      return;
    }
    if (!config.wechat_gateway_url) {
      showFormError("setupError", "请填写本地微信网关地址。");
      return;
    }

    try {
      await api("/setup", {
        method: "POST",
        body: JSON.stringify({
          init_token: initToken,
          admin_password: password,
          config
        })
      });
      state.flash = "初始化完成。登录后即可扫码并使用绑定码完成微信入口绑定。";
      await bootstrapApp();
    } catch (error) {
      showFormError("setupError", error.message);
    }
  });
}

function bindLogin() {
  document.getElementById("loginForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    clearFormError("loginError");

    try {
      await api("/login", {
        method: "POST",
        body: JSON.stringify({
          password: document.getElementById("login_password").value
        })
      });
      state.flash = "登录成功。";
      await bootstrapApp();
    } catch (error) {
      showFormError("loginError", error.message);
    }
  });
}

function bindDashboardTabs() {
  const root = document.querySelector(".dashboard-console");
  if (!root) {
    return;
  }

  const activate = (tabId) => {
    state.dashboardTab = tabId;
    root.dataset.activeTab = tabId;
    root.querySelectorAll("[data-dashboard-tab]").forEach((button) => {
      const active = button.dataset.dashboardTab === tabId;
      button.classList.toggle("active", active);
      button.setAttribute("aria-selected", active ? "true" : "false");
    });
    root.querySelectorAll("[data-tab-page]").forEach((page) => {
      page.classList.toggle("active", page.dataset.tabPage === tabId);
    });
  };

  root.querySelectorAll("[data-dashboard-tab]").forEach((button) => {
    button.addEventListener("click", () => activate(button.dataset.dashboardTab || "settings"));
  });
}

function bindDashboard() {
  document.getElementById("dashboardForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    clearFormError("dashboardError");

    const config = collectConfig();
    const newPassword = document.getElementById("new_admin_password").value;

    if (newPassword && newPassword.length < 8) {
      showFormError("dashboardError", "新管理员密码至少 8 位。");
      return;
    }
    if (!config.local_nas_username) {
      showFormError("dashboardError", "请填写本机 NAS 管理账号。");
      return;
    }
    if (!config.wechat_gateway_url) {
      showFormError("dashboardError", "请填写本地微信网关地址。");
      return;
    }

    try {
      await api("/save", {
        method: "POST",
        body: JSON.stringify({
          new_admin_password: newPassword,
          config
        })
      });
      state.flash = "配置已保存。";
      await bootstrapApp();
    } catch (error) {
      showFormError("dashboardError", error.message);
    }
  });

  document.querySelectorAll(".test-push-btn").forEach((button) => {
    button.addEventListener("click", async () => {
      clearFormError("dashboardError");
      try {
        await api("/test-push", { method: "POST", body: "{}" });
        state.flash = "测试通知已发送，请检查微信入口。";
        await loadGatewayStatus();
        renderApp();
      } catch (error) {
        showFormError("dashboardError", error.message);
      }
    });
  });
}

async function handleLogout() {
  try {
    await api("/logout", { method: "POST", body: "{}" });
  } finally {
    state.flash = "已退出登录。";
    await bootstrapApp();
  }
}

async function bootstrapApp() {
  renderLoading();
  try {
    if (isPreviewMode()) {
      state.bootstrap = buildPreviewBootstrap(currentPreviewMode());
      state.gatewayStatus = currentPreviewMode() === "dashboard" ? buildPreviewGatewayStatus() : null;
      renderApp();
      return;
    }
    await loadBootstrap();
    await loadGatewayStatus();
    renderApp();
  } catch (error) {
    renderError(error.message || "无法连接 NasNotify 服务");
  }
}

updateAppScale();
window.addEventListener("resize", updateAppScale);
bootstrapApp();
