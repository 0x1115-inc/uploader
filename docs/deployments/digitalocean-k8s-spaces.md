# Deploy Uploader on DOKS with DigitalOcean Spaces and Managed PostgreSQL

This guide deploys `uploader` to DigitalOcean Kubernetes (DOKS), stores files in Spaces, and stores metadata in DigitalOcean Managed PostgreSQL.

## Architecture

- App: Kubernetes Deployment (`replicas: 2` is supported with Postgres)
- Object storage: DigitalOcean Spaces (S3-compatible)
- Metadata DB: DigitalOcean Managed PostgreSQL

## Prerequisites

- DOKS cluster and `kubectl` configured
- Docker image pushed to a registry
- DigitalOcean Space and access keys
- Managed PostgreSQL cluster + database/user

## 1) Build and push

```bash
docker build -t registry.example.com/uploader:v1.0.0 .
docker push registry.example.com/uploader:v1.0.0
```

## 2) Create namespace and secrets

```bash
kubectl create namespace uploader

kubectl -n uploader create secret generic uploader-secrets \
  --from-literal=S3_ACCESS_KEY='<spaces-access-key>' \
  --from-literal=S3_SECRET_KEY='<spaces-secret-key>' \
  --from-literal=DATABASE_URL='postgres://USER:PASSWORD@HOST:25060/DB?sslmode=require'
```

## 3) Deploy resources

Create `k8s/uploader-do.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: uploader
  namespace: uploader
spec:
  replicas: 2
  selector:
    matchLabels:
      app: uploader
  template:
    metadata:
      labels:
        app: uploader
    spec:
      containers:
        - name: uploader
          image: registry.example.com/uploader:v1.0.0
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
          env:
            - name: PORT
              value: "8080"
            - name: LOG_LEVEL
              value: "info"
            - name: MAX_UPLOAD_MB
              value: "50"
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: uploader-secrets
                  key: DATABASE_URL
            - name: S3_ENDPOINT
              value: "nyc3.digitaloceanspaces.com"
            - name: S3_REGION
              value: "nyc3"
            - name: S3_BUCKET
              value: "uploader-prod"
            - name: S3_USE_SSL
              value: "true"
            - name: S3_ACCESS_KEY
              valueFrom:
                secretKeyRef:
                  name: uploader-secrets
                  key: S3_ACCESS_KEY
            - name: S3_SECRET_KEY
              valueFrom:
                secretKeyRef:
                  name: uploader-secrets
                  key: S3_SECRET_KEY
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: uploader
  namespace: uploader
spec:
  selector:
    app: uploader
  ports:
    - port: 80
      targetPort: 8080
      protocol: TCP
  type: LoadBalancer
```

Apply:

```bash
kubectl apply -f k8s/uploader-do.yaml
```

## 4) Validate

```bash
kubectl -n uploader get pods
kubectl -n uploader get svc uploader
kubectl -n uploader logs deploy/uploader --tail=200 -f
```

Then test upload:

```bash
curl -s -X POST http://<external-ip>/v1/files -F "file=@./test.pdf"
```

## Notes

- `DATABASE_URL` present: app uses Postgres.
- `DATABASE_URL` empty: app falls back to SQLite (`DB_PATH`) for local/dev.
- For DigitalOcean Managed PostgreSQL, keep `sslmode=require`.
