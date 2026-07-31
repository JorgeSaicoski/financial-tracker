import adapterNode from '@sveltejs/adapter-node';
import adapterStatic from '@sveltejs/adapter-static';

// The deployed Podman stack (deploy/compose.yaml) runs this as its own
// Node server via adapter-node. BACK-09/INFRA-06's standalone binary
// embeds the frontend inside the Go binary instead (internal/webui) —
// there's no Node runtime in there to serve adapter-node's output, so
// that build needs plain static files. BUILD_TARGET=static switches
// which adapter this config emits; only `npm run build:static` (via
// `make web-build-standalone`, see root README) sets it — every other
// build (npm run dev, deploy's Dockerfile, CI) leaves it unset and gets
// adapter-node exactly as before.
const adapter =
	process.env.BUILD_TARGET === 'static'
		? adapterStatic({
				pages: 'build',
				assets: 'build',
				fallback: 'index.html',
				strict: false
			})
		: adapterNode();

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter
	}
};

export default config;
