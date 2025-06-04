```
docker run -d \
  --name=prometheus \
  -p 9090:9090 \
  -v "$PWD/observability/prometheus.yml":/etc/prometheus/prometheus.yml \
  prom/prometheus
```