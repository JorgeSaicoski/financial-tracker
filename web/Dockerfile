# Production build (deploy/compose.yaml). For local dev, use `npm run dev`
# against the bind-mounted source (see docker-compose.yml / README) instead
# of this image — it has no hot reload.

FROM docker.io/library/node:20-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# adapter-node's output is self-contained (SvelteKit/Vite runtime bundled
# in); only `build/` and package.json (for "type": "module") are needed
# at runtime — no node_modules copy.
FROM docker.io/library/node:20-alpine AS run

WORKDIR /app
ENV NODE_ENV=production

COPY --from=build /app/build ./build
COPY --from=build /app/package.json ./package.json

EXPOSE 3000
CMD ["node", "build/index.js"]
