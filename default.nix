{ nixpkgs ? import ./nixpkgs.nix
, pkgs ? import nixpkgs {}
}:

pkgs.buildGoModule {
  name = "shelob";
  src = ./.;
  vendorSha256 = "sha256-PHUjvHwO6UOdWxTavFIkBbm/F1MDlSk/YmSTf1xGKS4=";
}
