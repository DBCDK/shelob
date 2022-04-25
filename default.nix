{ pkgs ? (import (builtins.fetchTarball {
  url = "https://github.com/NixOS/nixpkgs/archive/5fb3a179605141bfa4c9c423f9b1c33658b059c8.tar.gz";
  sha256 = "sha256:1p344s1i1qfy3kb5xrjhqafza9ha76d7y5xb88jr96gl5yq9iw2m";
}) { config = {}; overlays = []; }) }:
pkgs.buildGoModule {
  name = "shelob";
  src = ./.;
  vendorSha256 = "sha256-kYJg0RJ0v69IUEd8pFBmNERRb+gKrZB5kVbo+I3ZPvI=";
}
