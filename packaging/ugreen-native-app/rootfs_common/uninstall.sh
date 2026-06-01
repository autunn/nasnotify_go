#!/bin/sh
set -eu

socket_path=/var/packages/com.autunn.nasnotifyfresh/data/run/nasnotify.sock
app_id=com.autunn.nasnotifyfresh
rm -f /tmp/nasnotify.sock
rm -f /var/ugreen/nasnotify.sock
rm -f "$socket_path"
rm -f /var/targets/nasnotify

find_first_executable() {
    for candidate in "$@"; do
        [ -x "$candidate" ] || continue
        printf '%s\n' "$candidate"
        return 0
    done

    return 1
}

is_positive_pid() {
    case "${1:-}" in
        ''|*[!0-9]*) return 1 ;;
    esac

    [ "$1" -gt 0 ] 2>/dev/null
}

proc_root_for_pid() {
    pid="$1"
    for proc_root in /proc /proc/1/root/proc; do
        [ -d "$proc_root/$pid/root" ] || continue
        printf '%s\n' "$proc_root"
        return 0
    done
    return 1
}

find_entry_serv_target_from_proc() {
    scan_proc_root="$1"
    [ -d "$scan_proc_root" ] || return 1

    for candidate in "$scan_proc_root"/[0-9]*/comm; do
        [ -f "$candidate" ] || continue
        [ "$(cat "$candidate" 2>/dev/null || true)" = "entry_serv" ] || continue

        pid="${candidate%/comm}"
        pid="${pid##*/}"
        if is_positive_pid "$pid" && [ -d "$scan_proc_root/$pid/root" ]; then
            printf '%s:%s\n' "$scan_proc_root" "$pid"
            return 0
        fi
    done

    for candidate in "$scan_proc_root"/[0-9]*/cmdline; do
        [ -f "$candidate" ] || continue
        case "$(tr '\0' ' ' < "$candidate" 2>/dev/null || true)" in
            *entry_serv*) ;;
            *) continue ;;
        esac

        pid="${candidate%/cmdline}"
        pid="${pid##*/}"
        if is_positive_pid "$pid" && [ -d "$scan_proc_root/$pid/root" ]; then
            printf '%s:%s\n' "$scan_proc_root" "$pid"
            return 0
        fi
    done

    return 1
}

find_entry_serv_target() {
    systemctl_bin=$(find_first_executable /usr/bin/systemctl /bin/systemctl /proc/1/root/usr/bin/systemctl /proc/1/root/bin/systemctl || true)
    if [ -n "$systemctl_bin" ]; then
        pid=$("$systemctl_bin" show -p MainPID --value entry_serv.service 2>/dev/null || true)
        pid=$(printf '%s' "$pid" | tr -d '[:space:]')
        if is_positive_pid "$pid" && proc_root=$(proc_root_for_pid "$pid"); then
            printf '%s:%s\n' "$proc_root" "$pid"
            return 0
        fi
    fi

    pgrep_bin=$(find_first_executable /usr/bin/pgrep /bin/pgrep /proc/1/root/usr/bin/pgrep /proc/1/root/bin/pgrep || true)
    if [ -n "$pgrep_bin" ]; then
        pid=$("$pgrep_bin" -xo entry_serv 2>/dev/null || true)
        pid=$(printf '%s' "$pid" | tr -d '[:space:]')
        if is_positive_pid "$pid" && proc_root=$(proc_root_for_pid "$pid"); then
            printf '%s:%s\n' "$proc_root" "$pid"
            return 0
        fi
    fi

    pidof_bin=$(find_first_executable /usr/bin/pidof /bin/pidof /sbin/pidof /proc/1/root/usr/bin/pidof /proc/1/root/bin/pidof /proc/1/root/sbin/pidof || true)
    if [ -n "$pidof_bin" ]; then
        pid=$("$pidof_bin" entry_serv 2>/dev/null || true)
        pid="${pid%% *}"
        if is_positive_pid "$pid" && proc_root=$(proc_root_for_pid "$pid"); then
            printf '%s:%s\n' "$proc_root" "$pid"
            return 0
        fi
    fi

    find_entry_serv_target_from_proc /proc && return 0
    find_entry_serv_target_from_proc /proc/1/root/proc && return 0

    return 1
}

run_shell_at_proc_root() {
    proc_root="${1:-}"
    target_pid="${2:-}"
    shift 2 || true

    if ! is_positive_pid "$target_pid" || [ -z "$proc_root" ] || [ ! -d "$proc_root/$target_pid/root" ]; then
        return 1
    fi

    target_root="$proc_root/$target_pid/root"
    target_mount_ns="$proc_root/$target_pid/ns/mnt"
    nsenter_bin=$(find_first_executable /usr/bin/nsenter /usr/sbin/nsenter /bin/nsenter /proc/1/root/usr/bin/nsenter /proc/1/root/usr/sbin/nsenter /proc/1/root/bin/nsenter || true)
    chroot_bin=$(find_first_executable /usr/sbin/chroot /usr/bin/chroot /bin/chroot /proc/1/root/usr/sbin/chroot /proc/1/root/usr/bin/chroot /proc/1/root/bin/chroot || true)

    if [ -n "$nsenter_bin" ] && [ -n "$chroot_bin" ]; then
        if [ "$proc_root" = "/proc" ] && "$nsenter_bin" -t "$target_pid" -m -- "$chroot_bin" "$target_root" /bin/sh -eu -s "$@"; then
            return 0
        fi
        if [ -e "$target_mount_ns" ] && "$nsenter_bin" --mount="$target_mount_ns" -- "$chroot_bin" "$target_root" /bin/sh -eu -s "$@"; then
            return 0
        fi
    fi

    if [ -n "$chroot_bin" ]; then
        "$chroot_bin" "$target_root" /bin/sh -eu -s "$@"
        return $?
    fi

    return 127
}

run_shell_at_pid_root() {
    target_pid="${1:-}"
    shift || true

    if ! is_positive_pid "$target_pid"; then
        return 1
    fi

    proc_root=$(proc_root_for_pid "$target_pid" || true)
    [ -n "$proc_root" ] || return 1

    run_shell_at_proc_root "$proc_root" "$target_pid" "$@"
}

run_host_shell() {
    run_shell_at_pid_root 1 "$@"
}

cleanup_mobile_shortcuts_for_root() {
    root="$1"
    nas_root="$root/.config/.nas"
    shortcut_root="$root/static/shortcut"

    if [ -d "$nas_root" ]; then
        find "$nas_root" -path "*/desktop/$app_id.shortcut" -type f -exec rm -f {} +
        find "$nas_root" -path "*/desktop/$app_id.accessctrl" -type f -exec rm -f {} +
    fi

    if [ -d "$shortcut_root" ]; then
        find "$shortcut_root" -maxdepth 1 -type f -name "*_${app_id}.png" -exec rm -f {} +
    fi
}

cleanup_mobile_shortcuts_entry_serv() {
    entry_serv_target="${1:-}"
    [ -n "$entry_serv_target" ] || return 1
    entry_serv_proc_root="${entry_serv_target%:*}"
    entry_serv_pid="${entry_serv_target##*:}"
    [ -n "$entry_serv_proc_root" ] || return 1
    [ -n "$entry_serv_pid" ] || return 1

    run_shell_at_proc_root "$entry_serv_proc_root" "$entry_serv_pid" "$app_id" <<'EOF'
app_id="$1"
nas_root=/ugreen/.config/.nas
shortcut_root=/ugreen/static/shortcut

if [ -d "$nas_root" ]; then
    find "$nas_root" -path "*/desktop/$app_id.shortcut" -type f -exec rm -f {} +
    find "$nas_root" -path "*/desktop/$app_id.accessctrl" -type f -exec rm -f {} +
fi

if [ -d "$shortcut_root" ]; then
    find "$shortcut_root" -maxdepth 1 -type f -name "*_${app_id}.png" -exec rm -f {} +
fi
EOF
}

cleanup_mobile_shortcuts_host() {
    run_host_shell "$app_id" <<'EOF'
app_id="$1"
nas_root=/ugreen/.config/.nas
shortcut_root=/ugreen/static/shortcut

if [ -d "$nas_root" ]; then
    find "$nas_root" -path "*/desktop/$app_id.shortcut" -type f -exec rm -f {} +
    find "$nas_root" -path "*/desktop/$app_id.accessctrl" -type f -exec rm -f {} +
fi

if [ -d "$shortcut_root" ]; then
    find "$shortcut_root" -maxdepth 1 -type f -name "*_${app_id}.png" -exec rm -f {} +
fi
EOF
}

cleanup_mobile_shortcuts() {
    entry_serv_target=$(find_entry_serv_target || true)
    if [ -n "$entry_serv_target" ]; then
        cleanup_mobile_shortcuts_entry_serv "$entry_serv_target" || true
    fi

    cleanup_mobile_shortcuts_host || true

    for root in /ugreen /usr/ugreen /proc/self/root/ugreen /proc/1/root/ugreen /proc/[0-9]*/root/ugreen /proc/1/root/proc/[0-9]*/root/ugreen /proc/self/root/usr/ugreen /proc/1/root/usr/ugreen /proc/[0-9]*/root/usr/ugreen /proc/1/root/proc/[0-9]*/root/usr/ugreen; do
        [ -d "$root" ] || continue
        cleanup_mobile_shortcuts_for_root "$root"
    done
}

cleanup_mobile_shortcuts || true
