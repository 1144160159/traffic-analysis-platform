FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY . .

ARG VITE_API_BASE_URL=/api
ARG VITE_WS_URL=/ws/events
ARG VITE_ARKIME_BASE_URL=
ARG VITE_AUTH_ENABLED=true
ARG VITE_USE_MOCK=false
ARG VITE_ENABLE_REALTIME=false
ARG VITE_SCREEN_ACCESS_MODE=protected
ARG VITE_DESKTOP_SMOKE_TOKEN_ENABLED=false

ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
ENV VITE_WS_URL=${VITE_WS_URL}
ENV VITE_ARKIME_BASE_URL=${VITE_ARKIME_BASE_URL}
ENV VITE_AUTH_ENABLED=${VITE_AUTH_ENABLED}
ENV VITE_USE_MOCK=${VITE_USE_MOCK}
ENV VITE_ENABLE_REALTIME=${VITE_ENABLE_REALTIME}
ENV VITE_SCREEN_ACCESS_MODE=${VITE_SCREEN_ACCESS_MODE}
ENV VITE_DESKTOP_SMOKE_TOKEN_ENABLED=${VITE_DESKTOP_SMOKE_TOKEN_ENABLED}

RUN npm run build

FROM nginx:1.25-alpine AS production

RUN apk add --no-cache gettext

COPY --from=builder /app/dist /usr/share/nginx/html
COPY deployments/nginx.conf /etc/nginx/nginx.conf.template
COPY deployments/docker-entrypoint.sh /docker-entrypoint.sh
RUN find /usr/share/nginx/html -type d -exec chmod 0755 {} + \
  && find /usr/share/nginx/html -type f -exec chmod 0644 {} +
RUN chmod +x /docker-entrypoint.sh

RUN echo 'window.__RUNTIME_CONFIG__ = { \
  API_BASE_URL: "${API_BASE_URL}", \
  WS_URL: "${WS_URL}", \
  ARKIME_BASE_URL: "${ARKIME_BASE_URL}", \
  AUTH_ENABLED: "${AUTH_ENABLED}", \
  USE_MOCK: "${USE_MOCK}", \
  ENABLE_REALTIME: "${ENABLE_REALTIME}", \
  SCREEN_ACCESS_MODE: "${SCREEN_ACCESS_MODE}", \
  DESKTOP_SMOKE_TOKEN_ENABLED: "${DESKTOP_SMOKE_TOKEN_ENABLED}" \
};' > /usr/share/nginx/html/config.js.template

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:80/health || exit 1

EXPOSE 80

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
