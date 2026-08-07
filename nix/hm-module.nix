self:
{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.services.opx-authd;

  policyFile = pkgs.writeText "opx-policy.json" (builtins.toJSON cfg.policy);

  args = [
    "--backend=${cfg.backend}"
    "--ttl=${toString cfg.ttl}"
  ]
  ++ lib.optional (cfg.sessionTimeout != null) "--session-timeout=${toString cfg.sessionTimeout}"
  ++ lib.optionals cfg.enableAuditLog [
    "--enable-audit-log"
    "--audit-log-retention-days=${toString cfg.auditLogRetentionDays}"
  ]
  ++ lib.optional (!cfg.persistCache) "--persist-cache=false"
  ++ lib.optional (cfg.policy != null) "--policy=${policyFile}"
  ++ lib.optional cfg.verbose "--verbose"
  ++ cfg.extraFlags;
in
{
  options.services.opx-authd = {
    enable = lib.mkEnableOption "the opx secret batching daemon";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.opx;
      defaultText = lib.literalExpression "opx.packages.\${system}.opx";
      description = "The opx package providing opx-authd.";
    };

    backend = lib.mkOption {
      type = lib.types.enum [
        "opcli"
        "vault"
        "bao"
        "multi"
        "fake"
      ];
      default = "opcli";
      description = "Secret backend to serve.";
    };

    ttl = lib.mkOption {
      type = lib.types.int;
      default = 14400;
      description = ''
        Cache TTL in seconds. The default of 4 hours is what keeps 1Password
        from re-prompting; the daemon raises the session idle timeout to match.
      '';
    };

    sessionTimeout = lib.mkOption {
      type = lib.types.nullOr lib.types.int;
      default = null;
      description = ''
        Session idle timeout in hours. Null leaves the daemon default (8h).
        Values shorter than the cache TTL are raised to it by the daemon.
      '';
    };

    persistCache = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = ''
        Mirror the cache to an AES-256-GCM file so a restarted agent comes back
        warm. The key is held in the login Keychain. Set false to keep secrets
        memory-only, at the cost of a cold cache after every restart.
      '';
    };

    enableAuditLog = lib.mkEnableOption "structured audit logging";

    auditLogRetentionDays = lib.mkOption {
      type = lib.types.int;
      default = 30;
      description = "Days of audit logs to keep (0 = keep all).";
    };

    policy = lib.mkOption {
      type = lib.types.nullOr (lib.types.attrsOf lib.types.anything);
      default = null;
      example = lib.literalExpression ''
        {
          allow = [{ path = "/usr/bin/kubectl"; refs = [ "op://Production/k8s/*" ]; }];
          default_deny = true;
        }
      '';
      description = ''
        Access control policy, rendered to JSON and passed as --policy.

        Note: this lands in the world-readable Nix store. It holds no secret
        values, but the ref patterns and allowed binaries are visible to any
        local user. Point `extraFlags` at a policy file outside the store if
        your vault/item names are themselves sensitive.
      '';
    };

    opPath = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "/usr/local/bin/op";
      description = ''
        Absolute path to the 1Password CLI. A launchd agent does not inherit
        your shell PATH, so set this if `op` lives somewhere non-standard.
      '';
    };

    environmentFile = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      description = ''
        File sourced before starting the daemon, for secrets like VAULT_TOKEN
        that should not land in the Nix store.
      '';
    };

    verbose = lib.mkEnableOption "verbose daemon logging";

    extraFlags = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Extra flags appended to the opx-authd command line.";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = pkgs.stdenv.hostPlatform.isDarwin;
        message = "services.opx-authd currently only supports darwin (launchd).";
      }
    ];

    home.packages = [ cfg.package ];

    launchd.agents.opx-authd = {
      enable = true;
      config = {
        # environmentFile has to be sourced by a shell, so the daemon is never exec'd directly.
        ProgramArguments = [
          "/bin/sh"
          "-c"
          (lib.concatStringsSep " " (
            lib.optional (cfg.environmentFile != null) ". ${lib.escapeShellArg cfg.environmentFile};"
            ++ [ "exec ${cfg.package}/bin/opx-authd" ]
            ++ map lib.escapeShellArg args
          ))
        ];
        RunAtLoad = true;
        KeepAlive = true;
        EnvironmentVariables = lib.optionalAttrs (cfg.opPath != null) {
          OPX_OP_PATH = cfg.opPath;
        };
        StandardOutPath = "${config.home.homeDirectory}/Library/Logs/opx-authd.log";
        StandardErrorPath = "${config.home.homeDirectory}/Library/Logs/opx-authd.log";
      };
    };
  };
}
