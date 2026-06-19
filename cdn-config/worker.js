export default {
    async fetch(request, env, ctx) {
        const ORIGIN = env.ORIGIN_URL;
        if (!ORIGIN) {
            return new Response('ORIGIN_URL not set', { status: 500 });
        }

        const url = new URL(request.url);

        // Bot blocking
        const botPatterns = /Googlebot|Bingbot|Baiduspider|DuckDuckBot|YandexBot|Slurp|Facebot|Twitterbot|PetalBot|Applebot|AhrefsBot|SemrushBot|DotBot|Bytespider/i;
        if (botPatterns.test(request.headers.get('User-Agent') || '')) {
            return new Response('<html><body></body></html>', {
                status: 404,
                headers: { 'Content-Type': 'text/html' },
            });
        }

        const originUrl = new URL(ORIGIN);
        originUrl.pathname = url.pathname;
        originUrl.search = url.search;
        originUrl.hash = url.hash;

        const modifiedRequest = new Request(originUrl, {
            method: request.method,
            headers: request.headers,
            body: request.body,
            redirect: 'manual',
        });

        modifiedRequest.headers.delete('CF-Connecting-IP');
        modifiedRequest.headers.delete('CF-IPCountry');
        modifiedRequest.headers.delete('CF-Ray');
        modifiedRequest.headers.delete('CF-Visitor');

        try {
            const response = await fetch(modifiedRequest);

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
        } catch (e) {
            return new Response('Origin unreachable: ' + e.message, { status: 502 });
        }
    },
};
