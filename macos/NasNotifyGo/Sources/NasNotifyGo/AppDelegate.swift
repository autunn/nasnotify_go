import Cocoa
import Foundation
import WebKit

extension String {
    var shellQuoted: String {
        "'" + replacingOccurrences(of: "'", with: "'\\''") + "'"
    }

    var xmlEscaped: String {
        replacingOccurrences(of: "&", with: "&amp;")
            .replacingOccurrences(of: "\"", with: "&quot;")
            .replacingOccurrences(of: "'", with: "&apos;")
            .replacingOccurrences(of: "<", with: "&lt;")
            .replacingOccurrences(of: ">", with: "&gt;")
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    private let port = 5080
    private let serviceLabel = "com.autunn.nasnotify-go.service"
    private let appLabel = "com.autunn.nasnotify-go.app"

    private var statusItem: NSStatusItem?
    private let menu = NSMenu()

    private var statusItemTitle = NSMenuItem()
    private var openDashboardItem = NSMenuItem()
    private var openBrowserItem = NSMenuItem()
    private var startStopItem = NSMenuItem()
    private var restartItem = NSMenuItem()
    private var autoStartItem = NSMenuItem()
    private var openConfigItem = NSMenuItem()
    private var openLogItem = NSMenuItem()
    private var quitItem = NSMenuItem()

    private var dashboardWindow: NSWindow?
    private var dashboardWebView: WKWebView?
    private var runningMenuBarIcon: NSImage?
    private var stoppedMenuBarIcon: NSImage?
    private var timer: Timer?

    private var isLoginLaunch: Bool {
        CommandLine.arguments.contains("--login-item")
    }

    private var appSupportURL: URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/NasNotify-Go", isDirectory: true)
    }

    private var configURL: URL {
        appSupportURL.appendingPathComponent("config", isDirectory: true)
    }

    private var dataURL: URL {
        appSupportURL.appendingPathComponent("data", isDirectory: true)
    }

    private var logDirURL: URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Logs", isDirectory: true)
    }

    private var logFileURL: URL {
        logDirURL.appendingPathComponent("NasNotify-Go.log")
    }

    private var pidFileURL: URL {
        appSupportURL.appendingPathComponent("nasnotify-go.pid")
    }

    private var firstPromptURL: URL {
        appSupportURL.appendingPathComponent(".first_autostart_prompted")
    }

    private var launchAgentsURL: URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/LaunchAgents", isDirectory: true)
    }

    private var servicePlistURL: URL {
        launchAgentsURL.appendingPathComponent("\(serviceLabel).plist")
    }

    private var appPlistURL: URL {
        launchAgentsURL.appendingPathComponent("\(appLabel).plist")
    }

    private var resourcesURL: URL {
        Bundle.main.resourceURL!
    }

    private var backendURL: URL {
        resourcesURL.appendingPathComponent("nasnotify-go-app")
    }

    private var bundledWWWURL: URL {
        resourcesURL.appendingPathComponent("www", isDirectory: true)
    }

    private var appExecutableURL: URL {
        Bundle.main.executableURL!
    }

    private var dashboardURL: URL {
        URL(string: "http://localhost:\(port)")!
    }

    private var serviceBuildMarkerURL: URL {
        appSupportURL.appendingPathComponent(".service_build_marker")
    }

    private var backendEnvironmentShellPrefix: String {
        [
            "UGAPP_DATA_DIR=\(dataURL.path.shellQuoted)",
            "UGAPP_WEB_DIR=\(bundledWWWURL.path.shellQuoted)"
        ].joined(separator: " ")
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        prepareDirectories()
        createMenuBarItem()
        createMenu()
        refreshMenu()

        timer = Timer.scheduledTimer(
            timeInterval: 3,
            target: self,
            selector: #selector(refreshTimer),
            userInfo: nil,
            repeats: true
        )

        startService(openDashboard: !isLoginLaunch)

        if !isLoginLaunch {
            askAutoStartOnce()
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        timer?.invalidate()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }

    private func createMenuBarItem() {
        let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = item.button {
            if let icon = menuBarIcon(running: false) {
                button.title = ""
                button.image = icon
                button.imagePosition = .imageOnly
            } else {
                button.title = "NAS"
            }
            button.toolTip = "NasNotify-Go"
        }
        item.menu = menu
        statusItem = item
    }

    private func createMenu() {
        menu.autoenablesItems = false

        statusItemTitle = NSMenuItem(title: "NasNotify-Go：启动中", action: nil, keyEquivalent: "")
        statusItemTitle.isEnabled = false
        menu.addItem(statusItemTitle)

        menu.addItem(.separator())

        openDashboardItem = NSMenuItem(title: "打开桌面窗口", action: #selector(openDashboard), keyEquivalent: "o")
        openDashboardItem.target = self
        openDashboardItem.isEnabled = true
        menu.addItem(openDashboardItem)

        openBrowserItem = NSMenuItem(title: "在浏览器中打开", action: #selector(openDashboardInBrowser), keyEquivalent: "b")
        openBrowserItem.target = self
        openBrowserItem.isEnabled = true
        menu.addItem(openBrowserItem)

        menu.addItem(.separator())

        startStopItem = NSMenuItem(title: "启动服务", action: #selector(toggleService), keyEquivalent: "s")
        startStopItem.target = self
        startStopItem.isEnabled = true
        menu.addItem(startStopItem)

        restartItem = NSMenuItem(title: "重启服务", action: #selector(restartService), keyEquivalent: "r")
        restartItem.target = self
        restartItem.isEnabled = true
        menu.addItem(restartItem)

        menu.addItem(.separator())

        autoStartItem = NSMenuItem(title: "开启登录自启动", action: #selector(toggleAutoStart), keyEquivalent: "")
        autoStartItem.target = self
        autoStartItem.isEnabled = true
        menu.addItem(autoStartItem)

        menu.addItem(.separator())

        openConfigItem = NSMenuItem(title: "打开配置目录", action: #selector(openConfigDirectory), keyEquivalent: "")
        openConfigItem.target = self
        openConfigItem.isEnabled = true
        menu.addItem(openConfigItem)

        openLogItem = NSMenuItem(title: "打开日志", action: #selector(openLogFile), keyEquivalent: "")
        openLogItem.target = self
        openLogItem.isEnabled = true
        menu.addItem(openLogItem)

        menu.addItem(.separator())

        quitItem = NSMenuItem(title: "退出 NasNotify-Go", action: #selector(quitApp), keyEquivalent: "q")
        quitItem.target = self
        quitItem.isEnabled = true
        menu.addItem(quitItem)
    }

    @objc private func refreshTimer() {
        refreshMenu()
    }

    private func refreshMenu() {
        let running = isServiceRunning()
        let autoStart = isAutoStartEnabled()

        if let button = statusItem?.button {
            if let icon = menuBarIcon(running: running) {
                button.title = ""
                button.image = icon
                button.imagePosition = .imageOnly
            } else {
                button.title = running ? "NAS ●" : "NAS ○"
            }
            button.toolTip = running ? "NasNotify-Go 正在运行" : "NasNotify-Go 已停止"
        }

        statusItemTitle.title = running ? "NasNotify-Go：运行中" : "NasNotify-Go：已停止"
        startStopItem.title = running ? "停止服务" : "启动服务"
        restartItem.isEnabled = running
        openDashboardItem.isEnabled = true
        openBrowserItem.isEnabled = true
        autoStartItem.title = autoStart ? "关闭登录自启动" : "开启登录自启动"
    }

    private func menuBarIcon(running: Bool) -> NSImage? {
        if running, let icon = runningMenuBarIcon {
            return icon
        }
        if !running, let icon = stoppedMenuBarIcon {
            return icon
        }

        guard let sourceURL = Bundle.main.resourceURL?.appendingPathComponent("AppIcon.png"),
              let source = NSImage(contentsOf: sourceURL) else {
            return nil
        }

        let size = NSSize(width: 20, height: 20)
        let image = NSImage(size: size)
        image.lockFocus()
        source.draw(
            in: NSRect(x: 1.5, y: 1.5, width: 17, height: 17),
            from: .zero,
            operation: .sourceOver,
            fraction: running ? 1.0 : 0.58
        )

        let dotRect = NSRect(x: 14.2, y: 2.0, width: 4.8, height: 4.8)
        (running ? NSColor.systemGreen : NSColor.systemGray).setFill()
        NSBezierPath(ovalIn: dotRect).fill()
        NSColor.black.withAlphaComponent(0.35).setStroke()
        let dotStroke = NSBezierPath(ovalIn: dotRect)
        dotStroke.lineWidth = 0.6
        dotStroke.stroke()
        image.unlockFocus()
        image.isTemplate = false

        if running {
            runningMenuBarIcon = image
        } else {
            stoppedMenuBarIcon = image
        }

        return image
    }

    private func prepareDirectories() {
        let fm = FileManager.default

        do {
            try fm.createDirectory(at: appSupportURL, withIntermediateDirectories: true)
            try fm.createDirectory(at: configURL, withIntermediateDirectories: true)
            try fm.createDirectory(at: dataURL, withIntermediateDirectories: true)
            try fm.createDirectory(at: logDirURL, withIntermediateDirectories: true)
            try fm.createDirectory(at: launchAgentsURL, withIntermediateDirectories: true)
        } catch {
            alert("初始化目录失败", error.localizedDescription)
        }
    }

    private func isServiceRunning() -> Bool {
        run("/usr/sbin/lsof -nP -iTCP:\(port) -sTCP:LISTEN >/dev/null 2>&1").status == 0
    }

    private func isAutoStartEnabled() -> Bool {
        FileManager.default.fileExists(atPath: servicePlistURL.path)
            && FileManager.default.fileExists(atPath: appPlistURL.path)
    }

    private func currentServiceBuildMarker() -> String {
        var parts = [Bundle.main.bundlePath]
        appendFileMarker(backendURL, to: &parts)
        appendFileMarker(bundledWWWURL.appendingPathComponent("index.html"), to: &parts)
        appendFileMarker(bundledWWWURL.appendingPathComponent("version.json"), to: &parts)

        let assetURL = bundledWWWURL.appendingPathComponent("assets", isDirectory: true)
        if let enumerator = FileManager.default.enumerator(
            at: assetURL,
            includingPropertiesForKeys: [.isRegularFileKey, .contentModificationDateKey, .fileSizeKey],
            options: [.skipsHiddenFiles]
        ) {
            for case let url as URL in enumerator {
                appendFileMarker(url, to: &parts)
            }
        }

        return parts.joined(separator: "|")
    }

    private func appendFileMarker(_ url: URL, to parts: inout [String]) {
        guard let attrs = try? FileManager.default.attributesOfItem(atPath: url.path) else {
            parts.append("\(url.path):missing")
            return
        }

        let size = (attrs[.size] as? NSNumber)?.int64Value ?? 0
        let modifiedAt = ((attrs[.modificationDate] as? Date)?.timeIntervalSince1970 ?? 0).rounded()
        parts.append("\(url.path):\(size):\(Int(modifiedAt))")
    }

    private func serviceNeedsRefresh(currentMarker: String) -> Bool {
        let previous = (try? String(contentsOf: serviceBuildMarkerURL, encoding: .utf8))?
            .trimmingCharacters(in: .whitespacesAndNewlines)
        return previous != currentMarker
    }

    private func writeServiceBuildMarker(_ marker: String) {
        try? marker.write(to: serviceBuildMarkerURL, atomically: true, encoding: .utf8)
    }

    private func startService(openDashboard: Bool) {
        DispatchQueue.global(qos: .utility).async {
            let ok = self.startServiceBlocking()

            DispatchQueue.main.async {
                self.refreshMenu()

                if ok {
                    if openDashboard {
                        self.showDashboardWindow(reload: true)
                    }
                } else {
                    self.alert("NasNotify-Go 启动失败", "请查看日志：\n\(self.logFileURL.path)")
                }
            }
        }
    }

    private func startServiceBlocking() -> Bool {
        prepareDirectories()
        let marker = currentServiceBuildMarker()

        if isServiceRunning() {
            if serviceNeedsRefresh(currentMarker: marker) {
                stopServiceBlocking()
                Thread.sleep(forTimeInterval: 0.6)
            } else {
                return true
            }
        }

        if FileManager.default.fileExists(atPath: servicePlistURL.path) {
            writeServiceLaunchAgent()
            if FileManager.default.fileExists(atPath: appPlistURL.path) {
                writeAppLaunchAgent()
            }

            _ = run("/bin/launchctl bootout gui/$(id -u)/\(serviceLabel) >/dev/null 2>&1 || true")
            _ = run("/bin/launchctl bootout gui/$(id -u) \(servicePlistURL.path.shellQuoted) >/dev/null 2>&1 || true")
            _ = run("/bin/launchctl bootstrap gui/$(id -u) \(servicePlistURL.path.shellQuoted) >/dev/null 2>&1 || true")
            _ = run("/bin/launchctl kickstart -k gui/$(id -u)/\(serviceLabel) >/dev/null 2>&1 || true")
        } else {
            let command = """
            cd \(appSupportURL.path.shellQuoted) || exit 1
            \(backendEnvironmentShellPrefix) nohup \(backendURL.path.shellQuoted) >> \(logFileURL.path.shellQuoted) 2>&1 &
            echo $! > \(pidFileURL.path.shellQuoted)
            """

            _ = run(command)
        }

        for _ in 0..<30 {
            if isServiceRunning() {
                writeServiceBuildMarker(marker)
                return true
            }

            Thread.sleep(forTimeInterval: 1)
        }

        return false
    }

    private func stopService() {
        DispatchQueue.global(qos: .utility).async {
            self.stopServiceBlocking()

            DispatchQueue.main.async {
                self.refreshMenu()
            }
        }
    }

    private func stopServiceBlocking() {
        _ = run("/bin/launchctl bootout gui/$(id -u)/\(serviceLabel) >/dev/null 2>&1 || true")
        _ = run("/bin/launchctl bootout gui/$(id -u) \(servicePlistURL.path.shellQuoted) >/dev/null 2>&1 || true")

        if FileManager.default.fileExists(atPath: pidFileURL.path) {
            let pid = (try? String(contentsOf: pidFileURL, encoding: .utf8))?
                .trimmingCharacters(in: .whitespacesAndNewlines)

            if let pid, !pid.isEmpty {
                _ = run("/bin/kill \(pid) >/dev/null 2>&1 || true")
            }

            try? FileManager.default.removeItem(at: pidFileURL)
        }

        _ = run("/usr/bin/pkill -f nasnotify-go-app >/dev/null 2>&1 || true")
    }

    @objc private func toggleService() {
        if isServiceRunning() {
            stopService()
        } else {
            startService(openDashboard: true)
        }
    }

    @objc private func restartService() {
        DispatchQueue.global(qos: .utility).async {
            self.stopServiceBlocking()
            Thread.sleep(forTimeInterval: 1)
            let ok = self.startServiceBlocking()

            DispatchQueue.main.async {
                self.refreshMenu()

                if ok {
                    self.showDashboardWindow(reload: true)
                } else {
                    self.alert("NasNotify-Go 重启失败", "请查看日志：\n\(self.logFileURL.path)")
                }
            }
        }
    }

    @objc private func openDashboard() {
        if isServiceRunning() {
            showDashboardWindow(reload: false)
        } else {
            startService(openDashboard: true)
        }
    }

    @objc private func openDashboardInBrowser() {
        if isServiceRunning() {
            NSWorkspace.shared.open(dashboardURL)
            return
        }

        DispatchQueue.global(qos: .utility).async {
            let ok = self.startServiceBlocking()

            DispatchQueue.main.async {
                self.refreshMenu()
                if ok {
                    NSWorkspace.shared.open(self.dashboardURL)
                } else {
                    self.alert("NasNotify-Go 启动失败", "请查看日志：\n\(self.logFileURL.path)")
                }
            }
        }
    }

    private func showDashboardWindow(reload: Bool) {
        NSApp.setActivationPolicy(.regular)

        let webView = ensureDashboardWindow()
        let request = URLRequest(url: dashboardURL, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 30)

        if reload || webView.url == nil {
            loadDashboard(webView, request: request, clearingCache: reload)
        }

        dashboardWindow?.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    private func loadDashboard(_ webView: WKWebView, request: URLRequest, clearingCache: Bool) {
        guard clearingCache else {
            webView.load(request)
            return
        }

        let cacheTypes: Set<String> = [
            WKWebsiteDataTypeDiskCache,
            WKWebsiteDataTypeMemoryCache
        ]
        WKWebsiteDataStore.default().removeData(ofTypes: cacheTypes, modifiedSince: Date.distantPast) {
            DispatchQueue.main.async {
                webView.load(request)
            }
        }
    }

    @discardableResult
    private func ensureDashboardWindow() -> WKWebView {
        if let webView = dashboardWebView, dashboardWindow != nil {
            return webView
        }

        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .default()
        configuration.preferences.javaScriptCanOpenWindowsAutomatically = true

        let webView = WKWebView(frame: .zero, configuration: configuration)
        webView.allowsBackForwardNavigationGestures = true

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1180, height: 760),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        window.title = "NasNotify-Go"
        window.minSize = NSSize(width: 980, height: 640)
        window.backgroundColor = .black
        window.titlebarAppearsTransparent = true
        window.isReleasedWhenClosed = false
        window.contentView = webView
        window.center()

        dashboardWindow = window
        dashboardWebView = webView
        return webView
    }

    @objc private func toggleAutoStart() {
        if isAutoStartEnabled() {
            disableAutoStart()
        } else {
            enableAutoStart()
        }
    }

    private func enableAutoStart() {
        prepareDirectories()
        writeServiceLaunchAgent()
        writeAppLaunchAgent()

        _ = run("/bin/launchctl bootout gui/$(id -u)/\(serviceLabel) >/dev/null 2>&1 || true")
        _ = run("/bin/launchctl bootout gui/$(id -u) \(servicePlistURL.path.shellQuoted) >/dev/null 2>&1 || true")
        _ = run("/bin/launchctl bootstrap gui/$(id -u) \(servicePlistURL.path.shellQuoted) >/dev/null 2>&1 || true")
        _ = run("/bin/launchctl kickstart -k gui/$(id -u)/\(serviceLabel) >/dev/null 2>&1 || true")

        _ = run("/bin/launchctl bootout gui/$(id -u)/\(appLabel) >/dev/null 2>&1 || true")
        _ = run("/bin/launchctl bootout gui/$(id -u) \(appPlistURL.path.shellQuoted) >/dev/null 2>&1 || true")
        _ = run("/bin/launchctl bootstrap gui/$(id -u) \(appPlistURL.path.shellQuoted) >/dev/null 2>&1 || true")

        refreshMenu()
        notify("已开启登录自启动")
    }

    private func disableAutoStart() {
        _ = run("/bin/launchctl bootout gui/$(id -u)/\(appLabel) >/dev/null 2>&1 || true")
        _ = run("/bin/launchctl bootout gui/$(id -u)/\(serviceLabel) >/dev/null 2>&1 || true")
        _ = run("/bin/launchctl bootout gui/$(id -u) \(appPlistURL.path.shellQuoted) >/dev/null 2>&1 || true")
        _ = run("/bin/launchctl bootout gui/$(id -u) \(servicePlistURL.path.shellQuoted) >/dev/null 2>&1 || true")

        try? FileManager.default.removeItem(at: appPlistURL)
        try? FileManager.default.removeItem(at: servicePlistURL)

        refreshMenu()
        notify("已关闭登录自启动")
    }

    private func writeServiceLaunchAgent() {
        let plist = """
        <?xml version="1.0" encoding="UTF-8"?>
        <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
          "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
        <plist version="1.0">
        <dict>
          <key>Label</key>
          <string>\(serviceLabel)</string>
          <key>ProgramArguments</key>
          <array>
            <string>\(backendURL.path.xmlEscaped)</string>
          </array>
          <key>EnvironmentVariables</key>
          <dict>
            <key>UGAPP_DATA_DIR</key>
            <string>\(dataURL.path.xmlEscaped)</string>
            <key>UGAPP_WEB_DIR</key>
            <string>\(bundledWWWURL.path.xmlEscaped)</string>
          </dict>
          <key>WorkingDirectory</key>
          <string>\(appSupportURL.path.xmlEscaped)</string>
          <key>RunAtLoad</key>
          <true/>
          <key>KeepAlive</key>
          <true/>
          <key>StandardOutPath</key>
          <string>\(logDirURL.appendingPathComponent("NasNotify-Go.launchd.out.log").path.xmlEscaped)</string>
          <key>StandardErrorPath</key>
          <string>\(logDirURL.appendingPathComponent("NasNotify-Go.launchd.err.log").path.xmlEscaped)</string>
          <key>ProcessType</key>
          <string>Background</string>
        </dict>
        </plist>
        """

        do {
            try plist.write(to: servicePlistURL, atomically: true, encoding: .utf8)
            _ = run("/bin/chmod 644 \(servicePlistURL.path.shellQuoted)")
        } catch {
            alert("写入服务自启动失败", error.localizedDescription)
        }
    }

    private func writeAppLaunchAgent() {
        let plist = """
        <?xml version="1.0" encoding="UTF-8"?>
        <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
          "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
        <plist version="1.0">
        <dict>
          <key>Label</key>
          <string>\(appLabel)</string>
          <key>ProgramArguments</key>
          <array>
            <string>\(appExecutableURL.path.xmlEscaped)</string>
            <string>--login-item</string>
          </array>
          <key>RunAtLoad</key>
          <true/>
          <key>KeepAlive</key>
          <false/>
          <key>ProcessType</key>
          <string>Interactive</string>
        </dict>
        </plist>
        """

        do {
            try plist.write(to: appPlistURL, atomically: true, encoding: .utf8)
            _ = run("/bin/chmod 644 \(appPlistURL.path.shellQuoted)")
        } catch {
            alert("写入应用自启动失败", error.localizedDescription)
        }
    }

    private func askAutoStartOnce() {
        if FileManager.default.fileExists(atPath: firstPromptURL.path) {
            return
        }

        try? "yes".write(to: firstPromptURL, atomically: true, encoding: .utf8)

        let alert = NSAlert()
        alert.messageText = "是否开启登录自启动？"
        alert.informativeText = "开启后，macOS 登录后会自动启动 NasNotify-Go 后台服务，并保留菜单栏入口。"
        alert.addButton(withTitle: "开启自启动")
        alert.addButton(withTitle: "暂不启用")

        if alert.runModal() == .alertFirstButtonReturn {
            enableAutoStart()
        }
    }

    @objc private func openConfigDirectory() {
        NSWorkspace.shared.open(appSupportURL)
    }

    @objc private func openLogFile() {
        if FileManager.default.fileExists(atPath: logFileURL.path) {
            NSWorkspace.shared.open(logFileURL)
        } else {
            alert("日志不存在", logFileURL.path)
        }
    }

    @objc private func quitApp() {
        NSApp.terminate(nil)
    }

    private func alert(_ title: String, _ message: String) {
        DispatchQueue.main.async {
            let alert = NSAlert()
            alert.messageText = title
            alert.informativeText = message
            alert.runModal()
        }
    }

    private func notify(_ message: String) {
        let notification = NSUserNotification()
        notification.title = "NasNotify-Go"
        notification.informativeText = message
        NSUserNotificationCenter.default.deliver(notification)
    }

    @discardableResult
    private func run(_ command: String) -> (status: Int32, output: String) {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/bash")
        process.arguments = ["-lc", command]

        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = pipe

        do {
            try process.run()
            process.waitUntilExit()

            let data = pipe.fileHandleForReading.readDataToEndOfFile()
            let output = String(data: data, encoding: .utf8) ?? ""

            return (process.terminationStatus, output)
        } catch {
            return (1, error.localizedDescription)
        }
    }
}
