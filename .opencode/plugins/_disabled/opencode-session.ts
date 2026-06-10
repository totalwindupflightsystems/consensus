// Barrel re-export: only OpenCodeSessionPlugin is exposed to OpenCode's plugin loader.
// All internal helpers (applyLimitFn, shouldRunWatchdog, DEFAULT_SESSION_CONFIG, etc.)
// live in lib/opencode-session.ts to prevent OpenCode from iterating them as
// plugin factories.
import { OpenCodeSessionPlugin } from "../lib/opencode-session.ts";
export { OpenCodeSessionPlugin };
