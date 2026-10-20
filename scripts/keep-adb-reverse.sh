#!/usr/bin/env bash
# Automatically keeps adb reverse tunnels active whenever an Android device is attached.

while true; do
  DEVICE=$(adb devices 2>/dev/null | grep -E "^[a-zA-Z0-9_-]+[[:space:]]+device" | awk '{print $1}' | head -n 1)
  if [ -n "$DEVICE" ]; then
    LIST=$(adb -s "$DEVICE" reverse --list 2>/dev/null)
    if ! echo "$LIST" | grep -q "tcp:8080"; then
      adb -s "$DEVICE" reverse tcp:8080 tcp:8080 2>/dev/null
    fi
    if ! echo "$LIST" | grep -q "tcp:5173"; then
      adb -s "$DEVICE" reverse tcp:5173 tcp:5173 2>/dev/null
    fi
  else
    # If OnePlus device is plugged in via USB but ADB dropped, restart adb server
    USB_ONEPLUS=$(lsusb 2>/dev/null | grep -iE "22d9:2769|OPPO|OnePlus")
    if [ -n "$USB_ONEPLUS" ]; then
      adb kill-server 2>/dev/null
      adb start-server 2>/dev/null
    fi
  fi
  sleep 2
done
