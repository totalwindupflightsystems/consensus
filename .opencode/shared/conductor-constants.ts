/**
 * Shared constants for Conductor — extracted from plugins/conductor.ts
 * so the plugin loader doesn't crash on non-function exports.
 *
 * axiom:trace work_item=plugin-tool-registration-fix spec=specs/107-Conductor.md
 */

// Type inlined here to avoid circular import
export interface ConductorConfig {
  enabled: boolean;
  database_path: string;
  limits: {
    max_concurrent_agents: number;
    max_agents_per_session: number;
    default_timeout_minutes: number;
  };
  polling: {
    completion_check_interval_seconds: number;
    cost_update_interval_seconds: number;
  };
  context_banner: {
    enabled: boolean;
    verbosity: "minimal" | "normal" | "verbose";
    show_cost: boolean;
    show_elapsed: boolean;
  };
  lifecycle: {
    cancel_on_session_end: boolean;
    retention_days: number;
  };
  stash: {
    auto_create: boolean;
    default_log_level: string;
  };
  auth: {
    spire_socket_path: string;
    allow_spawn_secret_fallback: boolean;
    fallback_log_level: string;
  };
}

export const DEFAULT_CONFIG: ConductorConfig = {
  enabled: true,
  database_path: ".conductor/conductor.db",
  limits: {
    max_concurrent_agents: 10,
    max_agents_per_session: 50,
    default_timeout_minutes: 60,
  },
  polling: {
    completion_check_interval_seconds: 5,
    cost_update_interval_seconds: 30,
  },
  context_banner: {
    enabled: true,
    verbosity: "normal",
    show_cost: true,
    show_elapsed: true,
  },
  lifecycle: {
    cancel_on_session_end: true,
    retention_days: 7,
  },
  stash: {
    auto_create: true,
    default_log_level: "decisions",
  },
  auth: {
    spire_socket_path: "/run/spire/sockets/agent.sock",
    allow_spawn_secret_fallback: false,
    fallback_log_level: "critical",
  },
};
