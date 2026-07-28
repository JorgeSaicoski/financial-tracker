// This app has no server load functions and reads `window`/localStorage
// directly during init (see lib/auth.svelte.js) — it was already a de
// facto client-rendered SPA even under adapter-node's default SSR.
// Disabling SSR here makes that explicit and is what lets
// adapter-static's static build (BACK-09/INFRA-06's standalone binary,
// see svelte.config.js) prerender without erroring on browser-only
// globals; it's a no-op for the deployed adapter-node build, which
// never relied on SSR output either.
export const ssr = false;
