# ─── Stage 1: deps ────────────────────────────────────────────────────────────
# Install dependencies in a separate layer so they are cached between builds.
# Only package.json and package-lock.json are copied here — source changes
# don't invalidate the npm install cache.
FROM node:20-alpine AS deps

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --frozen-lockfile

# ─── Stage 2: build ───────────────────────────────────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /app

# Copy deps from previous stage
COPY --from=deps /app/node_modules ./node_modules

# Copy all source files
COPY . .

# NEXT_PUBLIC_* variables are inlined at build time by Next.js.
# Pass the backend URL as a build arg so it gets baked into the JS bundle.
# docker-compose overrides this with the real value at build time.
ARG NEXT_PUBLIC_BACKEND_URL=http://localhost:8080
ENV NEXT_PUBLIC_BACKEND_URL=$NEXT_PUBLIC_BACKEND_URL

RUN npm run build

# ─── Stage 3: run ─────────────────────────────────────────────────────────────
FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production

# Non-root user
RUN addgroup -S dashboard && adduser -S dashboard -G dashboard

# Copy only what next start needs
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json ./package.json

USER dashboard

EXPOSE 3000

CMD ["npm", "start"]