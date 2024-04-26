{ nixpkgs ? import ./nixpkgs.nix, pkgs ? import nixpkgs { }, version ? "dev" }:

pkgs.buildGoModule {
  name = "shelob-${version}";
  inherit version;

  src = pkgs.nix-gitignore.gitignoreSource [ ] ./.;

  vendorHash = "sha256-fTJ+9Oe2YPfZzJ4wqKmTVD2aF5GPx/B5bxPBQ0MxgCE=";
}
