# UGREEN Native App Packaging

This directory keeps the UGREEN native app packaging skeleton aligned with the official `project.yaml` + `rootfs_*` layout.

NasNotify keeps the standard `project.yaml` inputs required by `ugcli`, then
patches the generated `config.json` into the same route-style access mode used
by built-in UGREEN desktop apps.

The packaged runtime exposes the app through the bundled Unix socket sidecar and
the nginx route snippet at `/ugreen/v*/nasnotify`, so the NAS portal, DDNS, and
UGLink all stay on the main entry path instead of jumping to an extra mapped
port.

The backend still opens a loopback-only readiness port for UGREEN's package
checker, but user traffic goes through the route proxy and Unix socket.

## Layout

- `project.yaml`: app metadata and runtime settings
- `icon.png`: package icon used by `ugcli`
- `rootfs_common/`: files shared by all architectures
- `rootfs_common/init.d/`: service startup scripts aligned with official UGREEN apps
- `rootfs_common/http.d/`: nginx upstream snippets
- `rootfs_common/nginx/`: nginx proxy route snippets
- `rootfs_common/logrotate/`: runtime log rotation rule
- `rootfs_common/syslog/`: syslog routing rule
- `rootfs_common/uninstall.sh`: uninstall cleanup hook
- `rootfs_common/www/`: generated frontend assets copied by the build script
- `rootfs_amd64/bin/`: Linux AMD64 backend binary
- `rootfs_arm64/bin/`: Linux ARM64 backend binary

## Build Binaries

From the repository root:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-ugreen-native-app.ps1 -Build 1 -Arch all
```

This cross-compiles the Go backend, syncs the frontend into `rootfs_common/www`,
runs `ugcli check`, and lets `ugcli pack` produce the final signed UPK.

The pack script also prints the generated access profile from
`build_dir/rootfs/config.json` so it is obvious whether the final package kept
the intended route-mode metadata.

- `rootfs_amd64/bin/nasnotify`
- `rootfs_arm64/bin/nasnotify`

Generated UPK files are written to `build_dir/pkgs/upk/`.
