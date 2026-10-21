# Upgrading to azctx 2026.11

This guide tracks breaking changes from 2024.12.5. Later release layers add
their changes to the table below.

## Breaking changes

| Area | Change | Who is affected | What to do |
| --- | --- | --- | --- |
| Windows builds | The 32-bit `windows/arm` build is removed; Windows on ARM64 remains supported. | Users of the 32-bit Windows ARM binary. | Move to a supported 64-bit Windows system and use the `windows/arm64` or `windows/amd64` binary. |
| macOS | macOS 13 or later is required by Go 1.27. | Users running macOS 12 or earlier. | Upgrade to macOS 13 or later before installing azctx 2026.11. |
| Windows | Windows 10 or later is required by Go 1.27. | Users running older versions of Windows. | Upgrade to Windows 10 or later before installing azctx 2026.11. |
| Source builds | The minimum Go version is 1.26; the preferred toolchain is Go 1.27.2. | Contributors and users building azctx from source. | Use the Go version and toolchain declared in `go.mod`. |

## Upgrade checklist

- Check the breaking changes above before upgrading.
- Choose a binary for your operating system and architecture: Linux
  (`amd64`, `arm64`, `arm`), macOS (`amd64`, `arm64`), or Windows (`amd64`, `arm64`).
- Follow the [installation instructions](../README.md#installation) for your
  package manager or binary installation.
