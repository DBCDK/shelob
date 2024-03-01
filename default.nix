{ nixpkgs ? import ./nixpkgs.nix, pkgs ? import nixpkgs { } }:

pkgs.buildGoModule {
  name = "shelob";
  src = ./.;
  vendorHash = "sha256-PHUjvHwO6UOdWxTavFIkBbm/F1MDlSk/YmSTf1xGKS4=";
}
