{pkgs ? import <nixpkgs> {}}:
pkgs.buildGoModule {
  pname = "waybar-niri-windows";
  version = "1.0.0";

  src = ./.;

  vendorHash = null; # Set to null for automatic fetching, or run 'nix-build' to get the hash

  # Optional: Set the main package if not at root
  # subPackages = [ "." ];

  meta = with pkgs.lib; {
    description = "Waybar module for Niri windows";
    license = licenses.mit;
    maintainers = [];
  };
}
