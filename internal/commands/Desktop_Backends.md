# Desktop Backends — native, zero-dependency display + input

> **Status:** design + rollout plan. Phase 0 (this doc) is committed first so the
> goal is unambiguous; phases 1–4 implement it. Track progress against the
> **TODO** at the bottom.

## Goal

Drive the GUI (screenshot + mouse + keyboard) with **no external runtime
dependencies** — no `scrot`/`xdotool`/`grim`/`wtype`/`ydotool`/`cliclick` to
install, no `apt`/`brew`, no `sudo`, no `/dev/uinput` udev rules. The daemon
talks the platform's display + input interfaces **directly**, so the daemon
*is* the dependency. This is the non-invasive, best-practice shape for the
desktop-app ("use my own machine") path, and it deletes three whole failure
classes from the current shell-out design:

1. "desktop tooling missing" (wrong/absent package),
2. distro/package-name drift (apt vs dnf vs pacman vs apk),
3. `ydotool`'s `/dev/uinput` permission + `ydotoold` lifecycle.

The current shell-out backend (see `desktop_linux.go` etc.) stays as a
**fallback** during the migration and for environments the native path doesn't
cover, selectable via `IDAPT_DESKTOP_BACKEND`.

## Feasibility (verified May 2026)

| Platform | Native approach | Pure Go (no CGO)? | External runtime dep? | User consent? |
|---|---|---|---|---|
| **Linux / X11** | `xgb` X protocol: `XGetImage` (capture) + `XTEST` `FakeInput` (input) | ✅ yes | none (connects to `$DISPLAY`) | none |
| **Linux / Wayland (wlroots)** | `wlr-screencopy` (capture) + `zwlr_virtual_pointer_v1` + `zwp_virtual_keyboard_v1` (input) | ✅ yes | none (Wayland socket) | none — **no root / no uinput** |
| **Windows** | GDI `BitBlt` (capture) + `SendInput` via `user32` syscalls | ✅ yes | none (`user32`/`gdi32` are OS built-ins) | none |
| **macOS** | CoreGraphics: `CGDisplayCreateImage` (capture) + `CGEventPost` (input) | ❌ **needs CGO** | none (system framework, always present) | **TCC** (Screen Recording + Accessibility), OS-mandated |

References: `xgb`/`xtest` (`github.com/jezek/xgb`, fork of BurntSushi),
`wlr-virtual-pointer-unstable-v1` + `virtual-keyboard-unstable-v1` +
`wlr-screencopy-unstable-v1` (wayland.app/protocols; Go bindings exist, e.g.
`bnema/libwldevices-go`), `SendInput` without CGO (`stephen-fox/user32util`,
mind golang/go#31685), CoreGraphics via CGO (proven by `go-vgo/robotgo`).

### Carve-outs (explicitly out of scope for this initiative)

- **GNOME / KDE Wayland** do **not** implement the wlroots protocols. Capture is
  gated behind `xdg-desktop-portal` (PipeWire ScreenCast) and input behind the
  RemoteDesktop portal / `libei`, each with a consent dialog. Detected → clear
  "use an Xorg session or a wlroots compositor (Sway/Hyprland/…)" message. The
  portal path is a separate, larger phase.
- **macOS is CGO**, not pure-Go. That's fine: the `CGO_ENABLED=0` constraint
  only needs to hold for the **static Linux server daemon**. The macOS backend
  is build-tagged `darwin && cgo` and ships CGO-built inside the signed/notarized
  desktop `.app`; a `darwin && !cgo` fallback keeps `CGO_ENABLED=0` darwin builds
  compiling (shell-out / capture-only).

## Architecture

Replace the per-OS free functions (`desktopProbe`/`desktopCapture`/
`desktopInput`/`desktopCursorPosition`) with a selectable **backend**:

```go
// DesktopBackend is one way to drive a GUI session. Implementations are
// build-tagged per OS; selection is per command (the session can differ).
type DesktopBackend interface {
    Name() string                                   // "x11-native", "wayland-native", "shell", "windows", "macos"
    Probe() (ok bool, hint string)                  // is this backend usable right now?
    Capture(ctx context.Context) (png []byte, w, h int, err error)
    Input(ctx context.Context, p DesktopPayload) error
    CursorPosition(ctx context.Context) (x, y int, err error)
}
```

- `selectDesktopBackend(env) DesktopBackend` (per-OS file) picks the backend
  from the session + `IDAPT_DESKTOP_BACKEND` override:
  - `native` (default) → X11/Wayland/Windows/macOS native; falls back to `shell`
    if the native backend's `Probe()` fails and shell tooling is present.
  - `shell` → force the legacy shell-out backend (escape hatch / parity tests).
- `runDesktop` (in `desktop.go`) becomes backend-agnostic: validate runAs →
  select backend → `Probe()` (→ `runtime-unavailable` with hint) → dispatch the
  action to `Capture`/`Input`/`CursorPosition`. `Capture` returns PNG **bytes**
  directly (native backends encode `image.Image`→PNG in Go; the shell backend
  keeps its temp-file dance internally), removing the 100 KB stdout concern.
- Coordinates remain **real screen pixels** (cloud-side downscaling +
  coordinate scaling is unchanged — see `../../../../backend/lib/computer-use/Computer_Use.md`).

### Action coverage parity

Native backends must match the existing action vocabulary
(`screenshot`, `cursor-position`, `mouse-move`, `left/right/middle/double/triple-click`,
`left-click-drag`, `left-mouse-down/up`, `scroll`, `key`, `type`, `hold-key`,
`wait`). Where a platform genuinely can't (e.g. Wayland `cursor-position`,
Wayland `scroll`/`hold-key` in v1, macOS scroll/middle), return the **same clear
"unsupported on <platform> (v1)" errors** the shell backend returns today, so
behavior is identical regardless of backend.

## Rollout phases (each its own commit, integration-tested before the next)

Order is by value-per-risk. Each phase keeps the shell-out backend as fallback
and must pass the **existing** green test suite before landing.

- **Phase 1 — X11 native** (`xgb`/`xtest`). Replaces `scrot`+`xdotool`. Highest
  value, lowest risk (`xgb` is mature). Integration-tested on the daemon-test
  **Xvfb** (1920×1080) — the full action matrix already exercised there.
- **Phase 2 — Windows native** (syscall `SendInput` + GDI). Replaces PowerShell
  (also removes per-call PS latency). No Windows CI runner → unit-tested
  (command/struct construction) + manual real-host validation; documented.
- **Phase 3 — wlroots Wayland native** (virtual-pointer/keyboard + screencopy).
  Replaces `grim`+`wtype`+`ydotool` and removes the uinput requirement.
  Integration-tested on the daemon-test **headless sway**.
- **Phase 4 — macOS native** (CGO CoreGraphics) + `darwin && !cgo` fallback +
  TCC guidance. No macOS CI runner → unit-tested where possible + documented.

After all four land and are green, the shell-out tooling is removed from the
daemon-test container (kept only behind the `IDAPT_DESKTOP_BACKEND=shell`
escape hatch's documentation), confirming zero runtime deps.

## Integration test plan

The daemon-test container already runs **both** Xvfb (X11) and headless **sway**
(Wayland). The native backends are validated by re-running the existing
`computer-use-desktop.test.ts` (X11 + Wayland sub-suites) and
`computer-use-agent-flow.test.ts` against the native backend (default) — same
assertions, no skips. A parity guard runs the X11 suite once with
`IDAPT_DESKTOP_BACKEND=shell` and once `native` to prove identical results
during the migration window. Windows/macOS: Go unit tests + documented manual
validation (no CI runners).

## Dependencies to add (`services/idapt/go.mod`, all pure-Go except macOS)

- X11: `github.com/jezek/xgb` (+ `xgb/xtest`, `xgb/xproto`).
- Wayland: a wlroots client lib for virtual-pointer/keyboard + screencopy
  (evaluate `github.com/bnema/libwldevices-go` and `github.com/neurlang/wayland`;
  vendor the minimal protocol bindings if the dep surface is too large).
- Windows: `github.com/stephen-fox/user32util` (or hand-rolled `golang.org/x/sys/windows`
  `SendInput`); GDI capture via `x/sys/windows` + a small BitBlt helper.
- macOS: no Go dep — CGO `import "C"` against `CoreGraphics`/`ApplicationServices`.

Keep the daemon `CGO_ENABLED=0` for Linux + Windows. macOS native is a CGO build.

## TODO

### Phase 0 — docs (this commit)
- [x] Write this design doc; link from `Computer_Use.md`.

### Phase 1 — X11 native ✅ (pending final k8s confirmation)
- [x] Add `jezek/xgb` to `go.mod` (pure-Go; xgbutil dropped — keysym resolution is self-contained).
- [x] `DesktopBackend` interface + `selectDesktopBackend` + `IDAPT_DESKTOP_BACKEND` override.
- [x] Refactor `desktop.go` dispatch to the interface; `Capture` returns PNG bytes; `shellBackend` is the fallback.
- [x] `x11NativeBackend`: `GetImage`→PNG capture; `XTEST` `FakeInput` for move/click/drag/scroll/down-up; keysym resolution from the live `GetKeyboardMapping` for `key`/`type`/`hold-key`/modifier-held clicks; `QueryPointer` for `cursor-position`.
- [x] Go unit tests: pure keysym/chord resolver (`desktop_x11_keymap_test.go`, CI) + live backend test (`desktop_x11_native_test.go`, skips without X) — validated locally on Xvfb (capture + all 16 actions + cursor).
- [ ] Integration: `computer-use-desktop.test.ts` (X11) + agent-flow green on the native backend (k8s daemon-test).

### Phase 2 — Windows native ✅ (pending Windows-runner validation)
- [x] GDI `BitBlt`→PNG capture (virtual screen); `SendInput` mouse + keyboard via `user32` (documented amd64 `INPUT` union layout, golang/go#31685).
- [x] `chord`→VK mapping; `type` via `KEYEVENTF_UNICODE` (layout-independent, no map); `scroll` via `MOUSEEVENTF_WHEEL`/`HWHEEL`; `cursor-position` via `GetCursorPos`.
- [x] Pure VK-chord unit tests (CI, `desktop_windows_keymap_test.go`); compile-checked for `GOOS=windows` (the package now builds on Windows after the `procgroup` build-tag split of local_inference). Native is the default on Windows; `IDAPT_DESKTOP_BACKEND=shell` keeps PowerShell.
- [ ] Validate on a real Windows runner — the `SendInput` union layout + GDI calls are compile-checked but runtime-unverified here (no Windows CI), same posture as the prior PowerShell backend.

### Phase 3 — wlroots Wayland native ✅ input (pending final k8s confirmation)
- [x] Add `wayland-virtual-input-go` (virtual-pointer + virtual-keyboard; pure-Go, builds on neurlang/wayland).
- [x] `waylandNativeBackend`: native input — move/click/drag/**scroll**/down-up via virtual-pointer; type/key/**hold-key** via virtual-keyboard on a US evdev map; **no uinput, no ydotoold, no root** (replaces wtype + ydotool). `cursor-position` unsupported on Wayland (no global pointer query).
- [x] Go unit tests: pure evdev chord/rune resolver (CI) + live backend test (skips without a compositor) — validated locally on headless sway (capture + all input actions).
- [ ] Integration on headless sway (k8s daemon-test): native input.
- [ ] **Capture is still `grim`** — native in-Go screencopy is the remaining item: the input lib's minimal wl client can't pass the wl_shm fd, so do it via `neurlang/wayland` + a hand-written `zwlr_screencopy_v1` proxy. Tracked below.

### Phase 5 — native Wayland screencopy capture (drop grim)
- [ ] Implement `zwlr_screencopy_v1` + `wl_shm` capture over `neurlang/wayland` (has fd-passing + shm + output), read the shm buffer → PNG. Removes the last Wayland external dep.

### Phase 4 — macOS native ✅ (pending Mac build/runner validation)
- [x] `darwin && cgo` CoreGraphics backend: CGEvent mouse/keyboard input (replaces cliclick); `type` via `CGEventKeyboardSetUnicodeString`; capture via the built-in `screencapture` (already dep-free). `darwin && !cgo` fallback keeps the static `CGO_ENABLED=0` darwin build compiling (shell: screencapture + cliclick).
- [x] kVK chord mapping with pure CI unit tests (`desktop_darwin_keymap_test.go`); TCC (Accessibility + Screen Recording) requirement documented in the cgo file.
- [ ] Compile + validate on a Mac — the cgo file isn't built in this Linux CI, so the C/CoreGraphics layer is unverified here. Notarization + a TCC consent UX in the desktop app are follow-ups.

### Cleanup
- [x] `IDAPT_DESKTOP_BACKEND` escape hatch documented; `Computer_Use.md` cross-platform table updated to native backends.
- [x] Dropped `scrot/xdotool/wtype/ydotool` from `docker/daemon-test/Dockerfile` — the native backends use none of them; only `grim` (Wayland capture, until Phase 5) + Xvfb + sway remain, so the integration suite proves the native paths work with no input shell tools.

@see ../../../../backend/lib/computer-use/Computer_Use.md
