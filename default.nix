{ nixpkgs ? import ./nixpkgs.nix, pkgs ? import nixpkgs { }, version ? "dev" }:

pkgs.buildGoModule {
  name = "shelob-${version}";
  inherit version;

  src = pkgs.nix-gitignore.gitignoreSource [ ] ./.;

  vendorHash = "sha256-EbB4VzzkYI8/q+IXzLz7KacIUPs/5md024mIRIcQQzk=";
}
