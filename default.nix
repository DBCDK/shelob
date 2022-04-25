{ pkgs ? (import <nixpkgs> {}) }:
pkgs.buildGoModule {
  name = "shelob";
  src = ./.;
  vendorSha256 = "sha256-kYJg0RJ0v69IUEd8pFBmNERRb+gKrZB5kVbo+I3ZPvI=";
}
