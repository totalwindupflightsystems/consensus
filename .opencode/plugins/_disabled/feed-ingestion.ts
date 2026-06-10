// Barrel re-export: only FeedIngestionPlugin is exposed to OpenCode's plugin loader.
// All internal helpers live in lib/feed-ingestion.ts.
import { FeedIngestionPlugin } from "../lib/feed-ingestion.ts";
export { FeedIngestionPlugin };
