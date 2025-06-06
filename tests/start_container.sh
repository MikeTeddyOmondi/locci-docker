curl -X POST http://localhost:8998/api/v1/containers/start \
  -H "Authorization: Bearer token" \
  -H "Content-Type: application/json" \
  -d '{
    "container_id": "container-id"
  }'