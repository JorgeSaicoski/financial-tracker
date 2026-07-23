import adapter from '@sveltejs/adapter-node';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		// adapter-node: deployed as a standalone Node server in the Podman
		// stack (deploy/compose.yaml) — adapter-auto can't detect that target.
		adapter: adapter()
	}
};

export default config;
