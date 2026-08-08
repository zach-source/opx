{
  lib,
  buildGoModule,
  version ? "dev",
}:

buildGoModule {
  pname = "opx";
  inherit version;

  src = lib.cleanSource ../.;

  vendorHash = "sha256-Dn678Er70nJbUjFzr+L0RSdG+JS52uIkv840s1OgeDs=";

  subPackages = [
    "cmd/opx"
    "cmd/opx-authd"
  ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  meta = {
    description = "Secret batching daemon and client for 1Password, Vault and OpenBao";
    homepage = "https://github.com/zach-source/opx";
    license = lib.licenses.mit;
    mainProgram = "opx";
    platforms = lib.platforms.unix;
  };
}
