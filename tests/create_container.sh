curl -X POST http://localhost:8998/api/v1/containers \
  -H "Authorization: Bearer token" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "app-01",
    "image": "nginx:1.25.3-alpine-slim",
    "ports": {"80": "0"},
    "networks": ["app-network"],
    "subdomain": "app0-1",
    "environment": ["NGINX_HOST=app-01.locci-cloud.localhost"],
    "volumes": ["app-01-vol:/usr/share/nginx/html:ro"]
  }'

