{ nixpkgs ? import ./nixpkgs.nix
, pkgs ? import nixpkgs {}
}:

pkgs.buildGoModule {
  name = "shelob";
  src = ./.;
  vendorSha256 = "sha256-pUrrQJ1XfsBhrhoqC9Qa9vopuE+YA9Xdlbal1mzlj0I=";
}
