# [fork] Defuddle extraction sidecar (Node). Built locally; run as a compose service
# alongside hister. The Go `defuddle` extractor POSTs rendered HTML to /parse.
FROM node:22-alpine
WORKDIR /app
# defuddle = Obsidian Web Clipper's engine; linkedom = fast server-side DOM.
RUN npm install --omit=dev --no-package-lock defuddle linkedom \
    && npm ls defuddle linkedom
COPY server.mjs .
ENV PORT=3000
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:3000/health || exit 1
CMD ["node", "server.mjs"]
