{ pkgs, ... }: {
  # Go comes from nixpkgs, and devenv sets GOTOOLCHAIN=local, so the toolchain is
  # whatever devenv.lock pins (currently 1.26.4) and builds stay hermetic.
  # go.mod's `go 1.26.4` is the real floor: it wants the stdlib fixes for
  # GO-2026-5037 (crypto/x509) and GO-2026-5039 (net/textproto). The two are
  # coupled — if the locked nixpkgs ever falls below that floor, every build in
  # this shell fails outright rather than quietly using an old stdlib. Fix by
  # bumping the lock (`devenv update nixpkgs`), not by relaxing go.mod.
  languages.go.enable = true;

  packages = [ pkgs.just pkgs.openssl ];
}
