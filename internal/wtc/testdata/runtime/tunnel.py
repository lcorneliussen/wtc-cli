#!/usr/bin/env python3
"""Tunnel stand-in whose URL is only known after startup.

It relays on a free loopback port and announces the URL in the output shape of
a real tunnel client (TUNNEL_STYLE: cloudflared, ngrok or devtunnel). It never
exposes a public endpoint.
"""
import http.client
import http.server
import os
import time


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        upstream = http.client.HTTPConnection('127.0.0.1', int(os.environ['PORT']), timeout=2)
        try:
            upstream.request('GET', self.path)
            response = upstream.getresponse()
            self.send_response(response.status)
            self.end_headers()
            self.wfile.write(response.read())
        finally:
            upstream.close()

    def log_message(self, *args):
        pass


server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
url = f'http://127.0.0.1:{server.server_port}'
style = os.environ.get('TUNNEL_STYLE', 'cloudflared')
if style == 'cloudflared':
    print('INF +--------------------------------------------------------+')
    print('INF |  Your quick Tunnel has been created! Visit it at:      |')
    print(f'INF |  {url}  |')
    print('INF +--------------------------------------------------------+', flush=True)
elif style == 'ngrok':
    print(f't=2026-01-01T00:00:00+0000 lvl=info msg="started tunnel" obj=tunnels name=command_line addr=http://localhost:{os.environ["PORT"]} url={url}', flush=True)
else:
    print('Hosting port: ' + os.environ['PORT'])
    print(f'Connect via browser: {url}')
    print(f'Inspect network activity: {url}/inspect', flush=True)
print('TUNNEL_READY', flush=True)
# A real client announces its URL before certificates or routes are in place.
time.sleep(float(os.environ.get('TUNNEL_WARMUP', '0')))
server.serve_forever()
