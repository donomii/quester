# Reverse-proxy deployment

Quester should stay bound to loopback and receive public traffic only through an authenticating TLS reverse proxy. The proxy must replace, rather than forward, the `authentigate-id` header with the authenticated username.

This nginx example accepts only `tasks.example.com`, redirects HTTP to HTTPS, uses HTTP basic authentication as the identity source, and connects to Quester over loopback:

```nginx
server {
    listen 80 default_server;
    server_name _;
    return 404;
}

server {
    listen 80;
    server_name tasks.example.com;
    return 301 https://tasks.example.com$request_uri;
}

server {
    listen 443 ssl default_server;
    server_name _;
    ssl_certificate /etc/letsencrypt/live/tasks.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/tasks.example.com/privkey.pem;
    return 404;
}

server {
    listen 443 ssl;
    server_name tasks.example.com;

    ssl_certificate /etc/letsencrypt/live/tasks.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/tasks.example.com/privkey.pem;

    auth_basic "Quester";
    auth_basic_user_file /etc/nginx/quester.htpasswd;

    location /quester/ {
        proxy_pass http://127.0.0.1:93;
        proxy_set_header Host $host;
        proxy_set_header authentigate-id $remote_user;
        proxy_set_header X-Quester-User "";
        proxy_set_header X-Forwarded-For "";
        proxy_set_header X-Forwarded-Proto https;
    }
}
```

Run Quester with the proxy's exact source address in the trusted list:

```text
QUESTER_ADDR=127.0.0.1:93
QUESTER_PREFIX=/quester/
QUESTER_TRUSTED_PROXIES=127.0.0.1
```

`QUESTER_TRUSTED_PROXIES` accepts comma-separated IP addresses or CIDR blocks. Keep the list as narrow as the deployment allows. A request from another source is rejected, and a request through a trusted proxy is rejected when `authentigate-id` is absent.

After changing the configuration, verify that the configured hostname returns Quester after authentication, an unlisted hostname is rejected, direct access to the Quester port is unavailable from another machine, and a form submission succeeds with the issued CSRF cookie and token.
