# niri-windows module for Waybar

This is a module for [Waybar](https://github.com/Alexays/Waybar) that displays a focus indicator for the current [niri](https://github.com/YaLTeR/niri) workspace.

![Image of the module](screenshot.png)

Tiled columns are shown as `⋅` and floating windows are shown as `∗`. A circle is drawn around a focused column/window. For example:

Two columns, first column focused:

```
⊙⋅
```

Three columns, two floating windows, rightmost floating window is focused:

```
⋅⋅⋅ ∗⊛
```

> [!IMPORTANT]
> niri ≥ v25.08 is required (for the window locations in IPC messages to be available).

## Installation

Download the latest release for your platform from the [releases page](https://github.com/calico32/waybar-niri-windows/releases). Mark the binary executable and place it on your `$PATH` (e.g. `~/.local/bin`).

### From source

If you'd like to build from source (or if your platform doesn't have a pre-built binary), clone this repository and:

- run `go build .`, and copy the resulting binary to your `$PATH` (e.g. `~/.local/bin`); or
- run `go install .` to install to `$GOBIN` (usually `~/go/bin`, make sure to add it to your `$PATH`)

Add a custom module to your Waybar config (and add any actions you want to trigger on click/scroll):

```json
{
  "modules-left": ["custom/niri-windows"],
  "custom/niri-windows": {
    "exec": "/path/to/waybar-niri-windows",
    "return-type": "json",
    "restart-interval": 1,
    "hide-empty-text": true,
    "on-scroll-up": "niri msg action focus-column-left",
    "on-scroll-down": "niri msg action focus-column-right",
    "on-click": "niri msg action toggle-overview"
  }
}
```

Style the module however you like, using a font family that has glyphs for the characters `⋅⊙∗⊛`. for example:

```css
/* Use the icons from the Uiua386 font */
#custom-niri-windows {
  font-family: 'Uiua386';
  font-size: 18px;
  margin-top: -2px;
}
```

Restart Waybar to apply the changes.

## Configuration

Pass command-line arguments to the binary to change the symbols used to draw the indicator:

```
Usage: waybar-niri-windows [options]
  -f, --focused string
        Symbol for focused columns (default "⊙")
  -F, --focused-floating string
        Symbol for focused floating windows (default "⊛")
  -u, --unfocused string
        Symbol for unfocused columns (default "⋅")
  -U, --unfocused-floating string
        Symbol for unfocused floating windows (default "∗")
```

## Contributing

Contributions are welcome! If you find a bug or have a feature request, please open an issue or PR.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
