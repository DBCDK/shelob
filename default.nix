{
  nixpkgs ? import ./nixpkgs.nix,
  pkgs ? import nixpkgs { },
  version ? "dev",
}:

pkgs.buildGoModule {
  name = "shelob-${version}";
  inherit version;

  src = pkgs.nix-gitignore.gitignoreSource [ ] ./.;

  vendorHash = "sha256-EVG4fpkfT+zruAfbagi4ku7Icng6DBS9/7o4Ch1bThw=";
}
