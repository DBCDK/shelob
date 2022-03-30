{ pkgs ? (import <nixpkgs> {}) }:
pkgs.buildGoModule {
  name = "shelob";
  src = ./.;
  vendorSha256 = "sha256-8PCTrAAsp40Z7nXiIC7x24orpr0LBUdALVSGCItqjeE=";
}
