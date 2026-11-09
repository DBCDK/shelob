{ nixpkgs ? import ./nixpkgs.nix, pkgs ? import nixpkgs { }, version ? "dev", }:

pkgs.buildGoModule {
  name = "shelob-${version}";
  inherit version;

  src = pkgs.nix-gitignore.gitignoreSource [ ] ./.;

  vendorHash = "sha256-C9fukLjussA0UQ5/3H2ijpnWYr43N436byYrysDc2Ms=";
}
