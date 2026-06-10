/**
 * Graph Harness Plugin — thin loader barrel for OpenCode plugin auto-discovery.
 *
 * Follows the exact same pattern as plugins/axiom.ts:
 * - Single named export of the plugin factory function
 * - No default export
 * - No other exports
 *
 * axiom:trace work_item=graph-scheduler-rearchitecture-01 spec=specs/102-Graph-Harness.md
 */

import { GraphHarnessPlugin } from "../lib/graph-harness.ts";

// Re-export ONLY the plugin factory function — matches axiom.ts pattern exactly.
export { GraphHarnessPlugin };
