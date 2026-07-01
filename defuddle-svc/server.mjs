// [fork] Defuddle sidecar for Hister. Accepts rendered HTML + URL, runs Defuddle
// (Obsidian Web Clipper's extraction engine) via linkedom, returns cleaned content
// + metadata. The Go `defuddle` extractor calls POST /parse. Stateless, persistent.
import http from 'node:http';
import { parseHTML } from 'linkedom';
import { Defuddle } from 'defuddle/node';

const PORT = process.env.PORT || 3000;
const MAX_BYTES = 32 * 1024 * 1024;

const server = http.createServer((req, res) => {
  if (req.method === 'GET' && req.url === '/health') {
    res.writeHead(200); res.end('ok'); return;
  }
  if (req.method !== 'POST' || !req.url.startsWith('/parse')) {
    res.writeHead(404); res.end('not found'); return;
  }
  let body = '';
  req.on('data', (c) => {
    body += c;
    if (body.length > MAX_BYTES) { res.writeHead(413); res.end('too large'); req.destroy(); }
  });
  req.on('end', async () => {
    try {
      const { html, url, markdown } = JSON.parse(body);
      const { document } = parseHTML(html);
      const result = await Defuddle(document, url || '', {
        markdown: !!markdown,
        includeReplies: 'extractors', // keep discussion (Reddit/HN/etc.) via site extractors
      });
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(JSON.stringify(result));
    } catch (e) {
      res.writeHead(500, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ error: String((e && e.message) || e) }));
    }
  });
});

server.listen(PORT, () => console.log('defuddle-svc listening on :' + PORT));
