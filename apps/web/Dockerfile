FROM node:26.13.1-alpine AS build
RUN npm install -g pnpm@10.33.2
WORKDIR /work

COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY apps/web/package.json apps/web/
RUN pnpm install --filter web --frozen-lockfile

COPY api/openapi.yaml api/openapi.yaml
COPY apps/web ./apps/web

WORKDIR /work/apps/web
RUN pnpm build

FROM caddy:2-alpine
COPY --from=build /work/apps/web/dist /usr/share/caddy
COPY apps/web/Caddyfile /etc/caddy/Caddyfile
ENV PORT=80
EXPOSE 80
