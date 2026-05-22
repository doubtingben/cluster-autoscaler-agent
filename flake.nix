{
  description = "cluster-autoscaler-agent development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            golangci-lint
            grpcurl
            protobuf
            buf
            just
          ];

          shellHook = ''
            export GO111MODULE=on
            echo "cluster-autoscaler-agent dev shell ready"
            echo "Try: go test ./..."
          '';
        };
      });
}
