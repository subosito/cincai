{ pkgs, ... }: {
  languages.go.enable = true;
  # Pinned explicitly: the stdlib fixes for GO-2026-5037 (crypto/x509) and
  # GO-2026-5039 (net/textproto) land in Go 1.26.4. Keep this >= 1.26.4.
  languages.go.package = pkgs.go;

  enterShell = ''
    export GOPRIVATE="''${GOPRIVATE:+$GOPRIVATE,}github.com/subosito/cincai"
  '';

  packages = [ pkgs.just pkgs.openssl ];
}