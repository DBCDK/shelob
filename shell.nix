{ nixpkgs ? import ./nixpkgs.nix
, pkgs ? import nixpkgs {}
}:
let
  drv = pkgs.callPackage ./. {};
in
pkgs.mkShell {
  name = "shelob-env";
  inputsFrom = [ drv ];
}
