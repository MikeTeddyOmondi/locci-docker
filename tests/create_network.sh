curl -X POST http://localhost:8998/api/v1/networks \
  -H "Authorization: Bearer token" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "app-network",
    "driver": "bridge"
  }'
