{ nixpkgs ? import ./nixpkgs.nix
, pkgs ? import nixpkgs {}
}:

pkgs.buildGoModule {
  name = "shelob";
  src = ./.;
  vendorSha256 = "sha256-1bQajy/nhArEu0M1CC8pAOztvsMP5YG0+qnh0yQhVsE=";
}
