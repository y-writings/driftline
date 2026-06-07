{
  description = "driftline CLI";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forEachSystem = f:
        nixpkgs.lib.genAttrs systems (system:
          f system (import nixpkgs { inherit system; })
        );
    in
    {
      packages = forEachSystem (_system: pkgs:
        let
          driftline = pkgs.buildGoModule {
            pname = "driftline";
            version = "0.0.0";

            src = pkgs.lib.cleanSourceWith {
              src = ./.;
              filter = path: _type:
                let
                  rel = pkgs.lib.removePrefix "${toString ./.}/" (toString path);
                in
                rel == "go.mod"
                || rel == "go.sum"
                || rel == "src"
                || pkgs.lib.hasPrefix "src/" rel;
            };
            subPackages = [ "src/cmd/driftline" ];

            vendorHash = "sha256-g+yaVIx4jxpAQ/+WrGKxhVeliYx7nLQe/zsGpxV4Fn4=";

            nativeBuildInputs = [ pkgs.makeWrapper ];

            postFixup = ''
              wrapProgram $out/bin/driftline \
                --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.git ]}
            '';

            ldflags = [
              "-s"
              "-w"
            ];

            meta = {
              description = "Copy files from a GitHub Source Repository into a Target Repository";
              homepage = "https://github.com/y-writings/driftline";
              license = pkgs.lib.licenses.mit;
              mainProgram = "driftline";
            };
          };
        in
        {
          inherit driftline;
          default = driftline;
        });

      apps = forEachSystem (system: _pkgs:
        let
          driftline = {
            type = "app";
            program = "${self.packages.${system}.driftline}/bin/driftline";
          };
        in
        {
          inherit driftline;
          default = driftline;
        });
    };
}
