# niri-windows module for Waybar

This is a WIP module for [Waybar](https://github.com/Alexays/Waybar) that displays a window minimap for the current [niri](https://github.com/YaLTeR/niri) workspace.

![Image of the module](screenshot.png)

> [!IMPORTANT]
> niri ≥ v25.08 is required (for the window locations in IPC messages to be available).

## Installation

Download the latest release for your platform from the [releases
page](https://github.com/calico32/waybar-niri-windows/releases). You can place
the `.so` anywhere permanent. `~/.config/waybar` is a good place.

### From source

If you'd like to build from source (or if your platform doesn't have a pre-built binary):

1. Install GTK 3 + development headers (`apt install libgtk-3-dev`, `pacman -S gtk3`, etc.).
2. Clone this repository.
3. Run `make` to produce `waybar-niri-windows.so`. This might take a while (thanks to cgo).

Move the library anywhere permanent, e.g. `~/.config/waybar`.

Add a CFFI module to your Waybar config (and add any niri actions you want to trigger on scroll):

```jsonc
{
  "modules-left": ["cffi/niri-windows"],
  "cffi/niri-windows": {
    "module_path": "/path/to/waybar-niri-windows.so",
    // add CSS classes to windows based on their App ID (see `niri msg windows`):
    "rules": [
      // .alacritty will be added to all windows with the App ID "Alacritty"
      { "app-id": "Alacritty", "class": "alacritty" }
    ],
    "actions": {
      // use niri IPC action names to trigger them: https://yalter.github.io/niri/niri_ipc/enum.Action.html
      // any action that has no fields is supported
      "on-scroll-up": "FocusColumnLeft",
      "on-scroll-down": "FocusColumnRight"
      // don't configure click actions here: they're handled by the module itself
    }
  }
}
```

Use any of these selectors in your CSS to style the module:

- `.cffi-niri-windows .tile`
- `.cffi-niri-windows .tile.focused`
- `.cffi-niri-windows .tile.<custom-class>` (see `rules` in the config)
- `.cffi-niri-windows .tile.<custom-class>.focused` (see `rules` in the config)
- `.cffi-niri-windows .column`
- `.cffi-niri-windows .column.focused`

```css
.cffi-niri-windows .tile {
  background-color: rgba(255, 255, 255, 0.501);
}
.cffi-niri-windows .tile.focused {
  background-color: rgb(255, 255, 255);
}
```

Restart Waybar to apply the changes.

## Contributing

Contributions are welcome! If you find a bug or have a feature request, please open an issue or PR.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
