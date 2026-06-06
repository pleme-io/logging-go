# nix/modules/home-manager.nix — auto-generated typed module
# description: pleme-io's structured-logging convention for Go — the slog-based counterpart to the Rust tracing + tracing-subscriber stack.
{ config, lib, pkgs, ... }: let
  cfg = config.programs.logging-go;
in
{
  config = lib.mkIf cfg.enable {
    home.packages = [
      cfg.package
    ];
  };
  options.programs.logging-go = {
    enable = lib.mkEnableOption "logging-go";
    package = lib.mkOption {
      default = pkgs.logging-go or null;
      type = lib.types.package;
    };
  };
}
