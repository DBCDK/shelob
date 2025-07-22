{ nixpkgs ? import ./nixpkgs.nix, pkgs ? import nixpkgs { }, version ? "dev", }:

pkgs.buildGoModule {
  name = "shelob-${version}";
  inherit version;

  src = pkgs.nix-gitignore.gitignoreSource [ ] ./.;

  vendorHash = "sha256-zPADj0f8zcjXKEex5jx1+Fnw8TmFdUarZU3mi0LLPNk=";
}
