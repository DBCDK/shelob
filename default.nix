{ nixpkgs ? import ./nixpkgs.nix
, pkgs ? import nixpkgs {}
}:

pkgs.buildGoModule {
  name = "shelob";
  src = ./.;
  vendorSha256 = "sha256-+xxGoangySd679nW5fw6LIMZt6hN8br4sifvjQuizGs=";
}
