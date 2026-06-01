#!/bin/bash
socket_path=/var/packages/com.autunn.nasnotifyfresh/data/run/nasnotify.sock
rm -f /tmp/nasnotify.sock
rm -f /var/ugreen/nasnotify.sock
rm -f "$socket_path"
