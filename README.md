# niri-windows module for Waybar

This is a module for [Waybar](https://github.com/Alexays/Waybar) that displays a focus indicator for the current [Niri](https://github.com/YaLTeR/niri) workspace.

![Image of the module](screenshot.png)

## Installation

You'll need Go to build this module, and you should obviously be using Niri and
Waybar for this to work.

Clone this repository and:

- run `go build .`, and copy the resulting binary to your `$PATH`, or
- run `go install .`

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

Style the module however you like, for example:

```css
/* Use the icons from the Uiua386 font */
#custom-niri-windows {
  font-family: 'Uiua386';
  font-size: 18px;
  margin-top: -2px;
}
```

Restart Waybar to apply the changes.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
