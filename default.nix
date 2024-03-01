{ nixpkgs ? import ./nixpkgs.nix, pkgs ? import nixpkgs { }, version ? "dev" }:

pkgs.buildGoModule {
  name = "shelob-${version}";
  inherit version;

  src = pkgs.nix-gitignore.gitignoreSource [ ] ./.;

  vendorHash = "sha256-PHUjvHwO6UOdWxTavFIkBbm/F1MDlSk/YmSTf1xGKS4=";
}
