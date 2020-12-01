{ pkgs ? (import <nixpkgs> {}) }:
let
  go = pkgs.go_1_15;

  travis-build = pkgs.writeShellScriptBin "travis-build" ''
    set -euo pipefail
    GOROOT="${go}/share/go" GOPATH=/tmp ${go}/bin/go build
  '';
in
pkgs.mkShell {
  name = "shelob-env";
  buildInputs = [travis-build go pkgs.git];
}
