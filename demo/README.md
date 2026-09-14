# Local demo stack

Run KATANA against a **fake JFrog Xray** that returns canned scan results for a few demo images. Useful for screenshots and k3d admission demos without a real Artifactory.

## Demo images

| Image | Mock Xray result |
|-------|------------------|
| `artifactory.example.com/demo/clean:1.0` | indexed, 0 issues |
| `artifactory.example.com/demo/high:1.0` | indexed, High (+ Medium) |
| `artifactory.example.com/demo/critical:1.0` | indexed, Critical (+ High) |
| `artifactory.example.com/demo/unscanned:1.0` | not indexed |
| `artifactory.example.com/demo/unavailable:1.0` | HTTP 503 |

Edit [`dummy-jfrog/catalog.yaml`](dummy-jfrog/catalog.yaml) to change severities.

## Option A — host processes

```bash
# Terminal 1 — mock Xray
cd demo/dummy-jfrog && go run .

# Terminal 2 — Katana API
export JFROG_URL=http://127.0.0.1:8081
export JFROG_TOKEN=demo-token
export KATANA_ADMISSION_DRY_RUN=true
make -C ../.. dev

# Terminal 3 — UI (optional hot reload)
cd web && npm install --omit=optional && npm run dev
# open http://127.0.0.1:5173  (admin / admin)
```

Or with Compose for the mock only:

```bash
docker compose -f demo/docker-compose.yml up --build
```

## Option B — k3d (webhook + mock in-cluster)

Requires `docker`, `k3d`, and `kubectl`.

```bash
bash deploy/scripts/demo-k3d.sh
kubectl -n katana-system port-forward svc/katana 8443:443
# https://127.0.0.1:8443  — admin / admin
```

Manifests: [`k8s/`](k8s/). Tear down with `k3d cluster delete katana-demo`.
