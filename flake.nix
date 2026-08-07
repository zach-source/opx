{
  description = "opx - 1Password/Vault secret batching daemon and client";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.opx = pkgs.callPackage ./nix/package.nix {
          version = self.shortRev or self.dirtyShortRev or "dev";
        };
        packages.default = self.packages.${system}.opx;

        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.golangci-lint
          ];
        };
      }
    )
    // {
      homeManagerModules.opx = import ./nix/hm-module.nix self;
      homeManagerModules.default = self.homeManagerModules.opx;
    };
}
