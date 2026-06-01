Place the UGREEN `ugcli` executable here when building UPK packages locally.

The default Windows path expected by `scripts/build-ugreen-native-app.ps1` is:

```text
tools/ugcli/ugcli-v1.1.0.12-windows-amd64.exe
```

The binary is intentionally ignored by Git. Pass `-UgcliPath` to the build
script if you keep it elsewhere.
