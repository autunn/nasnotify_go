#!/bin/bash
set -eu

rootfs=$(dirname "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")")
data_dir=/var/packages/com.autunn.nasnotifyfresh/data
socket_path="$data_dir/run/nasnotify.sock"
app_id=com.autunn.nasnotifyfresh

cleanup_legacy_shortcuts_for_root() {
    local root="$1"
    local nas_root="$root/.config/.nas"
    local shortcut_root="$root/static/shortcut"

    if [ -d "$nas_root" ]; then
        find "$nas_root" -path "*/desktop/$app_id.shortcut" -type f -exec rm -f {} + 2>/dev/null || true
        find "$nas_root" -path "*/desktop/$app_id.accessctrl" -type f -exec rm -f {} + 2>/dev/null || true
    fi

    if [ -d "$shortcut_root" ]; then
        find "$shortcut_root" -maxdepth 1 -type f -name "*_${app_id}.png" -exec rm -f {} + 2>/dev/null || true
    fi
}

cleanup_legacy_shortcuts() {
    local root=""

    for root in \
        /ugreen \
        /usr/ugreen \
        /proc/self/root/ugreen \
        /proc/1/root/ugreen \
        /proc/self/root/usr/ugreen \
        /proc/1/root/usr/ugreen \
        /proc/[0-9]*/root/ugreen \
        /proc/[0-9]*/root/usr/ugreen \
        /proc/1/root/proc/[0-9]*/root/ugreen \
        /proc/1/root/proc/[0-9]*/root/usr/ugreen; do
        [ -d "$root" ] || continue
        cleanup_legacy_shortcuts_for_root "$root"
    done
}

mkdir -p /var/targets
ln -fsn "$rootfs/bin/nasnotify" /var/targets/nasnotify

mkdir -p "$data_dir/run" "$data_dir/log"
rm -f /tmp/nasnotify.sock
rm -f /var/ugreen/nasnotify.sock
rm -f "$socket_path"

cleanup_legacy_shortcuts
