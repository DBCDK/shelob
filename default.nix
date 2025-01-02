{
  nixpkgs ? import ./nixpkgs.nix,
  pkgs ? import nixpkgs { },
  version ? "dev",
}:

pkgs.buildGoModule {
  name = "shelob-${version}";
  inherit version;

  src = pkgs.nix-gitignore.gitignoreSource [ ] ./.;

  vendorHash = "sha256-8pQY8/3CSNpBkLY6NSe2psrr185DkIPqhzYa8zl3/fw=";
}
